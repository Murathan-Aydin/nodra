package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/auth"
	"github.com/Murathan-Aydin/nodra/internal/git"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const manifestStateTTL = time.Hour

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// sourceView : source sans secrets, avec des indicateurs d'état.
func sourceView(g *store.GitSource) map[string]any {
	return map[string]any{
		"id": g.ID, "team_id": g.TeamID, "type": g.Type, "name": g.Name, "html_url": g.HTMLURL, "api_url": g.APIURL,
		"organization": g.Organization, "is_public": g.IsPublic, "app_id": g.AppID, "app_slug": g.AppSlug, "client_id": g.ClientID,
		"installation_id": g.InstallationID, "created_at": g.CreatedAt,
		"configured": g.Type != git.GitHub || g.AppID > 0, "installed": g.Type != git.GitHub || g.InstallationID > 0,
		"has_private_key": g.PrivateKeyEnc != "", "has_webhook_secret": g.WebhookSecretEnc != "", "has_access_token": g.AccessTokenEnc != "",
	}
}

// gitSource déchiffre les identifiants d'une source pour le paquet git.
func (s *Server) gitSource(g *store.GitSource) (git.Source, error) {
	src := git.Source{ID: g.ID, Type: g.Type, APIURL: strings.TrimSuffix(g.APIURL, "/"), HTMLURL: g.HTMLURL,
		AppID: g.AppID, InstallationID: g.InstallationID}
	var err error
	if g.PrivateKeyEnc != "" {
		if src.PrivateKey, err = s.Box.Decrypt(g.PrivateKeyEnc); err != nil {
			return src, err
		}
	}
	if g.AccessTokenEnc != "" {
		if src.AccessToken, err = s.Box.Decrypt(g.AccessTokenEnc); err != nil {
			return src, err
		}
	}
	return src, nil
}

