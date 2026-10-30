package git

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// AppJWT signe le jeton (RS256, 9 min) qui authentifie une GitHub App.
func AppJWT(appID int64, privateKeyPEM string, now time.Time) (string, error) {
	key, err := parseRSAKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": strconv.FormatInt(appID, 10)})
	signing := header + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + enc.EncodeToString(sig), nil
}

func parseRSAKey(p string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(p))
	if block == nil {
		return nil, errors.New("clé privée de la GitHub App invalide (PEM attendu)")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("clé privée de la GitHub App invalide")
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("la clé de la GitHub App doit être RSA")
	}
	return rk, nil
}

// ValidateAppKey vérifie qu'une clé privée de GitHub App est exploitable.
func ValidateAppKey(p string) error { _, err := parseRSAKey(p); return err }

type cachedToken struct {
	token   string
	expires time.Time
}

var (
	tokenMu    sync.Mutex
	tokenCache = map[string]cachedToken{}
)

// InstallationToken renvoie un jeton d'installation (mis en cache jusqu'à 5 min avant expiration).
func InstallationToken(ctx context.Context, s Source) (string, error) {
	if s.AppID == 0 || s.InstallationID == 0 {
		return "", errors.New("la GitHub App n'est pas installée (installation_id manquant)")
	}
	key := fmt.Sprintf("%s|%s|%d|%d", s.ID, s.APIURL, s.AppID, s.InstallationID)
	tokenMu.Lock()
	if c, ok := tokenCache[key]; ok && time.Until(c.expires) > 5*time.Minute {
		tokenMu.Unlock()
		return c.token, nil
	}
	tokenMu.Unlock()
	jwt, err := AppJWT(s.AppID, s.PrivateKey, time.Now())
	if err != nil {
		return "", err
	}
	_, b, err := do(ctx, "POST", fmt.Sprintf("%s/app/installations/%d/access_tokens", s.APIURL, s.InstallationID), "Bearer "+jwt, nil)
	if err != nil {
		return "", err
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(b, &out); err != nil || out.Token == "" {
		return "", errors.New("réponse inattendue de GitHub pour le jeton d'installation")
	}
	tokenMu.Lock()
	tokenCache[key] = cachedToken{out.Token, out.ExpiresAt}
	tokenMu.Unlock()
	return out.Token, nil
}

// ForgetToken retire un jeton du cache (source modifiée ou supprimée).
func ForgetToken(sourceID string) {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	for k := range tokenCache {
		if len(k) >= len(sourceID) && k[:len(sourceID)] == sourceID {
			delete(tokenCache, k)
		}
	}
}

func githubRepos(ctx context.Context, s Source) ([]Repo, error) {
	tok, err := InstallationToken(ctx, s)
	if err != nil {
		return nil, err
	}
	return paginate(ctx, s.APIURL+"/installation/repositories?per_page=100", "Bearer "+tok, func(b []byte) ([]Repo, error) {
		var page struct {
			Repositories []struct {
				FullName      string `json:"full_name"`
				Name          string `json:"name"`
				Private       bool   `json:"private"`
				DefaultBranch string `json:"default_branch"`
				HTMLURL       string `json:"html_url"`
			} `json:"repositories"`
		}
		if err := json.Unmarshal(b, &page); err != nil {
			return nil, err
		}
		out := make([]Repo, len(page.Repositories))
		for i, r := range page.Repositories {
			out[i] = Repo{r.FullName, r.Name, r.Private, r.DefaultBranch, r.HTMLURL}
		}
		return out, nil
	})
}

// --- Création d'une GitHub App par manifeste ---

// Manifest construit le manifeste de création d'une GitHub App pointant vers l'instance.
func Manifest(name, baseURL, sourceID string, public bool) map[string]any {
	return map[string]any{
		"name":                     name,
		"url":                      baseURL,
		"hook_attributes":          map[string]any{"url": baseURL + "/webhooks/github", "active": true},
		"redirect_url":             baseURL + "/api/v1/sources/github/callback",
		"setup_url":                baseURL + "/api/v1/sources/github/install",
		"setup_on_update":          true,
		"public":                   public,
		"request_oauth_on_install": false,
		"default_permissions": map[string]string{
			"contents": "read", "metadata": "read", "pull_requests": "write", "emails": "read", "administration": "read",
		},
		"default_events": []string{"push", "pull_request"},
	}
}

// AppCredentials : identifiants renvoyés par GitHub à la conversion d'un manifeste.
type AppCredentials struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
	PEM           string `json:"pem"`
	HTMLURL       string `json:"html_url"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// ConvertManifest échange le code reçu après création de l'App contre ses identifiants.
func ConvertManifest(ctx context.Context, apiURL, code string) (*AppCredentials, error) {
	_, b, err := do(ctx, "POST", apiURL+"/app-manifests/"+code+"/conversions", "", bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	var c AppCredentials
	if err := json.Unmarshal(b, &c); err != nil || c.ID == 0 {
		return nil, errors.New("réponse inattendue de GitHub à la conversion du manifeste")
	}
	return &c, nil
}
