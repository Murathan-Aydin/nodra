package git

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// Event : événement de webhook normalisé, quelle que soit la forge.
type Event struct {
	Provider       string   `json:"provider"`
	Kind           string   `json:"kind"`   // push | pull_request | ping | installation
	Action         string   `json:"action"` // pull_request : opened, synchronize, reopened, closed ; installation : created, deleted…
	Repo           string   `json:"repo"`   // owner/name
	CloneURL       string   `json:"clone_url"`
	Branch         string   `json:"branch"` // push : branche poussée ; pull_request : branche de base
	Commit         string   `json:"commit"`
	Deleted        bool     `json:"deleted"` // push de suppression de branche
	PRNumber       int      `json:"pr_number"`
	PRTitle        string   `json:"pr_title"`
	PRURL          string   `json:"pr_url"`
	PRBranch       string   `json:"pr_branch"` // branche source de la PR
	IsFork         bool     `json:"is_fork"`
	Sender         string   `json:"sender"`
	InstallationID int64    `json:"installation_id"`
	Files          []string `json:"files,omitempty"` // fichiers modifiés (push), pour watch_paths
}

type commitFiles []struct {
	Added    []string `json:"added"`
	Modified []string `json:"modified"`
	Removed  []string `json:"removed"`
}

func (c commitFiles) files() []string {
	seen := map[string]bool{}
	var out []string
	for _, cm := range c {
		for _, l := range [][]string{cm.Added, cm.Modified, cm.Removed} {
			for _, f := range l {
				if !seen[f] {
					seen[f] = true
					out = append(out, f)
				}
			}
		}
	}
	return out
}

var ErrSignature = errors.New("signature du webhook invalide")

// VerifyHMAC compare une signature HMAC-SHA256 hexadécimale (avec ou sans préfixe "sha256=").
func VerifyHMAC(secret, signature string, body []byte) error {
	if secret == "" {
		return ErrSignature
	}
	signature = strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(strings.ToLower(signature))) {
		return ErrSignature
	}
	return nil
}

// VerifyToken compare un jeton secret en clair (GitLab : X-Gitlab-Token).
func VerifyToken(secret, token string) error {
	if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(token)) != 1 {
		return ErrSignature
	}
	return nil
}

// Verify vérifie la signature d'un webhook selon la forge.
func Verify(provider, secret string, h http.Header, body []byte) error {
	switch provider {
	case GitHub:
		return VerifyHMAC(secret, h.Get("X-Hub-Signature-256"), body)
	case GitLab:
		return VerifyToken(secret, h.Get("X-Gitlab-Token"))
	case Gitea:
		sig := h.Get("X-Gitea-Signature")
		if sig == "" {
			sig = h.Get("X-Forgejo-Signature")
		}
		return VerifyHMAC(secret, sig, body)
	case "bitbucket":
		return VerifyHMAC(secret, h.Get("X-Hub-Signature"), body)
	}
	return ErrSignature
}

func branchFromRef(ref string) string { return strings.TrimPrefix(ref, "refs/heads/") }

// Parse analyse le corps d'un webhook. Renvoie Kind vide pour un événement ignoré.
func Parse(provider string, h http.Header, body []byte) (*Event, error) {
	switch provider {
	case GitHub:
		return parseGitHub(h.Get("X-GitHub-Event"), body)
	case Gitea:
		ev := h.Get("X-Gitea-Event")
		if ev == "" {
			ev = h.Get("X-Forgejo-Event")
		}
		e, err := parseGitHub(ev, body) // charge utile compatible GitHub
		if e != nil {
			e.Provider = Gitea
		}
		return e, err
	case GitLab:
		return parseGitLab(h.Get("X-Gitlab-Event"), body)
	case "bitbucket":
		return parseBitbucket(h.Get("X-Event-Key"), body)
	}
	return nil, errors.New("forge inconnue")
}

