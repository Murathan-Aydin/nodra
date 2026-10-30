// Package api expose l'API HTTP de Nodra et sert le front React embarqué.
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Murathan-Aydin/nodra/internal/backup"
	"github.com/Murathan-Aydin/nodra/internal/crypto"
	"github.com/Murathan-Aydin/nodra/internal/deploy"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Mailer envoie les e-mails transactionnels (implémenté avec le module de notifications).
type Mailer interface {
	PasswordReset(ctx context.Context, to, token string)
	TeamInvitation(ctx context.Context, to, teamName, token string)
}

type Server struct {
	Store *store.Store
	Log   *slog.Logger
	Web   fs.FS // build du front (web/dist)
	Box   *crypto.Box
	// SecureCookies active l'attribut Secure (désactivé en développement HTTP).
	SecureCookies bool
	Mailer        Mailer
	// TrustProxy : utiliser l'IP transmise par le proxy (sinon contournement de la limite de tentatives).
	TrustProxy bool
	// Resolver : résolution DNS (net.DefaultResolver par défaut).
	Resolver Resolver
	// Deployer pilote les conteneurs des ressources (nil tant que le module n'est pas branché).
	Deployer Deployer
	// GitEvents traite les push et pull requests reçus par webhook.
	GitEvents GitEventHandler
	// Backups : sauvegardes (envoi et lecture de fichiers sur les serveurs) ; Targets fournit les cibles SSH.
	Backups *backup.Service
	Targets backup.Targets
	// Jobs enfile les tâches de fond.
	Jobs JobQueue
	// Executors : exécuteurs des actions soumises à approbation, par action (ex. "resource.deploy").
	Executors map[string]Executor
	// AuthRateLimit : tentatives d'authentification par IP et par minute (10 par défaut).
	AuthRateLimit int
}

