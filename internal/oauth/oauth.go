// Package oauth : connexion par fournisseur OAuth 2 / OpenID Connect (GitHub, GitLab, Google,
// Microsoft, Discord, OIDC générique), avec PKCE. Seule l'identité (sujet, e-mail vérifié, nom) est lue.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Provider : fournisseur proposé à la connexion.
type Provider struct {
	Key  string `json:"provider"`
	Name string `json:"name"`
}

var Providers = []Provider{
	{"github", "GitHub"}, {"gitlab", "GitLab"}, {"google", "Google"},
	{"microsoft", "Microsoft"}, {"discord", "Discord"}, {"oidc", "OpenID Connect"},
}

// Name renvoie le nom affiché d'un fournisseur ("" s'il est inconnu).
func Name(key string) string {
	for _, p := range Providers {
		if p.Key == key {
			return p.Name
		}
	}
	return ""
}

// Config : application OAuth déclarée chez le fournisseur.
type Config struct {
	Provider     string
	ClientID     string
	ClientSecret string
	BaseURL      string // GitHub Enterprise, GitLab auto-hébergé, émetteur OIDC
	Tenant       string // Microsoft (common par défaut)
	HTTP         *http.Client
}

// Identity : compte chez le fournisseur.
type Identity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// Validate vérifie la configuration d'un fournisseur.
func (c Config) Validate() error {
	if Name(c.Provider) == "" {
		return errors.New("fournisseur inconnu")
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return errors.New("client_id et client_secret requis")
	}
	if c.Provider == "oidc" && c.BaseURL == "" {
		return errors.New("base_url (émetteur OIDC) requis")
	}
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return errors.New("base_url : URL http(s) attendue")
		}
	}
	return nil
}

type endpoints struct {
	auth, token, userinfo string
	scopes                string
}

func (c Config) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (c Config) base(def string) string {
	if c.BaseURL != "" {
		return strings.TrimSuffix(c.BaseURL, "/")
	}
	return def
}

func (c Config) endpoints(ctx context.Context) (endpoints, error) {
	switch c.Provider {
	case "github":
		web, api := "https://github.com", "https://api.github.com"
		if c.BaseURL != "" { // GitHub Enterprise
			web = c.base("")
			api = web + "/api/v3"
		}
		return endpoints{web + "/login/oauth/authorize", web + "/login/oauth/access_token", api, "read:user user:email"}, nil
	case "gitlab":
		b := c.base("https://gitlab.com")
		return endpoints{b + "/oauth/authorize", b + "/oauth/token", b + "/oauth/userinfo", "openid profile email"}, nil
	case "google":
		return endpoints{"https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token",
			"https://openidconnect.googleapis.com/v1/userinfo", "openid email profile"}, nil
	case "microsoft":
		tenant := c.Tenant
		if tenant == "" {
			tenant = "common"
		}
		b := c.base("https://login.microsoftonline.com") + "/" + url.PathEscape(tenant) + "/oauth2/v2.0"
		return endpoints{b + "/authorize", b + "/token", "https://graph.microsoft.com/oidc/userinfo", "openid email profile"}, nil
	case "discord":
		return endpoints{"https://discord.com/oauth2/authorize", "https://discord.com/api/oauth2/token",
			"https://discord.com/api/users/@me", "identify email"}, nil
	case "oidc":
		var d struct {
			Auth     string `json:"authorization_endpoint"`
			Token    string `json:"token_endpoint"`
			UserInfo string `json:"userinfo_endpoint"`
		}
		if err := c.getJSON(ctx, c.base("")+"/.well-known/openid-configuration", "", &d); err != nil {
			return endpoints{}, fmt.Errorf("découverte OIDC : %w", err)
		}
		if d.Auth == "" || d.Token == "" || d.UserInfo == "" {
			return endpoints{}, errors.New("découverte OIDC incomplète")
		}
		return endpoints{d.Auth, d.Token, d.UserInfo, "openid email profile"}, nil
	}
	return endpoints{}, errors.New("fournisseur inconnu")
}

// PKCE renvoie un vérificateur et son défi S256.
func PKCE() (verifier, challenge string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthURL : page d'autorisation du fournisseur.
func (c Config) AuthURL(ctx context.Context, redirectURI, state, challenge string) (string, error) {
	e, err := c.endpoints(ctx)
	if err != nil {
		return "", err
	}
	q := url.Values{"client_id": {c.ClientID}, "redirect_uri": {redirectURI}, "response_type": {"code"}, "scope": {e.scopes},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	sep := "?"
	if strings.Contains(e.auth, "?") {
		sep = "&"
	}
	return e.auth + sep + q.Encode(), nil
}

// Exchange échange le code contre un jeton puis lit l'identité.
func (c Config) Exchange(ctx context.Context, redirectURI, code, verifier string) (Identity, error) {
	e, err := c.endpoints(ctx)
	if err != nil {
		return Identity{}, err
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI},
		"client_id": {c.ClientID}, "client_secret": {c.ClientSecret}, "code_verifier": {verifier}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.token, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := c.do(req, &tok); err != nil {
		return Identity{}, fmt.Errorf("échange du code : %w", err)
	}
	if tok.AccessToken == "" {
		return Identity{}, fmt.Errorf("échange du code refusé (%s)", tok.Error)
	}
	switch c.Provider {
	case "github":
		return c.github(ctx, e.userinfo, tok.AccessToken)
	case "discord":
		var u struct {
			ID         string `json:"id"`
			Email      string `json:"email"`
			Username   string `json:"username"`
			GlobalName string `json:"global_name"`
			Verified   bool   `json:"verified"`
		}
		if err := c.getJSON(ctx, e.userinfo, tok.AccessToken, &u); err != nil {
			return Identity{}, err
		}
		name := u.GlobalName
		if name == "" {
			name = u.Username
		}
		return Identity{Subject: u.ID, Email: u.Email, EmailVerified: u.Verified, Name: name}, nil
	}
	var claims map[string]any
	if err := c.getJSON(ctx, e.userinfo, tok.AccessToken, &claims); err != nil {
		return Identity{}, err
	}
	id := Identity{Subject: str(claims["sub"]), Email: str(claims["email"]), Name: str(claims["name"])}
	switch v := claims["email_verified"].(type) { // booléen, ou chaîne chez certains fournisseurs
	case bool:
		id.EmailVerified = v
	case string:
		id.EmailVerified, _ = strconv.ParseBool(v)
	}
	if id.Name == "" {
		id.Name = str(claims["preferred_username"])
	}
	if id.Subject == "" {
		return Identity{}, errors.New("identité sans sujet")
	}
	return id, nil
}

func (c Config) github(ctx context.Context, api, token string) (Identity, error) {
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := c.getJSON(ctx, api+"/user", token, &u); err != nil {
		return Identity{}, err
	}
	if u.ID == 0 {
		return Identity{}, errors.New("identité GitHub invalide")
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := c.getJSON(ctx, api+"/user/emails", token, &emails); err != nil {
		return Identity{}, err
	}
	id := Identity{Subject: strconv.FormatInt(u.ID, 10), Name: u.Name}
	if id.Name == "" {
		id.Name = u.Login
	}
	for _, e := range emails {
		if e.Primary {
			id.Email, id.EmailVerified = e.Email, e.Verified
		}
	}
	return id, nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func (c Config) getJSON(ctx context.Context, u, token string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return c.do(req, v)
}

// do envoie la requête ; le corps d'une réponse d'erreur n'est jamais renvoyé (URL configurable).
func (c Config) do(req *http.Request, v any) error {
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("réponse HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}
