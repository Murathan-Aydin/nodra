package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Murathan-Aydin/nodra/internal/auth"
	"github.com/Murathan-Aydin/nodra/internal/oauth"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Connexion par fournisseur OAuth. Le navigateur suit des redirections : succès vers `redirect`
// (chemin relatif du front, "/" par défaut), échec vers /login?oauth_error=… (ou la page de départ
// pour une liaison depuis le profil).

const oauthStateTTL = 10 * time.Minute

// safeRedirect n'accepte qu'un chemin relatif à l'instance (pas de redirection ouverte).
func safeRedirect(v string) string {
	if !strings.HasPrefix(v, "/") || strings.HasPrefix(v, "//") || strings.ContainsAny(v, "\\\r\n") {
		return "/"
	}
	return v
}

func withParam(target, key, value string) string {
	sep := "?"
	if strings.Contains(target, "?") {
		sep = "&"
	}
	return target + sep + key + "=" + url.QueryEscape(value)
}

func (s *Server) oauthRedirectURI(r *http.Request, provider string) string {
	return s.baseURL(r) + "/api/v1/auth/oauth/" + provider + "/callback"
}

// oauthConfig : fournisseur activé, secret déchiffré.
func (s *Server) oauthConfig(ctx context.Context, provider string) (oauth.Config, error) {
	p, err := s.Store.OAuthProviderByName(ctx, provider)
	if err != nil || !p.Enabled {
		return oauth.Config{}, errors.New("fournisseur de connexion indisponible")
	}
	secret, err := s.Box.Decrypt(p.ClientSecretEnc)
	if err != nil {
		return oauth.Config{}, err
	}
	return oauth.Config{Provider: p.Provider, ClientID: p.ClientID, ClientSecret: secret, BaseURL: p.BaseURL, Tenant: p.Tenant,
		HTTP: s.OAuthHTTP}, nil
}

// enabledOAuth : fournisseurs proposés sur la page de connexion.
func (s *Server) enabledOAuth(ctx context.Context) []oauth.Provider {
	out := []oauth.Provider{}
	list, err := s.Store.ListOAuthProviders(ctx)
	if err != nil {
		return out
	}
	for _, p := range list {
		if p.Enabled {
			out = append(out, oauth.Provider{Key: p.Provider, Name: oauth.Name(p.Provider)})
		}
	}
	return out
}

