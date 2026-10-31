package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Murathan-Aydin/nodra/internal/deploy"
	"github.com/Murathan-Aydin/nodra/internal/git"
	"github.com/Murathan-Aydin/nodra/internal/notify"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// GitEventResult : suite donnée à un événement Git pour une application.
type GitEventResult struct {
	ApplicationID     string `json:"application_id"`
	Action            string `json:"action"` // deploy | preview_deploy | preview_removed | skipped | approval_required
	DeploymentID      string `json:"deployment_id,omitempty"`
	ApprovalRequestID string `json:"approval_request_id,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

// HandleGitEvent : implémentation par défaut de GitEventHandler (applications liées à la source).
func (s *Server) HandleGitEvent(r *http.Request, src *store.GitSource, ev *git.Event) (any, error) {
	apps, err := s.Store.ApplicationsByRepo(r.Context(), repoKeys(ev))
	if err != nil {
		return nil, err
	}
	var mine []*store.Application
	for _, a := range apps {
		if a.SourceID != nil && *a.SourceID == src.ID {
			mine = append(mine, a)
		}
	}
	return s.dispatchGitEvent(r, mine, ev), nil
}

// repoKeys : formes possibles du dépôt telles qu'elles peuvent être saisies dans une application.
func repoKeys(ev *git.Event) []string {
	repo := strings.ToLower(strings.TrimSuffix(strings.Trim(ev.Repo, "/"), ".git"))
	if repo == "" {
		return nil
	}
	keys := []string{repo}
	if u, err := url.Parse(ev.CloneURL); err == nil && u.Host != "" {
		host := strings.ToLower(u.Host)
		keys = append(keys, "https://"+host+"/"+repo, "http://"+host+"/"+repo,
			"git@"+u.Hostname()+":"+repo, "ssh://git@"+host+"/"+repo)
	}
	return keys
}

func (s *Server) dispatchGitEvent(r *http.Request, apps []*store.Application, ev *git.Event) []GitEventResult {
	out := []GitEventResult{}
	for _, a := range apps {
		res := GitEventResult{ApplicationID: a.ID}
		var err error
		switch ev.Kind {
		case "push":
			err = s.pushEvent(r, a, ev, &res)
		case "pull_request":
			err = s.pullRequestEvent(r, a, ev, &res)
		default:
			res.Action, res.Reason = "skipped", "événement ignoré"
		}
		if err != nil {
			s.Log.Error("événement Git", "app", a.ID, "kind", ev.Kind, "err", err)
			res.Action, res.Reason = "skipped", "erreur interne"
		}
		out = append(out, res)
	}
	return out
}

func skip(res *GitEventResult, reason string) error {
	res.Action, res.Reason = "skipped", reason
	return nil
}

func (s *Server) pushEvent(r *http.Request, a *store.Application, ev *git.Event, res *GitEventResult) error {
	switch {
	case ev.Deleted:
		return skip(res, "branche supprimée")
	case !strings.EqualFold(ev.Branch, a.GitBranch):
		return skip(res, "branche "+ev.Branch+" non suivie")
	case !a.Settings.AutoDeploy:
		return skip(res, "déploiement automatique désactivé")
	case !git.MatchWatchPaths(a.WatchPaths, ev.Files):
		return skip(res, "aucun fichier surveillé modifié (watch_paths)")
	}
	req := deployRequest{Trigger: "webhook", Branch: a.GitBranch}
	if shaRe.MatchString(ev.Commit) && ev.Commit != "HEAD" {
		req.CommitSHA = ev.Commit
	}
	res.Action = "deploy"
	return s.webhookDeploy(r, a, req, "Déploiement de "+a.Name+" (push "+shortCommit(ev.Commit)+")", res)
}

func (s *Server) pullRequestEvent(r *http.Request, a *store.Application, ev *git.Event, res *GitEventResult) error {
	ctx := r.Context()
	switch {
	case ev.PRNumber <= 0:
		return skip(res, "numéro de PR manquant")
	case !a.Settings.PreviewDeployments:
		return skip(res, "prévisualisations désactivées")
	case !strings.EqualFold(ev.Branch, a.GitBranch):
		return skip(res, "branche cible "+ev.Branch+" non suivie")
	case a.BuildPack == "dockerimage" || a.GitRepository == "":
		return skip(res, "application sans dépôt Git")
	}
	if ev.Action == "closed" {
		res.Action = "preview_removed"
		return s.removePreview(r, a, ev.PRNumber)
	}
	if ev.Action != "opened" && ev.Action != "synchronize" && ev.Action != "reopened" {
		return skip(res, "action "+ev.Action+" ignorée")
	}
	if ev.IsFork && !a.Settings.PublicPRDeployments {
		return skip(res, "PR venant d'un fork (public_pr_deployments désactivé)")
	}
	pv := &store.Preview{ApplicationID: a.ID, PRNumber: ev.PRNumber, PRURL: ev.PRURL, Title: truncate(ev.PRTitle, 255),
		LastCommit: ev.Commit, Domains: previewDomains(a, ev.PRNumber), ComposeDomains: previewComposeDomains(a, ev.PRNumber)}
	if _, err := s.Store.UpsertPreview(ctx, pv); err != nil {
		return err
	}
	req := deployRequest{Trigger: "preview", Branch: a.GitBranch, PRNumber: ev.PRNumber}
	if shaRe.MatchString(ev.Commit) && ev.Commit != "HEAD" {
		req.CommitSHA = ev.Commit
	}
	res.Action = "preview_deploy"
	return s.webhookDeploy(r, a, req, fmt.Sprintf("Prévisualisation de %s (PR #%d)", a.Name, ev.PRNumber), res)
}

// webhookDeploy enfile un déploiement déclenché sans utilisateur (soumis aux mêmes règles d'approbation).
func (s *Server) webhookDeploy(r *http.Request, a *store.Application, req deployRequest, summary string, res *GitEventResult) error {
	req.TeamID, req.AppID = a.TeamID, a.ID
	pending, err := s.gateAs(r, a.TeamID, nil, "resource.deploy", resourceScope(a.ID), summary, req)
	if err != nil {
		return err
	}
	if pending != nil {
		res.Action, res.ApprovalRequestID = "approval_required", pending.ID
		return nil
	}
	d, err := s.queueDeployment(r.Context(), req)
	if err != nil {
		return err
	}
	res.DeploymentID = d.ID
	s.audit(r, &a.TeamID, "application.deploy", "application", a.ID, map[string]any{"deployment_id": d.ID, "trigger": d.Trigger,
		"commit": d.CommitSHA, "pr_number": d.PRNumber})
	return nil
}

// removePreview arrête les conteneurs de la preview et la supprime (PR fermée ou suppression manuelle).
func (s *Server) removePreview(r *http.Request, a *store.Application, pr int) error {
	ctx := r.Context()
	if _, err := s.Store.PreviewByNumber(ctx, a.ID, pr); errors.Is(err, store.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if err := s.Store.CancelPreviewDeployments(ctx, a.ID, pr); err != nil {
		return err
	}
	if s.Deployer != nil {
		if err := s.Deployer.Stop(ctx, a, pr); err != nil {
			s.Log.Warn("arrêt de la preview", "app", a.ID, "pr", pr, "err", err)
		}
	}
	if err := s.Store.DeletePreview(ctx, a.ID, pr); err != nil {
		return err
	}
	s.audit(r, &a.TeamID, "application.preview.deleted", "application", a.ID, map[string]any{"pr_number": pr})
	return nil
}

// previewDomains dérive les domaines d'une preview de ceux de l'application via preview_url_template
// ({{pr_id}}, {{domain}}, {{random}}) ; schéma, port et chemin sont conservés.
func previewDomains(a *store.Application, pr int) []string {
	return previewHosts(a.Domains, a.PreviewURLTemplate, pr)
}

// previewComposeDomains : mêmes règles, par service compose.
func previewComposeDomains(a *store.Application, pr int) map[string][]string {
	out := map[string][]string{}
	for svc, domains := range a.DockerComposeDomains {
		if d := previewHosts(domains, a.PreviewURLTemplate, pr); len(d) > 0 {
			out[svc] = d
		}
	}
	return out
}

func previewHosts(domains []string, template string, pr int) []string {
	out := []string{}
	for _, d := range domains {
		u, err := url.Parse(d)
		if err != nil || u.Host == "" {
			continue
		}
		host := strings.NewReplacer("{{pr_id}}", strconv.Itoa(pr), "{{domain}}", u.Hostname(), "{{random}}", randomHex(4)).
			Replace(template)
		if p := u.Port(); p != "" {
			host += ":" + p
		}
		u.Host = strings.ToLower(host)
		if n, _, ok := NormalizeAppDomain(u.String()); ok {
			out = append(out, n)
		}
	}
	return out
}

func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// --- Webhooks manuels (secret propre à l'application) ---

// manualWebhook reçoit les webhooks configurés à la main sur une forge : seules les applications
// du dépôt dont le secret valide la signature sont concernées.
func (s *Server) manualWebhook(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	switch provider {
	case git.GitHub, git.GitLab, git.Gitea, "bitbucket":
	default:
		writeError(w, http.StatusNotFound, "forge inconnue")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "corps trop volumineux")
		return
	}
	ev, err := git.Parse(provider, r.Header, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if ev.Kind == "ping" {
		writeJSON(w, http.StatusOK, map[string]string{"message": "pong"})
		return
	}
	apps, err := s.Store.ApplicationsByRepo(r.Context(), repoKeys(ev))
	if err != nil {
		s.internal(w, err)
		return
	}
	var verified []*store.Application
	for _, a := range apps {
		secret, err := s.Box.Decrypt(a.WebhookSecretEnc)
		if err == nil && git.Verify(provider, secret, r.Header, body) == nil {
			verified = append(verified, a)
		}
	}
	if len(verified) == 0 {
		writeError(w, http.StatusUnauthorized, "signature invalide ou dépôt inconnu")
		return
	}
	if ev.Kind != "push" && ev.Kind != "pull_request" {
		writeJSON(w, http.StatusOK, map[string]string{"message": "événement ignoré"})
		return
	}
	writeJSON(w, http.StatusOK, s.dispatchGitEvent(r, verified, ev))
}

// deployWebhook : déclenchement d'un déploiement par URL (CI, scripts). Le secret de l'application est
// transmis en en-tête (X-Nodra-Secret ou Authorization: Bearer) ou en paramètre `secret`.
func (s *Server) deployWebhook(w http.ResponseWriter, r *http.Request) {
	unauthorized := func() { writeError(w, http.StatusUnauthorized, "secret invalide") }
	id := chi.URLParam(r, "app")
	if !isUUID(id) {
		unauthorized()
		return
	}
	a, err := s.Store.ApplicationByIDAny(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		unauthorized()
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	given := r.Header.Get("X-Nodra-Secret")
	if given == "" {
		given, _ = strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if given == "" {
		given = r.URL.Query().Get("secret")
	}
	secret, err := s.Box.Decrypt(a.WebhookSecretEnc)
	if err != nil || secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(given)) != 1 {
		unauthorized()
		return
	}
	q := r.URL.Query()
	req := deployRequest{Trigger: "webhook", ForceRebuild: q.Get("force") == "true" || q.Get("force") == "1"}
	if c := q.Get("commit"); c != "" {
		if !shaRe.MatchString(c) {
			writeError(w, http.StatusUnprocessableEntity, "commit invalide")
			return
		}
		req.CommitSHA = c
	}
	res := GitEventResult{ApplicationID: a.ID, Action: "deploy"}
	if err := s.webhookDeploy(r, a, req, "Déploiement de "+a.Name+" (webhook)", &res); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, res)
}

// --- Prévisualisations ---

func (s *Server) listPreviews(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.read")
	if !ok {
		return
	}
	list, err := s.Store.ListPreviews(r.Context(), a.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) appPreview(w http.ResponseWriter, r *http.Request, perm string) (*store.Application, *store.Preview, bool) {
	a, ok := s.teamApp(w, r, perm)
	if !ok {
		return nil, nil, false
	}
	pr, err := strconv.Atoi(chi.URLParam(r, "pr"))
	if err != nil || pr <= 0 {
		writeError(w, http.StatusNotFound, "introuvable")
		return nil, nil, false
	}
	pv, err := s.Store.PreviewByNumber(r.Context(), a.ID, pr)
	if err != nil {
		s.fail(w, err)
		return nil, nil, false
	}
	return a, pv, true
}

func (s *Server) getPreview(w http.ResponseWriter, r *http.Request) {
	if _, pv, ok := s.appPreview(w, r, "resource.read"); ok {
		writeJSON(w, http.StatusOK, pv)
	}
}

// createPreview crée (ou recrée) manuellement la preview d'une PR et la déploie.
func (s *Server) createPreview(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.deploy")
	if !ok {
		return
	}
	var in struct {
		PRNumber int    `json:"pr_number"`
		Title    string `json:"title"`
		PRURL    string `json:"pr_url"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.PRNumber <= 0 || (in.PRURL != "" && !validHTTPURL(in.PRURL)) {
		writeError(w, http.StatusUnprocessableEntity, "pr_number ou pr_url invalide")
		return
	}
	if a.GitRepository == "" || a.BuildPack == "dockerimage" {
		writeError(w, http.StatusUnprocessableEntity, "application sans dépôt Git")
		return
	}
	if _, err := s.Store.UpsertPreview(r.Context(), &store.Preview{ApplicationID: a.ID, PRNumber: in.PRNumber, PRURL: in.PRURL,
		Title: truncate(in.Title, 255), Domains: previewDomains(a, in.PRNumber), ComposeDomains: previewComposeDomains(a, in.PRNumber)}); err != nil {
		s.internal(w, err)
		return
	}
	s.gatedDeploy(w, r, a, deployRequest{Trigger: "preview", Branch: a.GitBranch, PRNumber: in.PRNumber},
		fmt.Sprintf("Prévisualisation de %s (PR #%d)", a.Name, in.PRNumber))
}

