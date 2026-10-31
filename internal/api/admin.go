package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/backup"
	"github.com/Murathan-Aydin/nodra/internal/jobs"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/store"
	"github.com/Murathan-Aydin/nodra/internal/version"
)

// instanceCache : réglages de l'instance relus au plus toutes les 5 s (consultés à chaque requête par jeton).
type instanceCache struct {
	mu sync.Mutex
	v  *store.InstanceSettings
	at time.Time
}

func (s *Server) instance(ctx context.Context) (*store.InstanceSettings, error) {
	s.inst.mu.Lock()
	defer s.inst.mu.Unlock()
	if s.inst.v != nil && time.Since(s.inst.at) < 5*time.Second {
		return s.inst.v, nil
	}
	v, err := s.Store.InstanceSettings(ctx)
	if err != nil {
		return nil, err
	}
	s.inst.v, s.inst.at = v, time.Now()
	return v, nil
}

func (s *Server) forgetInstance() {
	s.inst.mu.Lock()
	s.inst.v = nil
	s.inst.mu.Unlock()
}

// ipAllowed : ip figure dans la liste (adresses ou plages CIDR).
func ipAllowed(ip string, list []string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, e := range list {
		if p, err := netip.ParsePrefix(e); err == nil && p.Contains(addr) {
			return true
		}
		if a, err := netip.ParseAddr(e); err == nil && a.Unmap() == addr {
			return true
		}
	}
	return false
}

// publicInstance : informations publiques (page de connexion du front).
func (s *Server) publicInstance(w http.ResponseWriter, r *http.Request) {
	i, err := s.instance(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": i.InstanceName, "registration_enabled": i.RegistrationEnabled,
		"version": version.Version, "oauth_providers": s.enabledOAuth(r.Context())})
}

func settingsView(i *store.InstanceSettings) map[string]any {
	return map[string]any{"settings": i, "version": version.Version, "update_available": version.Newer(i.LatestVersion, version.Version)}
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	i, err := s.Store.InstanceSettings(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsView(i))
}

type settingsInput struct {
	FQDN                 *string   `json:"fqdn"`
	InstanceName         *string   `json:"instance_name"`
	PublicIPv4           *string   `json:"public_ipv4"`
	PublicIPv6           *string   `json:"public_ipv6"`
	Timezone             *string   `json:"timezone"`
	RegistrationEnabled  *bool     `json:"registration_enabled"`
	AutoUpdateEnabled    *bool     `json:"auto_update_enabled"`
	AutoUpdateFrequency  *string   `json:"auto_update_frequency"`
	UpdateCheckFrequency *string   `json:"update_check_frequency"`
	DNSValidationEnabled *bool     `json:"dns_validation_enabled"`
	DNSServers           *[]string `json:"dns_servers"`
	APIEnabled           *bool     `json:"api_enabled"`
	APIAllowedIPs        *[]string `json:"api_allowed_ips"`
	MCPEnabled           *bool     `json:"mcp_enabled"`
}

// normalizeFQDN : URL http(s) de l'instance, sans chemin (https:// par défaut).
func normalizeFQDN(v string) (*string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("fqdn : URL http(s) sans chemin, ex. https://nodra.example.com")
	}
	out := u.Scheme + "://" + strings.ToLower(u.Host)
	return &out, nil
}