// baseURL : URL publique de l'instance (réglage d'instance, sinon déduite de la requête).
func (s *Server) baseURL(r *http.Request) string {
	if st, err := s.Store.InstanceSettings(r.Context()); err == nil && st.FQDN != nil && *st.FQDN != "" {
		return strings.TrimSuffix(*st.FQDN, "/")
	}
	scheme := "http"
	if r.TLS != nil || (s.TrustProxy && r.Header.Get("X-Forwarded-Proto") == "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "source.read", rbac.Scope{}) {
		return
	}
	list, err := s.Store.ListGitSources(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	out := make([]map[string]any, len(list))
	for i, g := range list {
		out[i] = sourceView(g)
	}
	writeJSON(w, http.StatusOK, out)
}

type sourceInput struct {
	Type           string  `json:"type"`
	Name           *string `json:"name"`
	HTMLURL        *string `json:"html_url"`
	APIURL         *string `json:"api_url"`
	Organization   *string `json:"organization"`
	IsPublic       *bool   `json:"is_public"`
	AppID          *int64  `json:"app_id"`
	InstallationID *int64  `json:"installation_id"`
	ClientID       *string `json:"client_id"`
	ClientSecret   *string `json:"client_secret"`
	WebhookSecret  *string `json:"webhook_secret"`
	PrivateKey     *string `json:"private_key"`
	AccessToken    *string `json:"access_token"`
}

// apply fusionne l'entrée dans g (secrets chiffrés) et renvoie un message d'erreur de validation.
func (s *Server) applySource(in *sourceInput, g *store.GitSource) (string, error) {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	set(&g.Name, in.Name)
	set(&g.HTMLURL, in.HTMLURL)
	set(&g.APIURL, in.APIURL)
	set(&g.Organization, in.Organization)
	set(&g.ClientID, in.ClientID)
	if in.IsPublic != nil {
		g.IsPublic = *in.IsPublic
	}
	if in.AppID != nil {
		g.AppID = *in.AppID
	}
	if in.InstallationID != nil {
		g.InstallationID = *in.InstallationID
	}
	g.HTMLURL = strings.TrimSuffix(g.HTMLURL, "/")
	if g.HTMLURL == "" {
		g.HTMLURL = map[string]string{git.GitHub: "https://github.com", git.GitLab: "https://gitlab.com"}[g.Type]
	}
	if g.APIURL == "" {
		switch {
		case g.Type == git.GitHub && g.HTMLURL == "https://github.com":
			g.APIURL = "https://api.github.com"
		case g.Type == git.GitHub:
			g.APIURL = g.HTMLURL + "/api/v3" // GitHub Enterprise Server
		case g.Type == git.GitLab:
			g.APIURL = g.HTMLURL + "/api/v4"
		case g.Type == git.Gitea:
			g.APIURL = g.HTMLURL + "/api/v1"
		}
	}
	g.APIURL = strings.TrimSuffix(g.APIURL, "/")
	enc := func(dst *string, v *string) error {
		if v == nil {
			return nil
		}
		if strings.TrimSpace(*v) == "" {
			*dst = ""
			return nil
		}
		val := strings.TrimSpace(*v)
		if strings.HasPrefix(val, "-----BEGIN") {
			val += "\n" // une clé PEM se termine par un saut de ligne
		}
		e, err := s.Box.Encrypt(val)
		*dst = e
		return err
	}
	for _, f := range []struct {
		dst *string
		v   *string
	}{{&g.ClientSecretEnc, in.ClientSecret}, {&g.WebhookSecretEnc, in.WebhookSecret}, {&g.PrivateKeyEnc, in.PrivateKey}, {&g.AccessTokenEnc, in.AccessToken}} {
		if err := enc(f.dst, f.v); err != nil {
			return "", err
		}
	}
	switch {
	case g.Name == "" || len(g.Name) > 100:
		return "nom requis (100 caractères max.)", nil
	case !validHTTPURL(g.HTMLURL) || !validHTTPURL(g.APIURL):
		return "html_url et api_url doivent être des URL http(s) sans identifiants", nil
	case g.Type == git.Gitea && in.HTMLURL == nil && g.ID == "":
		return "html_url requis pour Gitea/Forgejo", nil
	}
	if in.PrivateKey != nil && strings.TrimSpace(*in.PrivateKey) != "" {
		if err := git.ValidateAppKey(*in.PrivateKey); err != nil {
			return err.Error(), nil
		}
	}
	return "", nil
}

func (s *Server) createSource(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "source.manage", rbac.Scope{}) {
		return
	}
	var in sourceInput
	if !decode(w, r, &in) {
		return
	}
	if !slices.Contains(git.Types, in.Type) {
		writeError(w, http.StatusUnprocessableEntity, "type : github, gitlab ou gitea")
		return
	}
	g := &store.GitSource{TeamID: teamFrom(r.Context()).Team.ID, Type: in.Type}
	msg, err := s.applySource(&in, g)
	if err != nil {
		s.internal(w, err)
		return
	}
	if msg == "" && in.Type == git.GitHub && (g.AppID == 0 || g.PrivateKeyEnc == "" || g.WebhookSecretEnc == "") {
		msg = "GitHub App : app_id, private_key et webhook_secret requis (ou utilisez la création automatique)"
	}
	if msg == "" && in.Type != git.GitHub && g.AccessTokenEnc == "" {
		msg = "access_token requis"
	}
	if msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	created, err := s.Store.CreateGitSource(r.Context(), g)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "source.created", "git_source", created.ID, map[string]string{"type": created.Type, "name": created.Name})
	writeJSON(w, http.StatusCreated, sourceView(created))
}