func (s *Server) deployPreview(w http.ResponseWriter, r *http.Request) {
	a, pv, ok := s.appPreview(w, r, "resource.deploy")
	if !ok {
		return
	}
	var in struct {
		ForceRebuild bool `json:"force_rebuild"`
	}
	if r.ContentLength > 0 && !decode(w, r, &in) {
		return
	}
	s.gatedDeploy(w, r, a, deployRequest{Trigger: "preview", Branch: a.GitBranch, PRNumber: pv.PRNumber, ForceRebuild: in.ForceRebuild},
		fmt.Sprintf("Prévisualisation de %s (PR #%d)", a.Name, pv.PRNumber))
}

func (s *Server) updatePreview(w http.ResponseWriter, r *http.Request) {
	a, pv, ok := s.appPreview(w, r, "resource.update")
	if !ok {
		return
	}
	var in struct {
		Domains        *[]string            `json:"domains"`
		ComposeDomains *map[string][]string `json:"compose_domains"`
	}
	if !decode(w, r, &in) {
		return
	}
	norm := func(list []string) ([]string, bool) {
		out := []string{}
		for _, d := range list {
			n, _, ok := NormalizeAppDomain(d)
			if !ok {
				writeError(w, http.StatusUnprocessableEntity, "domaine invalide : "+d)
				return nil, false
			}
			out = append(out, n)
		}
		return out, true
	}
	if in.Domains != nil {
		domains, ok := norm(*in.Domains)
		if !ok {
			return
		}
		if err := s.Store.UpdatePreviewDomains(r.Context(), a.ID, pv.PRNumber, domains); err != nil {
			s.internal(w, err)
			return
		}
		pv.Domains = domains
	}
	if in.ComposeDomains != nil { // applications Docker Compose : domaines par service
		cd := map[string][]string{}
		for svc, list := range *in.ComposeDomains {
			if !serviceRe.MatchString(svc) {
				writeError(w, http.StatusUnprocessableEntity, "service invalide : "+svc)
				return
			}
			domains, ok := norm(list)
			if !ok {
				return
			}
			cd[svc] = domains
		}
		if err := s.Store.UpdatePreviewComposeDomains(r.Context(), a.ID, pv.PRNumber, cd); err != nil {
			s.internal(w, err)
			return
		}
		pv.ComposeDomains = cd
	}
	s.auditTeam(r, "application.preview.updated", "application", a.ID, map[string]any{"pr_number": pv.PRNumber, "domains": pv.Domains,
		"compose_domains": pv.ComposeDomains})
	writeJSON(w, http.StatusOK, pv)
}

