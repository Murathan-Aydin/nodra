package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestGitWebhooks(t *testing.T) {
	f := newAppFixture(t)
	h := newFakeHost(t, f)
	alice, base := f.alice, f.base
	f.e.api.Executors["resource.deploy"] = f.e.api.execDeploy

	var app map[string]any
	expect(t, "application", alice.do("POST", base+"/applications", f.app(map[string]any{"domains": []string{"https://site.example.com"},
		"watch_paths": "src/**\n!src/**/*.md", "settings": map[string]any{"preview_deployments": true}}), &app), 201)
	appID := app["id"].(string)
	var hooks struct {
		Secret string            `json:"secret"`
		URLs   map[string]string `json:"urls"`
	}
	expect(t, "webhooks", alice.do("GET", base+"/applications/"+appID+"/webhooks", nil, &hooks), 200)

	type result = GitEventResult
	send := func(event, secret string, payload map[string]any) (int, []result) {
		t.Helper()
		payload["repository"] = map[string]string{"full_name": "Acme/Site", "clone_url": "https://github.com/acme/site.git"}
		body, _ := json.Marshal(payload)
		m := hmac.New(sha256.New, []byte(secret))
		m.Write(body)
		req, _ := http.NewRequest("POST", f.e.srv.URL+"/webhooks/github/manual", strings.NewReader(string(body)))
		req.Header.Set("X-GitHub-Event", event)
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(m.Sum(nil)))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out []result
		b, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(b, &out)
		return resp.StatusCode, out
	}
	sha := "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	h.set("SHA", sha) // commit renvoyé par le faux git après checkout
	push := func(branch string, files ...string) map[string]any {
		return map[string]any{"ref": "refs/heads/" + branch, "after": sha, "commits": []map[string]any{{"modified": files}}}
	}
	one := func(step string, status int, res []result, action string) result {
		t.Helper()
		if status != 200 || len(res) != 1 || res[0].Action != action {
			t.Fatalf("%s : %d %+v (attendu %s)", step, status, res, action)
		}
		return res[0]
	}

	// Signature et filtrage.
	if st, _ := send("push", "mauvais", push("main", "src/a.go")); st != 401 {
		t.Fatalf("mauvaise signature : %d", st)
	}
	st, res := send("push", hooks.Secret, push("main", "docs/a.md", "src/README.md"))
	if r := one("watch_paths", st, res, "skipped"); !strings.Contains(r.Reason, "watch_paths") {
		t.Fatalf("raison : %s", r.Reason)
	}
	st, res = send("push", hooks.Secret, push("dev", "src/a.go"))
	one("autre branche", st, res, "skipped")
	st, res = send("push", hooks.Secret, map[string]any{"ref": "refs/heads/main", "after": strings.Repeat("0", 40), "deleted": true})
	one("suppression", st, res, "skipped")

	// Push sur la branche suivie : déploiement du commit poussé.
	st, res = send("push", hooks.Secret, push("main", "src/a.go"))
	r := one("push", st, res, "deploy")
	d := waitDeployment(t, alice, base, appID, r.DeploymentID)
	if d.Status != "finished" || d.Trigger != "webhook" || d.CommitSHA != sha {
		t.Fatalf("déploiement par webhook : %+v", d)
	}

	// Déploiement automatique désactivé.
	expect(t, "auto_deploy", alice.do("PATCH", base+"/applications/"+appID, map[string]any{"settings": map[string]any{"auto_deploy": false}}, nil), 200)
	st, res = send("push", hooks.Secret, push("main", "src/a.go"))
	one("auto_deploy désactivé", st, res, "skipped")

	// Pull requests : fork refusé par défaut, puis preview déployée et supprimée à la fermeture.
	pr := func(action, headRepo string) map[string]any {
		return map[string]any{"action": action, "number": 12, "pull_request": map[string]any{"title": "Nouvelle page",
			"html_url": "https://github.com/acme/site/pull/12", "head": map[string]any{"ref": "feature", "sha": sha,
				"repo": map[string]string{"full_name": headRepo}}, "base": map[string]string{"ref": "main"}}}
	}
	st, res = send("pull_request", hooks.Secret, pr("opened", "mallory/site"))
	one("fork", st, res, "skipped")
	st, res = send("pull_request", hooks.Secret, pr("opened", "acme/site"))
	r = one("PR ouverte", st, res, "preview_deploy")
	d = waitDeployment(t, alice, base, appID, r.DeploymentID)
	if d.Status != "finished" || d.PRNumber != 12 || d.Trigger != "preview" {
		t.Fatalf("preview : %+v\n%s", d, depLogs(t, alice, base, appID, d.ID, true))
	}
	if !strings.Contains(h.calls(), "git fetch -q --depth 1 origin pull/12/head") {
		t.Fatalf("la PR doit être récupérée :\n%s", h.calls())
	}
	var previews []store.Preview
	expect(t, "previews", alice.do("GET", base+"/applications/"+appID+"/previews", nil, &previews), 200)
	if len(previews) != 1 || previews[0].Status != "running" || !slices.Equal(previews[0].Domains, []string{"https://12.site.example.com"}) ||
		previews[0].Title != "Nouvelle page" {
		t.Fatalf("previews : %+v", previews)
	}
	if !slices.ContainsFunc(h.containers(), func(c string) bool { return strings.HasSuffix(c, "-pr-12") }) {
		t.Fatalf("conteneur de preview : %v", h.containers())
	}
	st, res = send("pull_request", hooks.Secret, pr("closed", "acme/site"))
	one("PR fermée", st, res, "preview_removed")
	expect(t, "previews", alice.do("GET", base+"/applications/"+appID+"/previews", nil, &previews), 200)
	if len(previews) != 0 || slices.ContainsFunc(h.containers(), func(c string) bool { return strings.HasSuffix(c, "-pr-12") }) {
		t.Fatalf("la preview doit être supprimée : %+v %v", previews, h.containers())
	}

	// Previews manuelles.
	var pd store.Deployment
	expect(t, "preview manuelle", alice.do("POST", base+"/applications/"+appID+"/previews", map[string]any{"pr_number": 3}, &pd), 202)
	if pd = waitDeployment(t, alice, base, appID, pd.ID); pd.Status != "finished" || pd.PRNumber != 3 {
		t.Fatalf("preview manuelle : %+v", pd)
	}
	var pv store.Preview
	expect(t, "domaine invalide", alice.do("PATCH", base+"/applications/"+appID+"/previews/3", map[string]any{"domains": []string{"pas un domaine"}}, nil), 422)
	expect(t, "domaines", alice.do("PATCH", base+"/applications/"+appID+"/previews/3", map[string]any{"domains": []string{"https://pr3.example.com"}}, &pv), 200)
	if !slices.Equal(pv.Domains, []string{"https://pr3.example.com"}) {
		t.Fatalf("domaines : %v", pv.Domains)
	}
	expect(t, "arrêt", alice.do("POST", base+"/applications/"+appID+"/previews/3/stop", nil, nil), 204)
	expect(t, "suppression", alice.do("DELETE", base+"/applications/"+appID+"/previews/3", nil, nil), 204)
	expect(t, "introuvable", alice.do("GET", base+"/applications/"+appID+"/previews/3", nil, nil), 404)

	// Webhook de déploiement (CI) : secret en en-tête ou en paramètre.
	deployHook := func(query, secretHeader string) (int, result) {
		t.Helper()
		req, _ := http.NewRequest("POST", f.e.srv.URL+"/webhooks/deploy/"+appID+query, nil)
		if secretHeader != "" {
			req.Header.Set("X-Nodra-Secret", secretHeader)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out result
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if st, _ := deployHook("?secret=faux", ""); st != 401 {
		t.Fatalf("secret invalide : %d", st)
	}
	if st, _ := deployHook("", ""); st != 401 {
		t.Fatalf("sans secret : %d", st)
	}
	st, r = deployHook("?force=1", hooks.Secret)
	if st != 202 || r.DeploymentID == "" {
		t.Fatalf("webhook de déploiement : %d %+v", st, r)
	}
	if d = waitDeployment(t, alice, base, appID, r.DeploymentID); d.Status != "finished" || !d.ForceRebuild {
		t.Fatalf("déploiement forcé : %+v", d)
	}
	if hooks.URLs["deploy"] != f.e.srv.URL+"/webhooks/deploy/"+appID {
		t.Fatalf("URL du webhook : %s", hooks.URLs["deploy"])
	}

	// Les règles d'approbation s'appliquent aussi aux webhooks.
	expect(t, "règle", alice.do("POST", base+"/approval-rules", map[string]any{"name": "Prod", "actions": []string{"resource.deploy"}}, nil), 201)
	st, r = deployHook(fmt.Sprintf("?secret=%s&commit=%s", hooks.Secret, sha[:12]), "")
	if st != 202 || r.Action != "approval_required" || r.ApprovalRequestID == "" {
		t.Fatalf("approbation : %d %+v", st, r)
	}
	var ar store.ApprovalRequest
	expect(t, "demande", alice.do("GET", base+"/approvals/"+r.ApprovalRequestID, nil, &ar), 200)
	if ar.RequestedBy != nil || !strings.Contains(string(ar.Payload), sha[:12]) {
		t.Fatalf("demande créée par le webhook : %+v", ar)
	}
}

func TestPreviewDomains(t *testing.T) {
	a := &store.Application{Domains: []string{"https://app.example.com", "http://api.example.com:8080/v1"}, PreviewURLTemplate: "{{pr_id}}.{{domain}}"}
	got := previewDomains(a, 7)
	if !slices.Equal(got, []string{"https://7.app.example.com", "http://7.api.example.com:8080/v1"}) {
		t.Fatalf("domaines : %v", got)
	}
	a.PreviewURLTemplate = "pr-{{pr_id}}-{{random}}.preview.example.com"
	if got := previewDomains(a, 7); len(got) != 2 || !strings.HasPrefix(got[0], "https://pr-7-") || got[0] == got[1] {
		t.Fatalf("modèle personnalisé : %v", got)
	}
	ev := &store.Deployment{Status: "finished", CommitSHA: sha40}
	body := previewComment(&store.Application{Name: "Site"}, &store.Preview{Domains: []string{"https://7.app.example.com"}}, ev)
	if !strings.Contains(body, "✅") || !strings.Contains(body, "https://7.app.example.com") || !strings.Contains(body, "`0123456`") {
		t.Fatalf("commentaire : %s", body)
	}
}

const sha40 = "0123456789abcdef0123456789abcdef01234567"

func TestComposePreview(t *testing.T) {
	f := newAppFixture(t)
	h := newFakeHost(t, f)
	alice, base := f.alice, f.base
	f.e.api.Executors["resource.deploy"] = f.e.api.execResourceDeploy
	var stack map[string]any
	expect(t, "compose", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Stack", "build_pack": "dockercompose",
		"docker_compose_raw":     "services:\n  web:\n    image: nginx\n    container_name: site\n    volumes: [\"data:/data\"]\nvolumes:\n  data:\n    name: site-data\n",
		"docker_compose_domains": map[string][]string{"web": {"https://web.example.com"}}}), &stack), 201)
	id := stack["id"].(string)
	var d store.Deployment
	expect(t, "production", alice.do("POST", base+"/applications/"+id+"/deploy", nil, &d), 202)
	if d = waitDeployment(t, alice, base, id, d.ID); d.Status != "finished" {
		t.Fatalf("production : %s", d.Error)
	}
	prod, _ := os.ReadFile(filepath.Join(h.state, "apps", id, "docker-compose.yml"))

	var pd store.Deployment
	expect(t, "preview", alice.do("POST", base+"/applications/"+id+"/previews", map[string]any{"pr_number": 5}, &pd), 202)
	if pd = waitDeployment(t, alice, base, id, pd.ID); pd.Status != "finished" {
		t.Fatalf("preview : %s\n%s", pd.Error, depLogs(t, alice, base, id, pd.ID, true))
	}
	var pv store.Preview
	expect(t, "preview", alice.do("GET", base+"/applications/"+id+"/previews/5", nil, &pv), 200)
	if len(pv.ComposeDomains["web"]) != 1 || pv.ComposeDomains["web"][0] != "https://5.web.example.com" {
		t.Fatalf("domaines de la preview : %+v", pv)
	}
	preview, err := os.ReadFile(filepath.Join(h.state, "apps", id, "pr-5", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	// Serveur de test sans proxy : pas de libellés de routage (vérifiés par TestTransformComposePreview).
	for _, want := range []string{"nodra.pr=5", "container_name: site-pr-5", "name: site-data-pr-5"} {
		if !strings.Contains(string(preview), want) {
			t.Fatalf("%q absent du compose de la preview :\n%s", want, preview)
		}
	}
	if after, _ := os.ReadFile(filepath.Join(h.state, "apps", id, "docker-compose.yml")); string(after) != string(prod) {
		t.Fatal("le compose de production a été modifié par la preview")
	}
	expect(t, "domaines personnalisés", alice.do("PATCH", base+"/applications/"+id+"/previews/5",
		map[string]any{"compose_domains": map[string][]string{"web": {"https://pr5.example.com"}}}, &pv), 200)
	if pv.ComposeDomains["web"][0] != "https://pr5.example.com" {
		t.Fatalf("mise à jour : %+v", pv)
	}
	expect(t, "arrêt de la preview", alice.do("POST", base+"/applications/"+id+"/previews/5/stop", nil, nil), 204)
	if _, err := os.Stat(filepath.Join(h.state, "apps", id, "pr-5")); !os.IsNotExist(err) {
		t.Fatalf("fichiers de la preview non retirés : %v", err)
	}
}