func (s *Server) teamSource(w http.ResponseWriter, r *http.Request, perm string) (*store.GitSource, bool) {
	if !s.authorize(w, r, perm, rbac.Scope{}) {
		return nil, false
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	g, err := s.Store.GitSourceByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return g, true
}

func (s *Server) getSource(w http.ResponseWriter, r *http.Request) {
	if g, ok := s.teamSource(w, r, "source.read"); ok {
		writeJSON(w, http.StatusOK, sourceView(g))
	}
}

func (s *Server) updateSource(w http.ResponseWriter, r *http.Request) {
	g, ok := s.teamSource(w, r, "source.manage")
	if !ok {
		return
	}
	var in sourceInput
	if !decode(w, r, &in) {
		return
	}
	in.Type = g.Type
	msg, err := s.applySource(&in, g)
	if err != nil {
		s.internal(w, err)
		return
	}
	if msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if err := s.Store.UpdateGitSource(r.Context(), g); err != nil {
		s.fail(w, err)
		return
	}
	git.ForgetToken(g.ID)
	s.auditTeam(r, "source.updated", "git_source", g.ID, map[string]any{"name": g.Name, "secrets_changed": in.PrivateKey != nil || in.AccessToken != nil || in.WebhookSecret != nil})
	writeJSON(w, http.StatusOK, sourceView(g))
}

func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request) {
	g, ok := s.teamSource(w, r, "source.manage")
	if !ok {
		return
	}
	if err := s.Store.DeleteGitSource(r.Context(), g.TeamID, g.ID); err != nil {
		s.inUse(w, err, "des applications utilisent encore cette source")
		return
	}
	git.ForgetToken(g.ID)
	s.auditTeam(r, "source.deleted", "git_source", g.ID, map[string]string{"name": g.Name})
	w.WriteHeader(http.StatusNoContent)
}

// forgeError traduit une erreur de forge en réponse 502 (sans détails internes).
func (s *Server) forgeError(w http.ResponseWriter, err error) {
	var apiErr *git.APIError
	if errors.As(err, &apiErr) {
		s.Log.Warn("forge", "status", apiErr.Status, "body", apiErr.Body)
	}
	writeError(w, http.StatusBadGateway, "échec de l'appel à la forge : "+err.Error())
}

func (s *Server) listRepositories(w http.ResponseWriter, r *http.Request) {
	g, ok := s.teamSource(w, r, "source.read")
	if !ok {
		return
	}
	src, err := s.gitSource(g)
	if err != nil {
		s.internal(w, err)
		return
	}
	repos, err := git.Repos(r.Context(), src)
	if err != nil {
		s.forgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, repos)
}

func (s *Server) listBranches(w http.ResponseWriter, r *http.Request) {
	g, ok := s.teamSource(w, r, "source.read")
	if !ok {
		return
	}
	src, err := s.gitSource(g)
	if err != nil {
		s.internal(w, err)
		return
	}
	branches, err := git.Branches(r.Context(), src, r.URL.Query().Get("repo"))
	if err != nil {
		s.forgeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, branches)
}

// --- Création d'une GitHub App par manifeste ---

func (s *Server) newState(g *store.GitSource) string {
	tok := auth.NewToken("nst_")
	exp := time.Now().Add(manifestStateTTL)
	g.StateHash, g.StateExpiresAt = auth.HashToken(tok), &exp
	return tok
}

// startGitHubManifest crée une source GitHub en attente et renvoie le formulaire à soumettre à GitHub.
func (s *Server) startGitHubManifest(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "source.manage", rbac.Scope{}) {
		return
	}
	var in struct {
		Name         string `json:"name"`
		Organization string `json:"organization"`
		HTMLURL      string `json:"html_url"`
		IsPublic     bool   `json:"is_public"`
	}
	if !decode(w, r, &in) {
		return
	}
	g := &store.GitSource{TeamID: teamFrom(r.Context()).Team.ID, Type: git.GitHub}
	si := sourceInput{Name: &in.Name, Organization: &in.Organization, IsPublic: &in.IsPublic}
	if in.HTMLURL != "" {
		si.HTMLURL = &in.HTMLURL
	}
	msg, err := s.applySource(&si, g)
	if err != nil {
		s.internal(w, err)
		return
	}
	if msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	state := s.newState(g)
	created, err := s.Store.CreateGitSource(r.Context(), g)
	if err != nil {
		s.fail(w, err)
		return
	}
	appName := strings.Trim(slugRe.ReplaceAllString(strings.ToLower("nodra-"+in.Name), "-"), "-")
	if len(appName) > 28 {
		appName = appName[:28]
	}
	appName = fmt.Sprintf("%s-%04d", appName, rand.IntN(10000))
	action := g.HTMLURL + "/settings/apps/new"
	if g.Organization != "" {
		action = g.HTMLURL + "/organizations/" + url.PathEscape(g.Organization) + "/settings/apps/new"
	}
	manifest, _ := json.Marshal(git.Manifest(appName, s.baseURL(r), created.ID, in.IsPublic))
	s.auditTeam(r, "source.github_app.started", "git_source", created.ID, map[string]string{"app_name": appName})
	writeJSON(w, http.StatusCreated, map[string]any{"source": sourceView(created), "form_action": action + "?state=" + url.QueryEscape(state),
		"manifest": string(manifest)})
}