func (s *Server) stopPreview(w http.ResponseWriter, r *http.Request) {
	a, pv, ok := s.appPreview(w, r, "resource.deploy")
	if !ok || !s.deployer(w) {
		return
	}
	if err := s.Deployer.Stop(r.Context(), a, pv.PRNumber); err != nil {
		writeError(w, http.StatusBadGateway, "arrêt impossible : "+err.Error())
		return
	}
	if err := s.Store.SetPreviewStatus(r.Context(), a.ID, pv.PRNumber, "stopped"); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "application.preview.stopped", "application", a.ID, map[string]any{"pr_number": pv.PRNumber})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deletePreview(w http.ResponseWriter, r *http.Request) {
	a, pv, ok := s.appPreview(w, r, "resource.deploy")
	if !ok {
		return
	}
	if err := s.removePreview(r, a, pv.PRNumber); err != nil {
		s.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Commentaires de PR ---

// DeploymentFinished publie (ou met à jour) un commentaire sur la PR (GitHub, GitLab, Gitea) à la fin d'un déploiement de preview.
// Branché sur deploy.Runner.OnFinish.
func (s *Server) DeploymentFinished(ctx context.Context, d *store.Deployment) {
	if d.Status != "finished" && d.Status != "failed" {
		return
	}
	s.notifyDeployment(ctx, d)
	if d.PRNumber == 0 {
		return
	}
	if err := s.commentPreview(ctx, d); err != nil {
		s.Log.Warn("commentaire de PR", "deployment", d.ID, "err", err)
	}
}

func (s *Server) commentPreview(ctx context.Context, d *store.Deployment) error {
	a, err := s.Store.ApplicationByIDAny(ctx, d.ApplicationID)
	if err != nil || a.SourceID == nil {
		return err
	}
	g, err := s.Store.GitSourceByID(ctx, a.TeamID, *a.SourceID)
	if err != nil || (g.Type == git.GitHub && g.InstallationID == 0) {
		return err
	}
	pv, err := s.Store.PreviewByNumber(ctx, a.ID, d.PRNumber)
	if err != nil {
		return err
	}
	src, err := deploy.SourceFor(s.Box, g)
	if err != nil {
		return err
	}
	body := previewComment(a, pv, d)
	id, err := git.CommentPR(ctx, src, a.GitRepository, d.PRNumber, pv.CommentID, body)
	if err != nil && pv.CommentID > 0 { // commentaire supprimé entre-temps : en recréer un
		id, err = git.CommentPR(ctx, src, a.GitRepository, d.PRNumber, 0, body)
	}
	if err != nil {
		return err
	}
	if id > 0 && id != pv.CommentID {
		return s.Store.SetPreviewComment(ctx, a.ID, d.PRNumber, id)
	}
	return nil
}

func previewComment(a *store.Application, pv *store.Preview, d *store.Deployment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Nodra** · prévisualisation de **%s**\n\n", a.Name)
	if d.Status == "finished" {
		fmt.Fprintf(&b, "✅ Déployée (commit `%s`).\n", shortCommit(d.CommitSHA))
		for _, u := range pv.Domains {
			fmt.Fprintf(&b, "\n- %s", u)
		}
	} else {
		fmt.Fprintf(&b, "❌ Échec du déploiement (commit `%s`) : consultez les logs dans Nodra.\n", shortCommit(d.CommitSHA))
	}
	return b.String()
}

// notifyDeployment publie le résultat d'un déploiement.
func (s *Server) notifyDeployment(ctx context.Context, d *store.Deployment) {
	name := d.ApplicationID
	if a, err := s.Store.ApplicationByIDAny(ctx, d.ApplicationID); err == nil {
		name = a.Name
	}
	if d.PRNumber > 0 {
		name += fmt.Sprintf(" (PR #%d)", d.PRNumber)
	}
	ok := d.Status == "finished"
	e := notify.Event{TeamID: d.TeamID, URL: "/applications/" + d.ApplicationID + "/deployments/" + d.ID,
		Fields: map[string]string{"Déclencheur": d.Trigger}}
	if d.CommitSHA != "" {
		e.Fields["Commit"] = d.CommitSHA[:min(12, len(d.CommitSHA))] + " " + d.CommitMessage
	}
	if ok {
		e.Type, e.Title, e.Level, e.Message = notify.DeploymentSuccess, "Déploiement réussi", "success", name
	} else {
		e.Type, e.Title, e.Level, e.Message = notify.DeploymentFailure, "Échec du déploiement", "error", name+" : "+d.Error
	}
	s.publish(ctx, e)
}