func parseGitHub(event string, body []byte) (*Event, error) {
	var p struct {
		Ref        string      `json:"ref"`
		After      string      `json:"after"`
		Deleted    bool        `json:"deleted"`
		Commits    commitFiles `json:"commits"`
		Action     string      `json:"action"`
		Number     int         `json:"number"`
		Repository struct {
			FullName string `json:"full_name"`
			CloneURL string `json:"clone_url"`
		} `json:"repository"`
		PullRequest struct {
			Title   string `json:"title"`
			HTMLURL string `json:"html_url"`
			Head    struct {
				Ref  string `json:"ref"`
				SHA  string `json:"sha"`
				Repo struct {
					FullName string `json:"full_name"`
				} `json:"repo"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
		} `json:"pull_request"`
		Sender struct {
			Login string `json:"login"`
		} `json:"sender"`
		Installation struct {
			ID int64 `json:"id"`
		} `json:"installation"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, errors.New("charge utile JSON invalide")
	}
	e := &Event{Provider: GitHub, Repo: p.Repository.FullName, CloneURL: p.Repository.CloneURL, Sender: p.Sender.Login,
		InstallationID: p.Installation.ID, Action: p.Action}
	switch event {
	case "push":
		if !strings.HasPrefix(p.Ref, "refs/heads/") {
			return &Event{Provider: GitHub}, nil // tags ignorés
		}
		e.Kind, e.Branch, e.Commit, e.Deleted = "push", branchFromRef(p.Ref), p.After, p.Deleted || strings.Trim(p.After, "0") == ""
		e.Files = p.Commits.files()
	case "pull_request":
		pr := p.PullRequest
		e.Kind, e.PRNumber, e.PRTitle, e.PRURL = "pull_request", p.Number, pr.Title, pr.HTMLURL
		e.Branch, e.PRBranch, e.Commit = pr.Base.Ref, pr.Head.Ref, pr.Head.SHA
		e.IsFork = pr.Head.Repo.FullName != "" && !strings.EqualFold(pr.Head.Repo.FullName, p.Repository.FullName)
	case "ping", "installation", "installation_repositories":
		e.Kind = event
	default:
		e.Kind = ""
	}
	return e, nil
}

func parseGitLab(event string, body []byte) (*Event, error) {
	var p struct {
		Ref          string      `json:"ref"`
		After        string      `json:"after"`
		Commits      commitFiles `json:"commits"`
		CheckoutSHA  string      `json:"checkout_sha"`
		UserUsername string      `json:"user_username"`
		Project      struct {
			PathWithNamespace string `json:"path_with_namespace"`
			GitHTTPURL        string `json:"git_http_url"`
		} `json:"project"`
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		ObjectAttributes struct {
			IID          int    `json:"iid"`
			Title        string `json:"title"`
			URL          string `json:"url"`
			Action       string `json:"action"`
			SourceBranch string `json:"source_branch"`
			TargetBranch string `json:"target_branch"`
			SourceProjID int    `json:"source_project_id"`
			TargetProjID int    `json:"target_project_id"`
			LastCommit   struct {
				ID string `json:"id"`
			} `json:"last_commit"`
		} `json:"object_attributes"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, errors.New("charge utile JSON invalide")
	}
	e := &Event{Provider: GitLab, Repo: p.Project.PathWithNamespace, CloneURL: p.Project.GitHTTPURL}
	switch event {
	case "Push Hook":
		e.Kind, e.Branch, e.Sender = "push", branchFromRef(p.Ref), p.UserUsername
		e.Commit, e.Deleted = p.After, strings.Trim(p.After, "0") == ""
		e.Files = p.Commits.files()
	case "Merge Request Hook":
		o := p.ObjectAttributes
		action := map[string]string{"open": "opened", "update": "synchronize", "reopen": "reopened", "close": "closed", "merge": "closed"}[o.Action]
		e.Kind, e.Action, e.PRNumber, e.PRTitle, e.PRURL = "pull_request", action, o.IID, o.Title, o.URL
		e.Branch, e.PRBranch, e.Commit, e.Sender = o.TargetBranch, o.SourceBranch, o.LastCommit.ID, p.User.Username
		e.IsFork = o.SourceProjID != o.TargetProjID
	default:
		e.Kind = ""
	}
	return e, nil
}

func parseBitbucket(event string, body []byte) (*Event, error) {
	var p struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Actor struct {
			Nickname string `json:"nickname"`
		} `json:"actor"`
		Push struct {
			Changes []struct {
				New *struct {
					Type   string `json:"type"`
					Name   string `json:"name"`
					Target struct {
						Hash string `json:"hash"`
					} `json:"target"`
				} `json:"new"`
				Old *struct {
					Name string `json:"name"`
				} `json:"old"`
			} `json:"changes"`
		} `json:"push"`
		PullRequest struct {
			ID     int                                  `json:"id"`
			Title  string                               `json:"title"`
			Links  struct{ HTML struct{ Href string } } `json:"links"`
			Source struct {
				Branch     struct{ Name string } `json:"branch"`
				Commit     struct{ Hash string } `json:"commit"`
				Repository struct {
					FullName string `json:"full_name"`
				} `json:"repository"`
			} `json:"source"`
			Destination struct {
				Branch struct{ Name string } `json:"branch"`
			} `json:"destination"`
		} `json:"pullrequest"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, errors.New("charge utile JSON invalide")
	}
	e := &Event{Provider: "bitbucket", Repo: p.Repository.FullName, Sender: p.Actor.Nickname,
		CloneURL: "https://bitbucket.org/" + p.Repository.FullName + ".git"}
	switch {
	case event == "repo:push" && len(p.Push.Changes) > 0:
		c := p.Push.Changes[0]
		if c.New == nil {
			if c.Old != nil {
				e.Kind, e.Branch, e.Deleted = "push", c.Old.Name, true
			}
			return e, nil
		}
		if c.New.Type != "branch" {
			return &Event{Provider: "bitbucket"}, nil
		}
		e.Kind, e.Branch, e.Commit = "push", c.New.Name, c.New.Target.Hash
	case strings.HasPrefix(event, "pullrequest:"):
		pr := p.PullRequest
		action := map[string]string{"pullrequest:created": "opened", "pullrequest:updated": "synchronize",
			"pullrequest:fulfilled": "closed", "pullrequest:rejected": "closed"}[event]
		e.Kind, e.Action, e.PRNumber, e.PRTitle, e.PRURL = "pull_request", action, pr.ID, pr.Title, pr.Links.HTML.Href
		e.Branch, e.PRBranch, e.Commit = pr.Destination.Branch.Name, pr.Source.Branch.Name, pr.Source.Commit.Hash
		e.IsFork = pr.Source.Repository.FullName != "" && !strings.EqualFold(pr.Source.Repository.FullName, p.Repository.FullName)
	}
	return e, nil
}

// MatchWatchPaths indique si un push concerne les chemins surveillés (un motif par ligne, ** autorisé,
// ! pour exclure). Sans motif, ou si la forge ne fournit pas la liste des fichiers, tout push compte.
func MatchWatchPaths(patterns string, files []string) bool {
	var include, exclude []*regexp.Regexp
	for _, l := range strings.Split(patterns, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if neg, ok := strings.CutPrefix(l, "!"); ok {
			exclude = append(exclude, globRe(neg))
		} else {
			include = append(include, globRe(l))
		}
	}
	if len(include) == 0 && len(exclude) == 0 || len(files) == 0 {
		return true
	}
	for _, f := range files {
		if slices.ContainsFunc(exclude, func(r *regexp.Regexp) bool { return r.MatchString(f) }) {
			continue
		}
		if len(include) == 0 || slices.ContainsFunc(include, func(r *regexp.Regexp) bool { return r.MatchString(f) }) {
			return true
		}
	}
	return false
}

func globRe(glob string) *regexp.Regexp {
	glob = strings.TrimPrefix(glob, "/")
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++
					b.WriteString("/?")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	if strings.HasSuffix(glob, "/") {
		b.WriteString(".*")
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
