package git

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func rsaPEM(t *testing.T) (string, *rsa.PublicKey) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})), &k.PublicKey
}

func TestAppJWT(t *testing.T) {
	key, pub := rsaPEM(t)
	tok, err := AppJWT(42, key, time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatal("JWT en 3 parties")
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatal("signature RS256 invalide")
	}
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c map[string]any
	_ = json.Unmarshal(claims, &c)
	if c["iss"] != "42" || c["iat"].(float64) != 1_700_000_000-60 || c["exp"].(float64) != 1_700_000_000+540 {
		t.Fatalf("revendications : %v", c)
	}
	if ValidateAppKey("pas une clé") == nil {
		t.Fatal("clé invalide acceptée")
	}
}

// fakeGitHub simule l'API GitHub (jeton d'installation, dépôts paginés, branches, conversion de manifeste).
func fakeGitHub(t *testing.T, tokenCalls *atomic.Int32) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/app/installations/7/access_tokens":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ey") {
				w.WriteHeader(401)
				return
			}
			tokenCalls.Add(1)
			fmt.Fprintf(w, `{"token":"ghs_installation","expires_at":%q}`, time.Now().Add(time.Hour).Format(time.RFC3339))
		case r.URL.Path == "/installation/repositories":
			if r.Header.Get("Authorization") != "Bearer ghs_installation" {
				w.WriteHeader(401)
				return
			}
			if r.URL.Query().Get("page") == "" {
				w.Header().Set("Link", fmt.Sprintf(`<%s/installation/repositories?per_page=100&page=2>; rel="next"`, srv.URL))
				fmt.Fprint(w, `{"repositories":[{"full_name":"acme/web","name":"web","private":true,"default_branch":"main"}]}`)
				return
			}
			fmt.Fprint(w, `{"repositories":[{"full_name":"acme/api","name":"api","default_branch":"master"}]}`)
		case r.URL.Path == "/repos/acme/web/branches":
			fmt.Fprint(w, `[{"name":"main"},{"name":"dev"}]`)
		case r.Method == "POST" && r.URL.Path == "/repos/acme/web/issues/12/comments":
			fmt.Fprint(w, `{"id":555}`)
		case r.Method == "PATCH" && r.URL.Path == "/repos/acme/web/issues/comments/555":
			fmt.Fprint(w, `{"id":555}`)
		case r.Method == "POST" && r.URL.Path == "/app-manifests/CODE/conversions":
			fmt.Fprint(w, `{"id":99,"slug":"nodra-test","client_id":"Iv1","client_secret":"cs","webhook_secret":"ws","pem":"PEM","owner":{"login":"acme"}}`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGitHubApp(t *testing.T) {
	var calls atomic.Int32
	api := fakeGitHub(t, &calls)
	key, _ := rsaPEM(t)
	src := Source{ID: "s1", Type: GitHub, APIURL: api.URL, HTMLURL: "https://github.com", AppID: 42, InstallationID: 7, PrivateKey: key}
	ctx := context.Background()
	repos, err := Repos(ctx, src)
	if err != nil || len(repos) != 2 || repos[0].FullName != "acme/web" || !repos[0].Private || repos[1].DefaultBranch != "master" {
		t.Fatalf("dépôts : %+v %v", repos, err)
	}
	branches, err := Branches(ctx, src, "acme/web")
	if err != nil || strings.Join(branches, ",") != "main,dev" {
		t.Fatalf("branches : %v %v", branches, err)
	}
	u, err := CloneURL(ctx, src, "acme/web")
	if err != nil || u != "https://x-access-token:ghs_installation@github.com/acme/web.git" {
		t.Fatalf("URL de clonage : %s %v", u, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("le jeton d'installation doit être mis en cache (%d appels)", calls.Load())
	}
	ForgetToken("s1")
	_, _ = Repos(ctx, src)
	if calls.Load() != 2 {
		t.Fatal("ForgetToken doit vider le cache")
	}
	if id, err := CommentPR(ctx, src, "acme/web", 12, 0, "preview"); err != nil || id != 555 {
		t.Fatalf("commentaire de PR : %d %v", id, err)
	}
	if id, err := CommentPR(ctx, src, "acme/web", 12, 555, "preview"); err != nil || id != 555 {
		t.Fatalf("mise à jour du commentaire : %d %v", id, err)
	}
	if _, err := CommentPR(ctx, src, "acme/web", 12, 556, "x"); err == nil {
		t.Fatal("commentaire inconnu")
	}
	if _, err := Branches(ctx, src, "../etc"); err == nil {
		t.Fatal("nom de dépôt invalide accepté")
	}
	src.InstallationID = 0
	if _, err := Repos(ctx, src); err == nil {
		t.Fatal("App non installée")
	}
	creds, err := ConvertManifest(ctx, api.URL, "CODE")
	if err != nil || creds.ID != 99 || creds.WebhookSecret != "ws" || creds.Owner.Login != "acme" {
		t.Fatalf("conversion : %+v %v", creds, err)
	}
	m := Manifest("nodra-x", "https://nodra.example.com", "id", false)
	if m["hook_attributes"].(map[string]any)["url"] != "https://nodra.example.com/webhooks/github" {
		t.Fatal("URL de webhook du manifeste")
	}
}

func TestGitLabGitea(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/projects" && r.Header.Get("Authorization") == "Bearer glpat":
			fmt.Fprint(w, `[{"path_with_namespace":"grp/app","path":"app","visibility":"private","default_branch":"main"}]`)
		case r.URL.RawPath == "/api/v4/projects/grp%2Fapp/repository/branches":
			fmt.Fprint(w, `[{"name":"main"}]`)
		case r.URL.Path == "/api/v1/user/repos" && r.Header.Get("Authorization") == "token gt":
			fmt.Fprint(w, `[{"full_name":"me/site","name":"site","private":false,"default_branch":"main"}]`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	gl := Source{Type: GitLab, APIURL: srv.URL + "/api/v4", HTMLURL: "https://gitlab.com", AccessToken: "glpat"}
	repos, err := Repos(ctx, gl)
	if err != nil || len(repos) != 1 || repos[0].FullName != "grp/app" || !repos[0].Private {
		t.Fatalf("gitlab : %+v %v", repos, err)
	}
	if b, err := Branches(ctx, gl, "grp/app"); err != nil || len(b) != 1 {
		t.Fatalf("branches gitlab : %v %v", b, err)
	}
	if u, _ := CloneURL(ctx, gl, "grp/app"); u != "https://oauth2:glpat@gitlab.com/grp/app.git" {
		t.Fatalf("clone gitlab : %s", u)
	}
	gt := Source{Type: Gitea, APIURL: srv.URL + "/api/v1", HTMLURL: "https://git.example.com", AccessToken: "gt"}
	if repos, err := Repos(ctx, gt); err != nil || repos[0].FullName != "me/site" {
		t.Fatalf("gitea : %+v %v", repos, err)
	}
	gt.AccessToken = "faux"
	if _, err := Repos(ctx, gt); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("erreur d'API attendue : %v", err)
	}
}

func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func TestWebhooks(t *testing.T) {
	push := []byte(`{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"acme/web","clone_url":"https://github.com/acme/web.git"},"sender":{"login":"bob"},"installation":{"id":7}}`)
	h := http.Header{"X-Github-Event": {"push"}, "X-Hub-Signature-256": {"sha256=" + sign("s3", push)}}
	if Verify(GitHub, "s3", h, push) != nil || Verify(GitHub, "autre", h, push) == nil || Verify(GitHub, "", h, push) == nil {
		t.Fatal("vérification HMAC GitHub")
	}
	e, err := Parse(GitHub, h, push)
	if err != nil || e.Kind != "push" || e.Branch != "main" || e.Commit != "abc123" || e.Repo != "acme/web" || e.InstallationID != 7 || e.Deleted {
		t.Fatalf("push GitHub : %+v %v", e, err)
	}
	tag, _ := Parse(GitHub, http.Header{"X-Github-Event": {"push"}}, []byte(`{"ref":"refs/tags/v1"}`))
	if tag.Kind != "" {
		t.Fatal("les tags sont ignorés")
	}
	del, _ := Parse(GitHub, http.Header{"X-Github-Event": {"push"}}, []byte(`{"ref":"refs/heads/old","after":"0000000000000000000000000000000000000000","deleted":true}`))
	if !del.Deleted {
		t.Fatal("suppression de branche")
	}
	pr := []byte(`{"action":"opened","number":12,"repository":{"full_name":"acme/web"},"pull_request":{"title":"Feature","html_url":"https://github.com/acme/web/pull/12","head":{"ref":"feat","sha":"def","repo":{"full_name":"fork/web"}},"base":{"ref":"main"}}}`)
	e, _ = Parse(GitHub, http.Header{"X-Github-Event": {"pull_request"}}, pr)
	if e.Kind != "pull_request" || e.Action != "opened" || e.PRNumber != 12 || e.Branch != "main" || e.PRBranch != "feat" || !e.IsFork {
		t.Fatalf("PR GitHub : %+v", e)
	}

	gl := []byte(`{"object_kind":"merge_request","user":{"username":"ann"},"project":{"path_with_namespace":"grp/app"},"object_attributes":{"iid":3,"title":"MR","action":"update","source_branch":"feat","target_branch":"main","source_project_id":1,"target_project_id":1,"last_commit":{"id":"c1"}}}`)
	glh := http.Header{"X-Gitlab-Event": {"Merge Request Hook"}, "X-Gitlab-Token": {"tok"}}
	if Verify(GitLab, "tok", glh, gl) != nil || Verify(GitLab, "x", glh, gl) == nil {
		t.Fatal("jeton GitLab")
	}
	e, _ = Parse(GitLab, glh, gl)
	if e.Kind != "pull_request" || e.Action != "synchronize" || e.PRNumber != 3 || e.IsFork || e.Commit != "c1" {
		t.Fatalf("MR GitLab : %+v", e)
	}
	e, _ = Parse(GitLab, http.Header{"X-Gitlab-Event": {"Push Hook"}}, []byte(`{"ref":"refs/heads/dev","after":"c2","user_username":"ann","project":{"path_with_namespace":"grp/app"}}`))
	if e.Kind != "push" || e.Branch != "dev" || e.Sender != "ann" {
		t.Fatalf("push GitLab : %+v", e)
	}

	gtb := []byte(`{"ref":"refs/heads/main","after":"g1","repository":{"full_name":"me/site"}}`)
	gth := http.Header{"X-Gitea-Event": {"push"}, "X-Gitea-Signature": {sign("gs", gtb)}}
	if Verify(Gitea, "gs", gth, gtb) != nil {
		t.Fatal("signature Gitea")
	}
	if e, _ = Parse(Gitea, gth, gtb); e.Provider != Gitea || e.Branch != "main" {
		t.Fatalf("push Gitea : %+v", e)
	}

	bb := []byte(`{"repository":{"full_name":"team/repo"},"actor":{"nickname":"zed"},"push":{"changes":[{"new":{"type":"branch","name":"main","target":{"hash":"b1"}}}]}}`)
	bbh := http.Header{"X-Event-Key": {"repo:push"}, "X-Hub-Signature": {"sha256=" + sign("bs", bb)}}
	if Verify("bitbucket", "bs", bbh, bb) != nil {
		t.Fatal("signature Bitbucket")
	}
	if e, _ = Parse("bitbucket", bbh, bb); e.Kind != "push" || e.Commit != "b1" || e.Repo != "team/repo" {
		t.Fatalf("push Bitbucket : %+v", e)
	}
}

func TestWatchPaths(t *testing.T) {
	cases := []struct {
		patterns string
		files    []string
		want     bool
	}{
		{"", []string{"README.md"}, true},
		{"src/**", []string{"src/app/main.go"}, true},
		{"src/**", []string{"docs/a.md"}, false},
		{"*.go", []string{"main.go"}, true},
		{"*.go", []string{"pkg/main.go"}, false},
		{"**/*.go", []string{"pkg/main.go", "main.go"}, true},
		{"!docs/**", []string{"docs/a.md"}, false},
		{"!docs/**", []string{"docs/a.md", "src/x"}, true},
		{"api/\n!api/README.md", []string{"api/README.md"}, false},
		{"api/", []string{"api/v1/x.go"}, true},
		{"src/**", nil, true},
	}
	for _, c := range cases {
		if got := MatchWatchPaths(c.patterns, c.files); got != c.want {
			t.Errorf("%q %v : %v, attendu %v", c.patterns, c.files, got, c.want)
		}
	}
	push := []byte(`{"ref":"refs/heads/main","after":"a","commits":[{"added":["a.go"],"modified":["b.go"]},{"modified":["a.go"],"removed":["c.go"]}]}`)
	e, _ := Parse(GitHub, http.Header{"X-Github-Event": {"push"}}, push)
	if strings.Join(e.Files, ",") != "a.go,b.go,c.go" {
		t.Fatalf("fichiers : %v", e.Files)
	}
}