// applySettings valide et applique l'entrée ; renvoie les champs modifiés.
func applySettings(in *settingsInput, i *store.InstanceSettings) ([]string, error) {
	var changed []string
	set := func(name string, ok bool) {
		if ok {
			changed = append(changed, name)
		}
	}
	if in.FQDN != nil {
		v, err := normalizeFQDN(*in.FQDN)
		if err != nil {
			return nil, err
		}
		i.FQDN = v
		set("fqdn", true)
	}
	if in.InstanceName != nil {
		v := strings.TrimSpace(*in.InstanceName)
		if v == "" || len(v) > 100 {
			return nil, errors.New("instance_name : 1 à 100 caractères")
		}
		i.InstanceName = v
		set("instance_name", true)
	}
	if in.PublicIPv4 != nil {
		v := strings.TrimSpace(*in.PublicIPv4)
		if a, err := netip.ParseAddr(v); v != "" && (err != nil || !a.Is4()) {
			return nil, errors.New("public_ipv4 invalide")
		}
		i.PublicIPv4 = v
		set("public_ipv4", true)
	}
	if in.PublicIPv6 != nil {
		v := strings.TrimSpace(*in.PublicIPv6)
		if a, err := netip.ParseAddr(v); v != "" && (err != nil || !a.Is6() || a.Is4In6()) {
			return nil, errors.New("public_ipv6 invalide")
		}
		i.PublicIPv6 = v
		set("public_ipv6", true)
	}
	if in.Timezone != nil {
		if _, err := time.LoadLocation(*in.Timezone); err != nil || *in.Timezone == "" {
			return nil, errors.New("timezone inconnu (ex. Europe/Paris)")
		}
		i.Timezone = *in.Timezone
		set("timezone", true)
	}
	for name, p := range map[string]*string{"auto_update_frequency": in.AutoUpdateFrequency, "update_check_frequency": in.UpdateCheckFrequency} {
		if p == nil {
			continue
		}
		if err := backup.ParseCron(*p); err != nil {
			return nil, errors.New(name + " : expression cron invalide")
		}
		if name == "auto_update_frequency" {
			i.AutoUpdateFrequency = *p
		} else {
			i.UpdateCheckFrequency = *p
		}
		set(name, true)
	}
	if in.DNSServers != nil {
		list := []string{}
		for _, v := range *in.DNSServers {
			v = strings.TrimSpace(v)
			host := v
			if h, port, err := net.SplitHostPort(v); err == nil {
				if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
					return nil, errors.New("dns_servers : port invalide pour " + v)
				}
				host = h
			}
			if _, err := netip.ParseAddr(host); err != nil {
				return nil, errors.New("dns_servers : adresse IP attendue, reçu " + v)
			}
			if !slices.Contains(list, v) {
				list = append(list, v)
			}
		}
		if len(list) > 5 {
			return nil, errors.New("dns_servers : 5 serveurs au maximum")
		}
		i.DNSServers = list
		set("dns_servers", true)
	}
	if in.APIAllowedIPs != nil {
		list := []string{}
		for _, v := range *in.APIAllowedIPs {
			v = strings.TrimSpace(v)
			if p, err := netip.ParsePrefix(v); err == nil {
				v = p.Masked().String()
			} else if a, err := netip.ParseAddr(v); err == nil {
				v = a.Unmap().String()
			} else {
				return nil, errors.New("api_allowed_ips : adresse IP ou plage CIDR attendue, reçu " + v)
			}
			if !slices.Contains(list, v) {
				list = append(list, v)
			}
		}
		if len(list) > 100 {
			return nil, errors.New("api_allowed_ips : 100 entrées au maximum")
		}
		i.APIAllowedIPs = list
		set("api_allowed_ips", true)
	}
	for name, b := range map[string]struct {
		in  *bool
		dst *bool
	}{
		"registration_enabled":   {in.RegistrationEnabled, &i.RegistrationEnabled},
		"auto_update_enabled":    {in.AutoUpdateEnabled, &i.AutoUpdateEnabled},
		"dns_validation_enabled": {in.DNSValidationEnabled, &i.DNSValidationEnabled},
		"api_enabled":            {in.APIEnabled, &i.APIEnabled},
		"mcp_enabled":            {in.MCPEnabled, &i.MCPEnabled},
	} {
		if b.in != nil {
			*b.dst = *b.in
			set(name, true)
		}
	}
	slices.Sort(changed)
	return changed, nil
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var in settingsInput
	if !decode(w, r, &in) {
		return
	}
	i, err := s.Store.InstanceSettings(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	changed, err := applySettings(&in, i)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := s.Store.UpdateInstanceSettings(r.Context(), i); err != nil {
		s.internal(w, err)
		return
	}
	s.forgetInstance()
	s.audit(r, nil, "instance.settings.updated", "instance", "settings", map[string]any{"fields": changed})
	if slices.Contains(changed, "fqdn") {
		s.routeInstance(r.Context(), i.FQDN)
	}
	writeJSON(w, http.StatusOK, settingsView(i))
}

// checkUpdates interroge immédiatement les releases publiées.
func (s *Server) checkUpdates(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	latest, err := version.Latest(ctx, s.ReleasesAPI)
	if err != nil {
		writeError(w, http.StatusBadGateway, "vérification impossible : "+err.Error())
		return
	}
	if err := s.Store.SetLatestVersion(r.Context(), latest); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": version.Version, "latest_version": latest,
		"update_available": version.Newer(latest, version.Version)})
}

// --- Utilisateurs ---