// sourceForState charge la source d'un flux en cours et vérifie que l'utilisateur peut la gérer.
func (s *Server) sourceForState(w http.ResponseWriter, r *http.Request) (*store.GitSource, *http.Request, bool) {
	g, err := s.Store.GitSourceByState(r.Context(), auth.HashToken(r.URL.Query().Get("state")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "lien expiré ou invalide : recommencez la création de la GitHub App")
		return nil, nil, false
	}
	r2, err := s.enterTeam(r, g.TeamID)
	if err != nil {
		s.fail(w, err)
		return nil, nil, false
	}
	if !s.authorize(w, r2, "source.manage", rbac.Scope{}) {
		return nil, nil, false
	}
	return g, r2, true
}

// githubCallback : GitHub redirige ici après la création de l'App (code à convertir en identifiants).
func (s *Server) githubCallback(w http.ResponseWriter, r *http.Request) {
	g, r2, ok := s.sourceForState(w, r)
	if !ok {
		return
	}
	creds, err := git.ConvertManifest(r.Context(), g.APIURL, r.URL.Query().Get("code"))
	if err != nil {
		s.forgeError(w, err)
		return
	}
	g.AppID, g.AppSlug, g.ClientID = creds.ID, creds.Slug, creds.ClientID
	for dst, v := range map[*string]string{&g.ClientSecretEnc: creds.ClientSecret, &g.WebhookSecretEnc: creds.WebhookSecret, &g.PrivateKeyEnc: creds.PEM} {
		if *dst, err = s.Box.Encrypt(v); err != nil {
			s.internal(w, err)
			return
		}
	}
	if g.Organization == "" {
		g.Organization = creds.Owner.Login
	}
	state := s.newState(g)
	if err := s.Store.UpdateGitSource(r.Context(), g); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r2, &g.TeamID, "source.github_app.created", "git_source", g.ID, map[string]any{"app_id": g.AppID, "slug": g.AppSlug})
	install := g.HTMLURL + "/apps/" + url.PathEscape(g.AppSlug) + "/installations/new"
	if g.HTMLURL != "https://github.com" {
		install = g.HTMLURL + "/github-apps/" + url.PathEscape(g.AppSlug) + "/installations/new"
	}
	http.Redirect(w, r, install+"?state="+url.QueryEscape(state), http.StatusFound)
}

// githubInstall : GitHub redirige ici après l'installation de l'App sur un compte ou une organisation.
func (s *Server) githubInstall(w http.ResponseWriter, r *http.Request) {
	g, r2, ok := s.sourceForState(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("installation_id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "installation_id manquant")
		return
	}
	g.InstallationID, g.StateHash, g.StateExpiresAt = id, "", nil
	if err := s.Store.UpdateGitSource(r.Context(), g); err != nil {
		s.internal(w, err)
		return
	}
	git.ForgetToken(g.ID)
	s.audit(r2, &g.TeamID, "source.github_app.installed", "git_source", g.ID, map[string]any{"installation_id": id})
	http.Redirect(w, r, "/sources/"+g.ID+"?installed=1", http.StatusFound)
}

// --- Événements Git ---

// GitEventHandler traite un événement (push, pull request) pour les applications concernées.
// Implémenté par le module de déploiement.
type GitEventHandler interface {
	HandleGitEvent(ctx context.Context, src *store.GitSource, ev *git.Event) (any, error)
}
