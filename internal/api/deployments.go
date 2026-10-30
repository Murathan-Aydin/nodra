package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

// deployRequest : paramètres d'un déploiement (aussi stockés dans une demande d'approbation).
type deployRequest struct {
	TeamID        string  `json:"team_id"`
	AppID         string  `json:"application_id"`
	Trigger       string  `json:"trigger"`
	Branch        string  `json:"branch,omitempty"`
	CommitSHA     string  `json:"commit_sha,omitempty"`
	PRNumber      int     `json:"pr_number,omitempty"`
	ForceRebuild  bool    `json:"force_rebuild,omitempty"`
	RollbackImage string  `json:"rollback_image,omitempty"`
	CreatedBy     *string `json:"created_by,omitempty"`
}

// queueDeployment enfile un déploiement (délai maximal : réglage du serveur).
func (s *Server) queueDeployment(ctx context.Context, req deployRequest) (*store.Deployment, error) {
	loc, err := s.Store.ResourceLocation(ctx, req.AppID)
	if err != nil {
		return nil, err
	}
	sv, err := s.Store.ServerByIDAny(ctx, loc.ServerID)
	if err != nil {
		return nil, err
	}
	return s.Jobs.EnqueueDeployment(ctx, &store.Deployment{TeamID: req.TeamID, ApplicationID: req.AppID, ServerID: sv.ID,
		Trigger: req.Trigger, Branch: req.Branch, CommitSHA: req.CommitSHA, PRNumber: req.PRNumber, ForceRebuild: req.ForceRebuild,
		RollbackImage: req.RollbackImage, CreatedBy: req.CreatedBy}, sv.Settings.DeploymentTimeout)
}

func (s *Server) execDeploy(ctx context.Context, req *store.ApprovalRequest) error {
	var p deployRequest
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	p.Trigger = "approval"
	_, err := s.queueDeployment(ctx, p)
	return err
}

// gatedDeploy soumet le déploiement aux approbations ou l'enfile directement (202 dans les deux cas).
func (s *Server) gatedDeploy(w http.ResponseWriter, r *http.Request, a *store.Application, req deployRequest, summary string) {
	ctx := r.Context()
	uid := userFrom(ctx).ID
	req.TeamID, req.AppID, req.CreatedBy = a.TeamID, a.ID, &uid
	if req.Trigger == "" {
		req.Trigger = "manual"
		if tokenFrom(ctx) != nil {
			req.Trigger = "api"
		}
	}
	pending, err := s.gate(r, "resource.deploy", resourceScope(a.ID), summary, req)
	if err != nil {
		s.fail(w, err)
		return
	}
	if pending != nil {
		respondPending(w, pending)
		return
	}
	d, err := s.queueDeployment(ctx, req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "application.deploy", "application", a.ID, map[string]any{"deployment_id": d.ID, "trigger": d.Trigger,
		"force_rebuild": d.ForceRebuild, "rollback_image": d.RollbackImage})
	writeJSON(w, http.StatusAccepted, d)
}

func (s *Server) deployApplication(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.deploy")
	if !ok {
		return
	}
	var in struct {
		ForceRebuild bool   `json:"force_rebuild"`
		CommitSHA    string `json:"commit_sha"`
	}
	if r.ContentLength > 0 && !decode(w, r, &in) {
		return
	}
	if in.CommitSHA != "" && !shaRe.MatchString(in.CommitSHA) {
		writeError(w, http.StatusUnprocessableEntity, "commit_sha invalide")
		return
	}
	s.gatedDeploy(w, r, a, deployRequest{ForceRebuild: in.ForceRebuild, CommitSHA: in.CommitSHA}, "Déploiement de "+a.Name)
}

