package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Resolver résout les noms de domaine (remplaçable dans les tests).
type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

func (s *Server) getProxy(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, proxyView(sv))
}

func proxyView(sv *store.Server) map[string]any {
	return map[string]any{
		"type": sv.Proxy.Type, "status": sv.Proxy.Status, "acme_email": sv.Proxy.AcmeEmail,
		"compose": servers.EffectiveCompose(sv.Proxy), "is_custom": strings.TrimSpace(sv.Proxy.Compose) != "",
		"default_compose": servers.DefaultProxyCompose(sv.Proxy.Type, sv.Proxy.AcmeEmail),
	}
}

func (s *Server) updateProxy(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.proxy.manage")
	if !ok {
		return
	}
	var in struct {
		Type      *string `json:"type"`
		AcmeEmail *string `json:"acme_email"`
		Compose   *string `json:"compose"` // "" = revenir à la configuration générée
	}
	if !decode(w, r, &in) {
		return
	}
	p := sv.Proxy
	if in.Type != nil && *in.Type != p.Type {
		if !slices.Contains([]string{servers.ProxyTraefik, servers.ProxyCaddy, servers.ProxyNone}, *in.Type) {
			writeError(w, http.StatusUnprocessableEntity, "type de proxy : traefik, caddy ou none")
			return
		}
		if p.Status == "running" {
			writeError(w, http.StatusConflict, "arrêtez le proxy avant d'en changer")
			return
		}
		p.Type, p.Compose = *in.Type, "" // la configuration personnalisée ne vaut que pour l'ancien type
	}
	if in.AcmeEmail != nil {
		p.AcmeEmail = strings.TrimSpace(*in.AcmeEmail)
		if _, err := mail.ParseAddress(p.AcmeEmail); p.AcmeEmail != "" && err != nil {
			writeError(w, http.StatusUnprocessableEntity, "adresse e-mail ACME invalide")
			return
		}
	}
	if in.Compose != nil {
		p.Compose = *in.Compose
		if strings.TrimSpace(p.Compose) != "" {
			if p.Type == servers.ProxyNone {
				writeError(w, http.StatusUnprocessableEntity, "aucun proxy configuré")
				return
			}
			if err := servers.ValidateCompose(p.Compose); err != nil {
				writeError(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
		}
	}
	if err := s.Store.UpdateProxy(r.Context(), sv.ID, p); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "server.proxy.updated", "server", sv.ID, map[string]any{
		"before": map[string]any{"type": sv.Proxy.Type, "acme_email": sv.Proxy.AcmeEmail, "custom": sv.Proxy.Compose != ""},
		"after":  map[string]any{"type": p.Type, "acme_email": p.AcmeEmail, "custom": p.Compose != ""},
	})
	sv.Proxy = p
	writeJSON(w, http.StatusOK, proxyView(sv))
}

// syncProxy réécrit la configuration du proxy sur le serveur (si le serveur est joignable).
func (s *Server) syncProxy(ctx context.Context, sv *store.Server) {
	if sv.HostKey == "" || sv.Proxy.Type == servers.ProxyNone {
		return // écrite au prochain démarrage du proxy
	}
	uid := userFrom(ctx).ID
	if _, err := s.Jobs.EnqueueServerTask(ctx, &store.ServerTask{TeamID: sv.TeamID, ServerID: sv.ID,
		Kind: servers.TaskProxySync, CreatedBy: &uid}); err != nil {
		s.Log.Error("synchronisation du proxy", "server", sv.ID, "err", err)
	}
}

// --- Configuration dynamique ---

func (s *Server) listProxyConfigs(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return
	}
	list, err := s.Store.ListProxyConfigs(r.Context(), sv.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) putProxyConfig(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.proxy.manage")
	if !ok {
		return
	}
	name := chi.URLParam(r, "name")
	var in struct {
		Content string `json:"content"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !servers.ProxyConfigNameRe.MatchString(name) {
		writeError(w, http.StatusUnprocessableEntity, "nom de fichier invalide (ex. redirections.yml ou site.caddy)")
		return
	}
	if err := servers.ValidateDynamicConfig(sv.Proxy.Type, name, in.Content); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := s.Store.UpsertProxyConfig(r.Context(), sv.ID, name, in.Content); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "server.proxy.config_saved", "server", sv.ID, map[string]string{"name": name})
	s.syncProxy(r.Context(), sv)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteProxyConfig(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.proxy.manage")
	if !ok {
		return
	}
	name := chi.URLParam(r, "name")
	if err := s.Store.DeleteProxyConfig(r.Context(), sv.ID, name); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "server.proxy.config_deleted", "server", sv.ID, map[string]string{"name": name})
	s.syncProxy(r.Context(), sv)
	w.WriteHeader(http.StatusNoContent)
}

// --- Certificats personnalisés ---

func (s *Server) listCertificates(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return
	}
	list, err := s.Store.ListCertificates(r.Context(), sv.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// ParseCertificate vérifie qu'un certificat PEM et sa clé correspondent ; renvoie domaines et expiration.
func ParseCertificate(certPEM, keyPEM string, now time.Time) ([]string, time.Time, error) {
	if _, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM)); err != nil {
		return nil, time.Time{}, fmt.Errorf("certificat ou clé invalide, ou ne correspondant pas : %w", err)
	}
	block, _ := pem.Decode([]byte(certPEM))
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, time.Time{}, err
	}
	if now.After(cert.NotAfter) {
		return nil, time.Time{}, errors.New("le certificat a expiré")
	}
	domains := cert.DNSNames
	if len(domains) == 0 && cert.Subject.CommonName != "" {
		domains = []string{cert.Subject.CommonName}
	}
	if len(domains) == 0 {
		return nil, time.Time{}, errors.New("le certificat ne contient aucun nom de domaine")
	}
	return domains, cert.NotAfter, nil
}

func (s *Server) createCertificate(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.proxy.manage")
	if !ok {
		return
	}
	var in struct {
		Name        string `json:"name"`
		Certificate string `json:"certificate"`
		PrivateKey  string `json:"private_key"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (100 caractères max.)")
		return
	}
	domains, expires, err := ParseCertificate(in.Certificate, in.PrivateKey, time.Now())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	enc, err := s.Box.Encrypt(in.PrivateKey)
	if err != nil {
		s.internal(w, err)
		return
	}
	c, err := s.Store.CreateCertificate(r.Context(), &store.Certificate{ServerID: sv.ID, Name: in.Name, Domains: domains,
		Certificate: in.Certificate, PrivateKeyEnc: enc, ExpiresAt: expires})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "server.certificate.created", "server", sv.ID, map[string]any{"name": c.Name, "domains": domains, "expires_at": expires})
	s.syncProxy(r.Context(), sv)
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) deleteCertificate(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.proxy.manage")
	if !ok {
		return
	}
	id, ok := idParam(w, r, "cert")
	if !ok {
		return
	}
	if err := s.Store.DeleteCertificate(r.Context(), sv.ID, id); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "server.certificate.deleted", "server", sv.ID, map[string]string{"certificate_id": id})
	s.syncProxy(r.Context(), sv)
	w.WriteHeader(http.StatusNoContent)
}