func (s *Server) Router() http.Handler {
	if s.GitEvents == nil {
		s.GitEvents = s
	}
	if s.Executors == nil {
		s.Executors = map[string]Executor{}
	}
	for action, exec := range map[string]Executor{
		"server.delete":           s.execDeleteServer,
		"project.delete":          s.execDeleteProject,
		"environment.delete":      s.execDeleteEnvironment,
		"resource.delete":         s.execDeleteResource,
		"resource.env.write":      s.execEnvWrite,
		"resource.deploy":         s.execResourceDeploy,
		"resource.backup.restore": s.execRestore,
	} {
		if _, ok := s.Executors[action]; !ok {
			s.Executors[action] = exec
		}
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	if s.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(s.logRequests, middleware.Recoverer)

	limit := s.AuthRateLimit
	if limit == 0 {
		limit = 10
	}
	authLimit := newRateLimiter(limit, time.Minute)

	// Webhooks des forges : authentifiés par signature, hors session et hors garde CSRF.
	r.Post("/webhooks/github", s.githubAppWebhook)
	r.Post("/webhooks/{provider}/manual", s.manualWebhook)
	r.Get("/webhooks/deploy/{app}", s.deployWebhook)
	r.Post("/webhooks/deploy/{app}", s.deployWebhook)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.health)

		r.Group(func(r chi.Router) {
			r.Use(csrfGuard, s.authenticate)

			r.Route("/auth", func(r chi.Router) {
				r.With(authLimit.middleware).Post("/register", s.register)
				r.With(authLimit.middleware).Post("/login", s.login)
				r.With(authLimit.middleware).Post("/login/2fa", s.loginTwoFactor)
				r.With(authLimit.middleware).Post("/password/forgot", s.forgotPassword)
				r.With(authLimit.middleware).Post("/password/reset", s.resetPassword)
				r.Post("/logout", s.logout)
			})

			r.Group(func(r chi.Router) {
				r.Use(requireUser)
				r.Get("/me", s.me)

				// Gestion du compte : session interactive uniquement.
				r.Group(func(r chi.Router) {
					r.Use(requireSession)
					r.Patch("/me", s.updateProfile)
					r.Post("/me/password", s.changePassword)
					r.Post("/me/2fa/setup", s.setupTwoFactor)
					r.Post("/me/2fa/enable", s.enableTwoFactor)
					r.Post("/me/2fa/disable", s.disableTwoFactor)
					r.Get("/me/sessions", s.listSessions)
					r.Delete("/me/sessions/{id}", s.revokeSession)
					r.Get("/me/tokens", s.listTokens)
					r.Post("/me/tokens", s.createToken)
					r.Delete("/me/tokens/{id}", s.deleteToken)
					r.Post("/invitations/preview", s.previewInvitation)
					r.Post("/invitations/accept", s.acceptInvitation)
				})

				r.Get("/permissions", s.listPermissions)
				r.Get("/service-templates", s.listServiceTemplates)
				r.Get("/service-templates/{slug}", s.getServiceTemplate)
				// Retours de GitHub après création / installation d'une App (navigateur, session requise).
				r.With(requireSession).Get("/sources/github/callback", s.githubCallback)
				r.With(requireSession).Get("/sources/github/install", s.githubInstall)
				r.Get("/teams", s.listTeams)
				r.Post("/teams", s.createTeam)

				r.Route("/teams/{team}", func(r chi.Router) {
					r.Use(s.withTeam)
					r.Get("/", s.getTeam)
					r.Patch("/", s.updateTeam)
					r.Delete("/", s.deleteTeam)
					r.Get("/permissions", s.myPermissions)
					r.Post("/leave", s.leaveTeam)
					r.Post("/transfer-ownership", s.transferOwnership)

					r.Get("/members", s.listMembers)
					r.Post("/members", s.addMember)
					r.Delete("/members/{user}", s.removeMember)

					r.Get("/invitations", s.listInvitations)
					r.Post("/invitations", s.createInvitation)
					r.Delete("/invitations/{id}", s.deleteInvitation)

					r.Get("/roles", s.listRoles)
					r.Post("/roles", s.createRole)
					r.Patch("/roles/{id}", s.updateRole)
					r.Delete("/roles/{id}", s.deleteRole)

					r.Get("/role-assignments", s.listAssignments)
					r.Post("/role-assignments", s.createAssignment)
					r.Delete("/role-assignments/{id}", s.deleteAssignment)

					r.Get("/groups", s.listGroups)
					r.Post("/groups", s.createGroup)
					r.Patch("/groups/{id}", s.updateGroup)
					r.Delete("/groups/{id}", s.deleteGroup)
					r.Put("/groups/{id}/members/{user}", s.addGroupMember)
					r.Delete("/groups/{id}/members/{user}", s.removeGroupMember)

					r.Get("/audit", s.listAudit)
					r.Get("/audit/export", s.exportAudit)

					r.Get("/approval-rules", s.listApprovalRules)
					r.Post("/approval-rules", s.createApprovalRule)
					r.Patch("/approval-rules/{id}", s.updateApprovalRule)
					r.Delete("/approval-rules/{id}", s.deleteApprovalRule)
					r.Get("/approvals", s.listApprovalRequests)
					r.Get("/approvals/{id}", s.getApprovalRequest)
					r.Post("/approvals/{id}/approve", s.voteApproval("approve"))
					r.Post("/approvals/{id}/reject", s.voteApproval("reject"))
					r.Post("/approvals/{id}/cancel", s.cancelApproval)

					r.Get("/private-keys", s.listKeys)
					r.Post("/private-keys", s.createKey)
					r.Get("/private-keys/{id}", s.getKey)
					r.Patch("/private-keys/{id}", s.updateKey)
					r.Delete("/private-keys/{id}", s.deleteKey)

					r.Get("/servers", s.listServers)
					r.Post("/servers", s.createServer)
					r.Route("/servers/{server}", func(r chi.Router) {
						r.Get("/", s.getServer)
						r.Patch("/", s.updateServer)
						r.Delete("/", s.deleteServer)
						r.Post("/validate", s.serverAction(servers.TaskValidate, "server.update"))
						r.Post("/install-docker", s.serverAction(servers.TaskInstallDocker, "server.update"))
						r.Post("/docker-cleanup", s.serverAction(servers.TaskDockerCleanup, "server.update"))
						r.Post("/packages/check", s.serverAction(servers.TaskPackagesCheck, "server.update"))
						r.Post("/packages/update", s.serverAction(servers.TaskPackagesUpdate, "server.update"))
						r.Post("/reboot", s.serverAction(servers.TaskReboot, "server.update"))
						r.Post("/commands", s.runCommand)
						r.Put("/cloudflared", s.startCloudflared)
						r.Delete("/cloudflared", s.serverAction(servers.TaskCloudflaredStop, "server.update"))
						r.Post("/host-key/reset", s.resetHostKey)
						r.Get("/tasks", s.listServerTasks)
						r.Get("/tasks/{task}", s.getServerTask)
						r.Post("/tasks/{task}/cancel", s.cancelServerTask)
						r.Get("/metrics", s.serverMetrics)
						r.Get("/destinations", s.listDestinations)
						r.Post("/destinations", s.createDestination)
						r.Delete("/destinations/{dest}", s.deleteDestination)

						r.Get("/proxy", s.getProxy)
						r.Patch("/proxy", s.updateProxy)
						r.Post("/proxy/start", s.serverAction(servers.TaskProxyStart, "server.proxy.manage"))
						r.Post("/proxy/stop", s.serverAction(servers.TaskProxyStop, "server.proxy.manage"))
						r.Post("/proxy/restart", s.serverAction(servers.TaskProxyRestart, "server.proxy.manage"))
						r.Get("/proxy/configs", s.listProxyConfigs)
						r.Put("/proxy/configs/{name}", s.putProxyConfig)
						r.Delete("/proxy/configs/{name}", s.deleteProxyConfig)
						r.Get("/proxy/certificates", s.listCertificates)
						r.Post("/proxy/certificates", s.createCertificate)
						r.Delete("/proxy/certificates/{cert}", s.deleteCertificate)
					})
					r.Post("/dns-check", s.dnsCheck)

					r.Get("/projects", s.listProjects)
					r.Post("/projects", s.createProject)
					r.Route("/projects/{project}", func(r chi.Router) {
						r.Get("/", s.getProject)
						r.Patch("/", s.updateProject)
						r.Delete("/", s.deleteProject)
						r.Post("/environments", s.createEnvironment)
						r.Get("/environments/{env}", s.getEnvironment)
						r.Patch("/environments/{env}", s.updateEnvironment)
						r.Delete("/environments/{env}", s.deleteEnvironment)
						r.Post("/environments/{env}/clone", s.cloneEnvironment)
					})

					r.Get("/variables", s.listVariables)
					r.Post("/variables", s.createVariable)
					r.Patch("/variables/{id}", s.updateVariable)
					r.Delete("/variables/{id}", s.deleteVariable)

					r.Get("/sources", s.listSources)
					r.Post("/sources", s.createSource)
					r.Post("/sources/github/manifest", s.startGitHubManifest)
					r.Get("/sources/{id}", s.getSource)
					r.Patch("/sources/{id}", s.updateSource)
					r.Delete("/sources/{id}", s.deleteSource)
					r.Get("/sources/{id}/repositories", s.listRepositories)
					r.Get("/sources/{id}/branches", s.listBranches)

					r.Get("/applications", s.listApplications)
					r.Post("/applications", s.createApplication)
					r.Route("/applications/{app}", func(r chi.Router) {
						r.Get("/", s.getApplication)
						r.Patch("/", s.updateApplication)
						r.Delete("/", s.deleteApplication)
						r.Get("/webhooks", s.appWebhooks)
						r.Post("/webhooks/rotate", s.appWebhooks)
						r.Put("/tags", s.setApplicationTags)
						r.Post("/deploy", s.deployApplication)
						r.Post("/restart", s.restartApplication)
						r.Post("/start", s.restartApplication)
						r.Post("/stop", s.stopApplication)
						r.Post("/rollback", s.rollbackApplication)
						r.Get("/images", s.applicationImages)
						r.Get("/logs", s.applicationLogs)
						r.Get("/deployments", s.listDeployments)
						r.Get("/deployments/{dep}", s.getDeployment)
						r.Get("/deployments/{dep}/logs", s.deploymentLogs)
						r.Post("/deployments/{dep}/cancel", s.cancelDeployment)
						r.Get("/previews", s.listPreviews)
						r.Post("/previews", s.createPreview)
						r.Get("/previews/{pr}", s.getPreview)
						r.Patch("/previews/{pr}", s.updatePreview)
						r.Delete("/previews/{pr}", s.deletePreview)
						r.Post("/previews/{pr}/deploy", s.deployPreview)
						r.Post("/previews/{pr}/stop", s.stopPreview)
						r.Get("/envs", s.listEnvs(s.appLoader))
						r.Post("/envs", s.createEnv(s.appLoader))
						r.Post("/envs/bulk", s.bulkEnvs(s.appLoader))
						r.Patch("/envs/{env}", s.updateEnv(s.appLoader))
						r.Delete("/envs/{env}", s.deleteEnv(s.appLoader))
						r.Get("/storages", s.listStorages(s.appLoader))
						r.Post("/storages", s.createStorage(s.appLoader))
						r.Patch("/storages/{storage}", s.updateStorage(s.appLoader))
						r.Delete("/storages/{storage}", s.deleteStorage(s.appLoader))
					})

					r.Get("/s3-storages", s.listS3)
					r.Post("/s3-storages", s.createS3)
					r.Get("/s3-storages/{id}", s.getS3)
					r.Patch("/s3-storages/{id}", s.updateS3)
					r.Delete("/s3-storages/{id}", s.deleteS3)
					r.Post("/s3-storages/{id}/check", s.checkS3Handler)

					r.Get("/backups", s.listBackupSchedules)
					r.Post("/backups", s.createBackupSchedule)
					r.Patch("/backups/{id}", s.updateBackupSchedule)
					r.Delete("/backups/{id}", s.deleteBackupSchedule)
					r.Post("/backups/{id}/run", s.runBackupNow)
					r.Get("/backup-executions", s.listBackupExecutions)
					r.Post("/backup-executions/{id}/restore", s.restoreBackup)
					r.Get("/backup-executions/{id}/download", s.downloadBackup)
					r.Delete("/backup-executions/{id}", s.deleteBackupExecution)

					r.Get("/scheduled-tasks", s.listScheduledTasks)
					r.Post("/scheduled-tasks", s.createScheduledTask)
					r.Patch("/scheduled-tasks/{id}", s.updateScheduledTask)
					r.Delete("/scheduled-tasks/{id}", s.deleteScheduledTask)
					r.Post("/scheduled-tasks/{id}/run", s.runScheduledTask)
					r.Get("/scheduled-tasks/{id}/executions", s.scheduledTaskExecutions)

					r.Get("/services", s.listServices)
					r.Post("/services", s.createService)
					r.Route("/services/{svc}", func(r chi.Router) {
						r.Get("/", s.getService)
						r.Patch("/", s.updateService)
						r.Delete("/", s.deleteService)
						r.Put("/tags", s.setServiceTags)
						r.Patch("/apps/{app}", s.updateServiceApp)
						r.Post("/apps/{app}/restart", s.serviceAction(deploy.TaskServiceRestartApp))
						r.Post("/start", s.serviceAction(deploy.TaskServiceStart))
						r.Post("/stop", s.serviceAction(deploy.TaskServiceStop))
						r.Post("/restart", s.serviceAction(deploy.TaskServiceRestart))
						r.Get("/tasks", s.serviceTasks)
						r.Get("/logs", s.serviceLogs)
						r.Get("/envs", s.listEnvs(s.svcLoader))
						r.Post("/envs", s.createEnv(s.svcLoader))
						r.Post("/envs/bulk", s.bulkEnvs(s.svcLoader))
						r.Patch("/envs/{env}", s.updateEnv(s.svcLoader))
						r.Delete("/envs/{env}", s.deleteEnv(s.svcLoader))
					})

					r.Get("/databases", s.listDatabases)
					r.Post("/databases", s.createDatabase)
					r.Route("/databases/{db}", func(r chi.Router) {
						r.Get("/", s.getDatabase)
						r.Patch("/", s.updateDatabase)
						r.Delete("/", s.deleteDatabase)
						r.Put("/tags", s.setDatabaseTags)
						r.Post("/start", s.databaseAction(deploy.TaskDatabaseStart))
						r.Post("/stop", s.databaseAction(deploy.TaskDatabaseStop))
						r.Post("/restart", s.databaseAction(deploy.TaskDatabaseRestart))
						r.Get("/tasks", s.databaseTasks)
						r.Get("/logs", s.databaseLogs)
						r.Post("/import", s.importDatabase)
						r.Get("/envs", s.listEnvs(s.dbLoader))
						r.Post("/envs", s.createEnv(s.dbLoader))
						r.Post("/envs/bulk", s.bulkEnvs(s.dbLoader))
						r.Patch("/envs/{env}", s.updateEnv(s.dbLoader))
						r.Delete("/envs/{env}", s.deleteEnv(s.dbLoader))
						r.Get("/storages", s.listStorages(s.dbLoader))
						r.Post("/storages", s.createStorage(s.dbLoader))
						r.Patch("/storages/{storage}", s.updateStorage(s.dbLoader))
						r.Delete("/storages/{storage}", s.deleteStorage(s.dbLoader))
					})

					r.Get("/tags", s.listTags)
					r.Post("/tags", s.createTag)
					r.Patch("/tags/{id}", s.updateTag)
					r.Delete("/tags/{id}", s.deleteTag)
				})
			})
		})
	})
	r.Handle("/*", spa(s.Web))
	return r
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Pool.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "database": "down"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		s.Log.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start))
	})
}

// spa sert les fichiers statiques et renvoie index.html pour les routes du front.
func spa(webFS fs.FS) http.Handler {
	files := http.FileServerFS(webFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(webFS, p); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFileFS(w, r, webFS, "index.html")
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) internal(w http.ResponseWriter, err error) {
	s.Log.Error("erreur interne", "err", err)
	writeError(w, http.StatusInternalServerError, "erreur interne")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