func (s *Server) listAdminUsers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	list, err := s.Store.ListUsers(r.Context(), strings.TrimSpace(r.URL.Query().Get("search")), limit)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// adminTarget : utilisateur visé, jamais soi-même (évite de perdre son propre accès).
func (s *Server) adminTarget(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	id, ok := idParam(w, r, "user")
	if !ok {
		return nil, false
	}
	if id == userFrom(r.Context()).ID {
		writeError(w, http.StatusConflict, "action impossible sur votre propre compte")
		return nil, false
	}
	u, err := s.Store.UserByID(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return u, true
}

func (s *Server) updateAdminUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminTarget(w, r)
	if !ok {
		return
	}
	var in struct {
		IsInstanceAdmin *bool `json:"is_instance_admin"`
		Disabled        *bool `json:"disabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	if in.IsInstanceAdmin != nil {
		if err := s.Store.SetUserAdmin(ctx, u.ID, *in.IsInstanceAdmin); err != nil {
			s.fail(w, err)
			return
		}
	}
	if in.Disabled != nil {
		if err := s.Store.SetUserDisabled(ctx, u.ID, *in.Disabled); err != nil {
			s.fail(w, err)
			return
		}
	}
	s.audit(r, nil, "instance.user.updated", "user", u.ID, map[string]any{"email": u.Email, "is_instance_admin": in.IsInstanceAdmin,
		"disabled": in.Disabled})
	u, err := s.Store.UserByID(ctx, u.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) logoutAdminUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminTarget(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteUserSessions(r.Context(), u.ID); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "instance.user.logged_out", "user", u.ID, map[string]string{"email": u.Email})
	w.WriteHeader(http.StatusNoContent)
}

// adminAudit : journal sans équipe (connexions, administration) ou de toute l'instance (scope=all).
func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	base := store.AuditFilter{Instance: true}
	if r.URL.Query().Get("scope") == "all" {
		base = store.AuditFilter{AllTeams: true}
	}
	f, ok := parseAuditQuery(w, r, base, 200)
	if !ok {
		return
	}
	if r.URL.Query().Get("limit") == "" {
		f.Limit = 50
	}
	events, err := s.Store.ListAudit(r.Context(), f)
	if err != nil {
		s.internal(w, err)
		return
	}
	var next *int64
	if len(events) == f.Limit {
		next = &events[len(events)-1].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "next_before": next})
}

// routeInstance écrit (ou retire) sur le proxy du serveur localhost la configuration qui sert
// l'interface de Nodra sur son domaine, puis synchronise le proxy.
func (s *Server) routeInstance(ctx context.Context, fqdn *string) {
	id, err := s.Store.LocalhostServerID(ctx)
	if err != nil || id == "" {
		return
	}
	sv, err := s.Store.ServerByIDAny(ctx, id)
	if err != nil {
		return
	}
	for _, typ := range []string{servers.ProxyTraefik, servers.ProxyCaddy} { // anciens fichiers retirés
		if name, _ := servers.InstanceProxyConfig(typ, "https://x"); name != "" {
			_ = s.Store.DeleteProxyConfig(ctx, sv.ID, name)
		}
	}
	if fqdn != nil {
		if name, content := servers.InstanceProxyConfig(sv.Proxy.Type, *fqdn); name != "" {
			if err := s.Store.UpsertProxyConfig(ctx, sv.ID, name, content); err != nil {
				s.Log.Error("domaine de l'instance", "err", err)
				return
			}
		}
	}
	s.syncProxy(ctx, sv)
}

var releaseTagRe = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// startUpdate lance la mise à jour de l'instance (update.sh sur le serveur localhost).
func (s *Server) startUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version string `json:"version"`
	}
	if r.ContentLength > 0 && !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	if in.Version == "" {
		i, err := s.Store.InstanceSettings(ctx)
		if err != nil {
			s.internal(w, err)
			return
		}
		in.Version = i.LatestVersion
	}
	if !releaseTagRe.MatchString(in.Version) {
		writeError(w, http.StatusUnprocessableEntity, "version invalide (ex. v1.2.0) : lancez d'abord /admin/update-check")
		return
	}
	if s.InstallDir == "" {
		writeError(w, http.StatusConflict, jobs.ErrNoLocalhost.Error())
		return
	}
	task, err := jobs.StartInstanceUpdate(ctx, s.Store, s.Jobs.EnqueueServerTask, s.InstallDir, in.Version, &userFrom(ctx).ID)
	if errors.Is(err, jobs.ErrNoLocalhost) {
		writeError(w, http.StatusConflict, err.Error())
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "instance.update.started", "instance", "update", map[string]string{"version": in.Version, "from": version.Version})
	writeJSON(w, http.StatusAccepted, task)
}