// restartApplication redéploie la dernière image (sans reconstruction).
func (s *Server) restartApplication(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.deploy")
	if !ok {
		return
	}
	img, err := s.Store.LastDeployedImage(r.Context(), a.ID, 0)
	if errors.Is(err, store.ErrNotFound) || a.BuildPack == "dockercompose" {
		s.gatedDeploy(w, r, a, deployRequest{Trigger: "restart"}, "Redémarrage de "+a.Name)
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	s.gatedDeploy(w, r, a, deployRequest{Trigger: "restart", RollbackImage: img}, "Redémarrage de "+a.Name)
}

var rollbackTagRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

func (s *Server) rollbackApplication(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.deploy")
	if !ok {
		return
	}
	var in struct {
		Tag string `json:"tag"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !rollbackTagRe.MatchString(in.Tag) || a.BuildPack == "dockercompose" {
		writeError(w, http.StatusUnprocessableEntity, "tag d'image invalide")
		return
	}
	s.gatedDeploy(w, r, a, deployRequest{Trigger: "rollback", RollbackImage: "nodra/" + a.ID + ":" + in.Tag}, "Retour arrière de "+a.Name+" vers "+in.Tag)
}

func (s *Server) deployer(w http.ResponseWriter) bool {
	if s.Deployer == nil {
		writeError(w, http.StatusServiceUnavailable, "module de déploiement indisponible")
		return false
	}
	return true
}

func (s *Server) stopApplication(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.deploy")
	if !ok || !s.deployer(w) {
		return
	}
	if err := s.Deployer.Stop(r.Context(), a, 0); err != nil {
		writeError(w, http.StatusBadGateway, "arrêt impossible : "+err.Error())
		return
	}
	s.auditTeam(r, "application.stopped", "application", a.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listDeployments(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.read")
	if !ok {
		return
	}
	list, err := s.Store.ListDeployments(r.Context(), a.ID, 50)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) appDeployment(w http.ResponseWriter, r *http.Request, perm string) (*store.Deployment, bool) {
	a, ok := s.teamApp(w, r, perm)
	if !ok {
		return nil, false
	}
	id, ok := idParam(w, r, "dep")
	if !ok {
		return nil, false
	}
	d, err := s.Store.DeploymentByID(r.Context(), id)
	if err != nil || d.ApplicationID != a.ID {
		writeError(w, http.StatusNotFound, "introuvable")
		return nil, false
	}
	return d, true
}

func (s *Server) getDeployment(w http.ResponseWriter, r *http.Request) {
	if d, ok := s.appDeployment(w, r, "resource.read"); ok {
		writeJSON(w, http.StatusOK, d)
	}
}

// deploymentLogs renvoie les lignes après le curseur `after` (suivi en direct par interrogation).
func (s *Server) deploymentLogs(w http.ResponseWriter, r *http.Request) {
	d, ok := s.appDeployment(w, r, "resource.logs.read")
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	logs, err := s.Store.DeploymentLogs(r.Context(), d.ID, after, r.URL.Query().Get("debug") == "1", 2000)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": d.Status, "logs": logs})
}

func (s *Server) cancelDeployment(w http.ResponseWriter, r *http.Request) {
	d, ok := s.appDeployment(w, r, "resource.deploy")
	if !ok {
		return
	}
	done, err := s.Store.RequestCancel(r.Context(), d.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if !done {
		writeError(w, http.StatusConflict, "le déploiement est déjà terminé")
		return
	}
	s.auditTeam(r, "application.deploy.cancelled", "deployment", d.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) applicationImages(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.read")
	if !ok || !s.deployer(w) {
		return
	}
	images, err := s.Deployer.Images(r.Context(), a)
	if err != nil {
		writeError(w, http.StatusBadGateway, "serveur inaccessible : "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, images)
}

func (s *Server) applicationLogs(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.logs.read")
	if !ok || !s.deployer(w) {
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines <= 0 || lines > 5000 {
		lines = 100
	}
	pr, _ := strconv.Atoi(r.URL.Query().Get("pr"))
	out, err := s.Deployer.ContainerLogs(r.Context(), a, pr, lines)
	if err != nil {
		writeError(w, http.StatusBadGateway, "serveur inaccessible : "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": out})
}
