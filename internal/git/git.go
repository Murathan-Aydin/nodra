// Package git dialogue avec les forges (GitHub App, GitLab, Gitea/Forgejo) : dépôts, branches,
// URL de clonage authentifiées, et analyse/vérification des webhooks.
package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	GitHub = "github"
	GitLab = "gitlab"
	Gitea  = "gitea"
)

var Types = []string{GitHub, GitLab, Gitea}

// Source : informations (déchiffrées) nécessaires pour parler à une forge.
type Source struct {
	ID             string
	Type           string
	APIURL         string // ex. https://api.github.com, https://gitlab.com/api/v4, https://gitea.example.com/api/v1
	HTMLURL        string // ex. https://github.com
	AppID          int64
	InstallationID int64
	PrivateKey     string // GitHub App (PEM)
	AccessToken    string // GitLab / Gitea
}

type Repo struct {
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// Repos liste les dépôts accessibles par la source.
func Repos(ctx context.Context, s Source) ([]Repo, error) {
	switch s.Type {
	case GitHub:
		return githubRepos(ctx, s)
	case GitLab:
		return gitlabRepos(ctx, s)
	case Gitea:
		return giteaRepos(ctx, s)
	}
	return nil, fmt.Errorf("type de source inconnu : %s", s.Type)
}

// Branches liste les branches d'un dépôt (owner/name).
func Branches(ctx context.Context, s Source, fullName string) ([]string, error) {
	if !validRepo(fullName) {
		return nil, errors.New("nom de dépôt invalide")
	}
	switch s.Type {
	case GitHub:
		tok, err := InstallationToken(ctx, s)
		if err != nil {
			return nil, err
		}
		return paginate(ctx, s.APIURL+"/repos/"+fullName+"/branches?per_page=100", "Bearer "+tok, func(b []byte) ([]string, error) {
			var list []struct{ Name string }
			err := json.Unmarshal(b, &list)
			return names(list, func(x struct{ Name string }) string { return x.Name }), err
		})
	case GitLab:
		return paginate(ctx, s.APIURL+"/projects/"+url.PathEscape(fullName)+"/repository/branches?per_page=100", "Bearer "+s.AccessToken,
			func(b []byte) ([]string, error) {
				var list []struct{ Name string }
				err := json.Unmarshal(b, &list)
				return names(list, func(x struct{ Name string }) string { return x.Name }), err
			})
	case Gitea:
		return paginate(ctx, s.APIURL+"/repos/"+fullName+"/branches?limit=50", "token "+s.AccessToken, func(b []byte) ([]string, error) {
			var list []struct{ Name string }
			err := json.Unmarshal(b, &list)
			return names(list, func(x struct{ Name string }) string { return x.Name }), err
		})
	}
	return nil, fmt.Errorf("type de source inconnu : %s", s.Type)
}

// CloneURL renvoie une URL HTTPS de clonage authentifiée (à ne jamais journaliser).
func CloneURL(ctx context.Context, s Source, fullName string) (string, error) {
	if !validRepo(fullName) {
		return "", errors.New("nom de dépôt invalide")
	}
	u, err := url.Parse(strings.TrimSuffix(s.HTMLURL, "/") + "/" + fullName + ".git")
	if err != nil {
		return "", err
	}
	switch s.Type {
	case GitHub:
		tok, err := InstallationToken(ctx, s)
		if err != nil {
			return "", err
		}
		u.User = url.UserPassword("x-access-token", tok)
	case GitLab, Gitea:
		u.User = url.UserPassword("oauth2", s.AccessToken)
	default:
		return "", fmt.Errorf("type de source inconnu : %s", s.Type)
	}
	return u.String(), nil
}

func validRepo(fullName string) bool {
	parts := strings.Split(fullName, "/")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.ContainsAny(p, " ?#%\\") {
			return false
		}
	}
	return true
}

func names[T any](list []T, f func(T) string) []string {
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = f(x)
	}
	return out
}

// APIError : réponse d'erreur d'une forge.
type APIError struct {
	Status int
	Body   string
}

// Error n'inclut pas le corps de la réponse : l'URL d'API étant configurable, le renvoyer
// permettrait de lire des services internes (SSRF). Le corps reste disponible pour les logs.
func (e *APIError) Error() string {
	return fmt.Sprintf("la forge a répondu HTTP %d", e.Status)
}

func do(ctx context.Context, method, rawURL, auth string, body io.Reader) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Nodra")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 300 {
		msg := string(b)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return resp, b, &APIError{Status: resp.StatusCode, Body: msg}
	}
	return resp, b, nil
}

// paginate suit les en-têtes Link (rel="next") jusqu'à 20 pages.
func paginate[T any](ctx context.Context, rawURL, auth string, parse func([]byte) ([]T, error)) ([]T, error) {
	var out []T
	for page := 0; rawURL != "" && page < 20; page++ {
		resp, b, err := do(ctx, "GET", rawURL, auth, nil)
		if err != nil {
			return nil, err
		}
		items, err := parse(b)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		rawURL = nextLink(resp.Header.Get("Link"))
	}
	return out, nil
}

func nextLink(h string) string {
	for _, part := range strings.Split(h, ",") {
		if strings.Contains(part, `rel="next"`) {
			s, e := strings.Index(part, "<"), strings.Index(part, ">")
			if s >= 0 && e > s {
				return part[s+1 : e]
			}
		}
	}
	return ""
}