// --- Vérification DNS ---

// NormalizeDomain extrait le nom d'hôte d'une saisie (URL complète, port, chemin) et le valide.
func NormalizeDomain(in string) (string, bool) {
	in = strings.TrimSpace(strings.ToLower(in))
	if strings.Contains(in, "://") {
		u, err := url.Parse(in)
		if err != nil {
			return "", false
		}
		in = u.Hostname()
	} else if h, _, err := net.SplitHostPort(in); err == nil {
		in = h
	}
	in = strings.TrimSuffix(strings.SplitN(in, "/", 2)[0], ".")
	check := strings.TrimPrefix(in, "*.")
	return in, strings.Contains(check, ".") && hostnameRe.MatchString(check)
}

func (s *Server) dnsCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Domain   string `json:"domain"`
		ServerID string `json:"server_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	domain, ok := NormalizeDomain(in.Domain)
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "nom de domaine invalide")
		return
	}
	if !isUUID(in.ServerID) {
		writeError(w, http.StatusUnprocessableEntity, "server_id requis")
		return
	}
	ctx := r.Context()
	sv, err := s.Store.ServerByID(ctx, teamFrom(ctx).Team.ID, in.ServerID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !s.authorize(w, r, "server.read", rbac.Scope{Type: rbac.ScopeServer, ID: sv.ID}) {
		return
	}
	inst, err := s.instance(ctx)
	if err != nil {
		s.internal(w, err)
		return
	}
	if !inst.DNSValidationEnabled {
		writeJSON(w, http.StatusOK, map[string]any{"domain": domain, "resolved": []string{}, "expected": []string{sv.IP}, "ok": true,
			"skipped": true, "message": "vérification DNS désactivée sur l'instance"})
		return
	}
	resolver := s.Resolver
	if resolver == nil {
		resolver = dnsResolver(inst.DNSServers)
	}
	lookup := domain
	if strings.HasPrefix(domain, "*.") { // domaine joker : on teste un sous-domaine au hasard
		lookup = fmt.Sprintf("nodra-check-%d%s", rand.IntN(1_000_000), domain[1:])
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resolved, lerr := resolver.LookupHost(cctx, lookup)
	expected := []string{sv.IP}
	if net.ParseIP(sv.IP) == nil {
		if ips, err := resolver.LookupHost(cctx, sv.IP); err == nil {
			expected = ips
		}
	}
	match := slices.ContainsFunc(resolved, func(ip string) bool { return slices.Contains(expected, ip) })
	msg := "le domaine pointe vers le serveur"
	switch {
	case lerr != nil:
		msg = "le domaine ne résout vers aucune adresse : créez un enregistrement A/AAAA vers " + strings.Join(expected, ", ")
	case !match && sv.Settings.CloudflareTunnel:
		msg = "le serveur utilise Cloudflare Tunnel : le domaine peut pointer vers Cloudflare"
		match = true
	case !match:
		msg = "le domaine ne pointe pas vers ce serveur (proxy Cloudflare actif ?) : attendu " + strings.Join(expected, ", ")
	}
	if resolved == nil {
		resolved = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"domain": domain, "resolved": resolved, "expected": expected, "ok": match, "message": msg})
}

// dnsResolver : résolveur système, ou interrogeant les serveurs DNS réglés sur l'instance (port 53 par défaut).
func dnsResolver(servers []string) Resolver {
	if len(servers) == 0 {
		return net.DefaultResolver
	}
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		var err error
		for _, srv := range servers {
			if _, _, e := net.SplitHostPort(srv); e != nil {
				srv = net.JoinHostPort(srv, "53")
			}
			c, e := d.DialContext(ctx, network, srv)
			if e == nil {
				return c, nil
			}
			err = e
		}
		return nil, err
	}}
}