func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provider := chi.URLParam(r, "provider")
	st := &store.OAuthState{Provider: provider, Redirect: safeRedirect(r.URL.Query().Get("redirect"))}
	errTarget := "/login"
	if r.URL.Query().Get("link") == "1" { // liaison depuis le profil : session interactive requise
		u := userFrom(ctx)
		if u == nil || sessionFrom(ctx) == nil {
			http.Redirect(w, r, withParam(errTarget, "oauth_error", "connectez-vous pour lier un compte"), http.StatusFound)
			return
		}
		st.LinkUserID, errTarget = &u.ID, st.Redirect
	}
	cfg, err := s.oauthConfig(ctx, provider)
	if err != nil {
		http.Redirect(w, r, withParam(errTarget, "oauth_error", err.Error()), http.StatusFound)
		return
	}
	verifier, challenge := oauth.PKCE()
	st.Verifier = verifier
	state := auth.NewToken(auth.PrefixOAuth)
	if err := s.Store.CreateOAuthState(ctx, auth.HashToken(state), st, oauthStateTTL); err != nil {
		s.internal(w, err)
		return
	}
	target, err := cfg.AuthURL(ctx, s.oauthRedirectURI(r, provider), state, challenge)
	if err != nil {
		http.Redirect(w, r, withParam(errTarget, "oauth_error", "fournisseur injoignable : "+err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provider := chi.URLParam(r, "provider")
	q := r.URL.Query()
	fail := func(target, msg string) { http.Redirect(w, r, withParam(target, "oauth_error", msg), http.StatusFound) }
	st, err := s.Store.ConsumeOAuthState(ctx, auth.HashToken(q.Get("state")))
	if err != nil || st.Provider != provider {
		fail("/login", "lien de connexion invalide ou expiré : recommencez")
		return
	}
	errTarget := "/login"
	if st.LinkUserID != nil {
		errTarget = st.Redirect
	}
	if q.Get("error") != "" || q.Get("code") == "" {
		fail(errTarget, "connexion refusée par le fournisseur")
		return
	}
	cfg, err := s.oauthConfig(ctx, provider)
	if err != nil {
		fail(errTarget, err.Error())
		return
	}
	id, err := cfg.Exchange(ctx, s.oauthRedirectURI(r, provider), q.Get("code"), st.Verifier)
	if err != nil {
		s.Log.Warn("connexion OAuth", "provider", provider, "err", err)
		fail(errTarget, "connexion impossible : "+err.Error())
		return
	}
	name := oauth.Name(provider)
	if st.LinkUserID != nil {
		u := userFrom(ctx)
		if u == nil || u.ID != *st.LinkUserID {
			fail(errTarget, "session expirée : reconnectez-vous")
			return
		}
		if err := s.Store.LinkIdentity(ctx, u.ID, provider, id.Subject, id.Email); errors.Is(err, store.ErrConflict) {
			fail(errTarget, "ce compte "+name+" est déjà lié à un utilisateur, ou un compte "+name+" est déjà lié au vôtre")
			return
		} else if err != nil {
			s.internal(w, err)
			return
		}
		s.audit(r, nil, "auth.oauth.linked", "user", u.ID, map[string]string{"provider": provider})
		http.Redirect(w, r, withParam(st.Redirect, "oauth_linked", provider), http.StatusFound)
		return
	}
	userID, err := s.Store.IdentityUser(ctx, provider, id.Subject)
	if errors.Is(err, store.ErrNotFound) {
		var msg string
		if userID, msg, err = s.oauthAccount(r, provider, id); err == nil && msg != "" {
			fail("/login", msg)
			return
		}
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	u, err := s.Store.UserByID(ctx, userID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if u.DisabledAt != nil {
		s.audit(withUser(r, u), nil, "auth.login.disabled", "user", u.ID, map[string]string{"provider": provider})
		fail("/login", "compte désactivé par l'administrateur de l'instance")
		return
	}
	mfa := u.TwoFactorEnabled()
	if err := s.startSession(w, r, u.ID, mfa); err != nil {
		s.internal(w, err)
		return
	}
	if mfa {
		http.Redirect(w, r, withParam("/login/2fa", "redirect", st.Redirect), http.StatusFound)
		return
	}
	s.audit(withUser(r, u), nil, "auth.login", "user", u.ID, map[string]string{"provider": provider})
	http.Redirect(w, r, st.Redirect, http.StatusFound)
}

// oauthAccount rattache un compte externe inconnu : à l'utilisateur de même adresse si le fournisseur
// l'a vérifiée (sinon refus : prise de contrôle possible), ou à un nouvel utilisateur si les
// inscriptions sont ouvertes. msg non vide : refus à afficher.
func (s *Server) oauthAccount(r *http.Request, provider string, id oauth.Identity) (userID, msg string, err error) {
	ctx := r.Context()
	name := oauth.Name(provider)
	email := strings.TrimSpace(id.Email)
	if email == "" {
		return "", name + " n'a pas communiqué d'adresse e-mail", nil
	}
	existing, err := s.Store.UserByEmail(ctx, email)
	switch {
	case err == nil:
		if !id.EmailVerified {
			return "", "un compte existe déjà avec cette adresse : connectez-vous puis liez " + name + " depuis votre profil", nil
		}
		if err := s.Store.LinkIdentity(ctx, existing.ID, provider, id.Subject, email); errors.Is(err, store.ErrConflict) {
			return "", "un autre compte " + name + " est déjà lié à cet utilisateur", nil
		} else if err != nil {
			return "", "", err
		}
		s.audit(withUser(r, existing), nil, "auth.oauth.linked", "user", existing.ID, map[string]string{"provider": provider})
		return existing.ID, "", nil
	case !errors.Is(err, store.ErrNotFound):
		return "", "", err
	}
	settings, err := s.Store.InstanceSettings(ctx)
	if err != nil {
		return "", "", err
	}
	if n, err := s.Store.CountUsers(ctx); err != nil {
		return "", "", err
	} else if n > 0 && !settings.RegistrationEnabled {
		return "", "les inscriptions sont fermées sur cette instance", nil
	}
	u, err := s.Store.CreateUser(ctx, email, strings.TrimSpace(id.Name), "") // sans mot de passe (« mot de passe oublié » pour en créer un)
	if errors.Is(err, store.ErrConflict) {
		return "", "un compte existe déjà avec cette adresse", nil
	} else if err != nil {
		return "", "", err
	}
	if err := s.Store.LinkIdentity(ctx, u.ID, provider, id.Subject, email); err != nil {
		return "", "", err
	}
	if id.EmailVerified {
		if err := s.Store.MarkEmailVerified(ctx, u.ID); err != nil {
			return "", "", err
		}
	}
	s.audit(withUser(r, u), nil, "auth.registered", "user", u.ID, map[string]string{"provider": provider})
	if u.IsInstanceAdmin {
		if err := s.BootstrapLocalhost(ctx); err != nil {
			s.Log.Error("serveur localhost", "err", err)
		}
	}
	return u.ID, "", nil
}

// --- Comptes liés (profil) ---

func (s *Server) listIdentities(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListIdentities(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) unlinkIdentity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	provider := chi.URLParam(r, "provider")
	if u.PasswordHash == "" {
		list, err := s.Store.ListIdentities(ctx, u.ID)
		if err != nil {
			s.internal(w, err)
			return
		}
		if len(list) <= 1 {
			writeError(w, http.StatusConflict, "dernier moyen de connexion : définissez d'abord un mot de passe (mot de passe oublié)")
			return
		}
	}
	if err := s.Store.DeleteIdentity(ctx, u.ID, provider); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, nil, "auth.oauth.unlinked", "user", u.ID, map[string]string{"provider": provider})
	w.WriteHeader(http.StatusNoContent)
}

// --- Réglages des fournisseurs (administrateur de l'instance) ---

func (s *Server) listOAuthSettings(w http.ResponseWriter, r *http.Request) {
	stored, err := s.Store.ListOAuthProviders(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	byKey := map[string]*store.OAuthProvider{}
	for _, p := range stored {
		byKey[p.Provider] = p
	}
	out := []map[string]any{}
	for _, info := range oauth.Providers {
		v := map[string]any{"provider": info.Key, "name": info.Name, "enabled": false, "client_id": "", "client_secret": "",
			"base_url": "", "tenant": "", "redirect_uri": s.oauthRedirectURI(r, info.Key)}
		if p := byKey[info.Key]; p != nil {
			v["enabled"], v["client_id"], v["base_url"], v["tenant"] = p.Enabled, p.ClientID, p.BaseURL, p.Tenant
			if p.ClientSecretEnc != "" {
				v["client_secret"] = "********"
			}
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) updateOAuthSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provider := chi.URLParam(r, "provider")
	if oauth.Name(provider) == "" {
		writeError(w, http.StatusNotFound, "fournisseur inconnu")
		return
	}
	var in struct {
		Enabled      bool   `json:"enabled"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		BaseURL      string `json:"base_url"`
		Tenant       string `json:"tenant"`
	}
	if !decode(w, r, &in) {
		return
	}
	p, err := s.Store.OAuthProviderByName(ctx, provider)
	if errors.Is(err, store.ErrNotFound) {
		p = &store.OAuthProvider{Provider: provider}
	} else if err != nil {
		s.internal(w, err)
		return
	}
	secret := ""
	if p.ClientSecretEnc != "" {
		if secret, err = s.Box.Decrypt(p.ClientSecretEnc); err != nil {
			s.internal(w, err)
			return
		}
	}
	if in.ClientSecret != "" && in.ClientSecret != "********" {
		secret = in.ClientSecret
	}
	p.Enabled, p.ClientID, p.BaseURL, p.Tenant = in.Enabled, strings.TrimSpace(in.ClientID), strings.TrimSuffix(strings.TrimSpace(in.BaseURL), "/"),
		strings.TrimSpace(in.Tenant)
	cfg := oauth.Config{Provider: provider, ClientID: p.ClientID, ClientSecret: secret, BaseURL: p.BaseURL, Tenant: p.Tenant}
	if err := cfg.Validate(); err != nil && (p.Enabled || p.BaseURL != "") {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if p.ClientSecretEnc, err = s.Box.Encrypt(secret); err != nil {
		s.internal(w, err)
		return
	}
	if err := s.Store.UpsertOAuthProvider(ctx, p); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "instance.oauth.updated", "instance", provider, map[string]any{"enabled": p.Enabled})
	w.WriteHeader(http.StatusNoContent)
}
