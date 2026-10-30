package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Temps réel : WebSocket authentifiés comme le reste de l'API (cookie de session ou jeton). L'origine
// du navigateur doit être celle de l'instance (ou figurer dans AllowedOrigins) : protection contre le
// détournement de WebSocket entre sites.
//
// Messages texte : JSON {type: …}. Messages binaires : octets bruts (logs suivis, terminal).

var containerRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,254}$`)

type streams struct {
	once   sync.Once
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *Server) streamsCtx() context.Context {
	s.streams.once.Do(func() { s.streams.ctx, s.streams.cancel = context.WithCancel(context.Background()) })
	return s.streams.ctx
}

// CloseStreams ferme les connexions longues (à l'arrêt du serveur HTTP, qui ne les suit plus après leur reprise).
func (s *Server) CloseStreams() {
	s.streamsCtx()
	s.streams.cancel()
}

// wsAccept ouvre la connexion ; le contexte renvoyé s'annule à sa fermeture ou à l'arrêt du serveur.
func (s *Server) wsAccept(w http.ResponseWriter, r *http.Request) (*websocket.Conn, context.Context, context.CancelFunc, bool) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.AllowedOrigins})
	if err != nil {
		return nil, nil, nil, false // réponse d'erreur déjà écrite
	}
	c.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(s.streamsCtx(), cancel)
	go func() { // maintien de la connexion (proxys qui coupent les connexions inactives)
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, pcancel := context.WithTimeout(ctx, 15*time.Second)
				err := c.Ping(pctx)
				pcancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	return c, ctx, func() {
		stop()
		cancel()
		_ = c.CloseNow()
	}, true
}

func wsSend(ctx context.Context, c *websocket.Conn, v any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return wsjson.Write(ctx, c, v)
}

// wsEnd envoie le dernier message puis ferme proprement.
func wsEnd(ctx context.Context, c *websocket.Conn, v any) {
	_ = wsSend(ctx, c, v)
	_ = c.Close(websocket.StatusNormalClosure, "")
}

func wsError(ctx context.Context, c *websocket.Conn, msg string) {
	wsEnd(ctx, c, map[string]string{"type": "error", "message": msg})
}

// wsWriter envoie chaque écriture dans un message binaire.
type wsWriter struct {
	ctx context.Context
	c   *websocket.Conn
}

func (w wsWriter) Write(p []byte) (int, error) {
	if err := w.c.Write(w.ctx, websocket.MessageBinary, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// --- Événements de l'équipe ---

// teamEvents diffuse les changements d'état (déploiements, tâches, statuts des ressources, serveurs
// et proxys) des objets que l'utilisateur peut lire.
func (s *Server) teamEvents(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	team := teamFrom(r.Context()).Team.ID
	c, ctx, done, ok := s.wsAccept(w, r)
	if !ok {
		return
	}
	defer done()
	ctx = c.CloseRead(ctx)
	events, unsub := s.Hub.Subscribe(func(e store.Event) bool { return e.Team == team })
	defer unsub()
	if wsSend(ctx, c, map[string]string{"type": "ready"}) != nil {
		return
	}
	visible := map[string]bool{}
	reset := time.NewTicker(time.Minute) // droits relus régulièrement
	defer reset.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-reset.C:
			clear(visible)
		case e := <-events:
			perm, sc := "resource.read", rbac.Scope{Type: rbac.ScopeResource, ID: e.Resource}
			switch e.Kind {
			case store.EventResourceStatus:
				sc.ID = e.ID
			case store.EventTaskOutput, store.EventTaskStatus, store.EventServerStatus, store.EventProxyStatus:
				perm, sc = "server.read", rbac.Scope{Type: rbac.ScopeServer, ID: e.Server}
			}
			key := perm + "|" + sc.ID
			ok, seen := visible[key]
			if !seen {
				ok, _ = s.can(ctx, perm, sc)
				visible[key] = ok
			}
			if ok && wsSend(ctx, c, map[string]any{"type": "event", "event": e}) != nil {
				return
			}
		}
	}
}

// --- Logs de déploiement et sortie des tâches ---

func (s *Server) streamDeploymentLogs(w http.ResponseWriter, r *http.Request) {
	d, ok := s.appDeployment(w, r, "resource.logs.read")
	if !ok {
		return
	}
	debug := r.URL.Query().Get("debug") == "1"
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	c, ctx, done, ok := s.wsAccept(w, r)
	if !ok {
		return
	}
	defer done()
	ctx = c.CloseRead(ctx)
	wake, unsub := s.Hub.Subscribe(func(e store.Event) bool {
		return e.ID == d.ID && (e.Kind == store.EventDeploymentLog || e.Kind == store.EventDeploymentStatus)
	})
	defer unsub()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	status := ""
	for {
		// Statut lu avant les logs : un déploiement terminé n'écrit plus, la dernière lecture est complète.
		cur, err := s.Store.DeploymentByID(ctx, d.ID)
		if err != nil {
			wsError(ctx, c, "déploiement introuvable")
			return
		}
		for {
			logs, err := s.Store.DeploymentLogs(ctx, d.ID, after, debug, 1000)
			if err != nil {
				wsError(ctx, c, "lecture des logs impossible")
				return
			}
			if len(logs) > 0 {
				after = logs[len(logs)-1].ID
				if wsSend(ctx, c, map[string]any{"type": "logs", "logs": logs}) != nil {
					return
				}
			}
			if len(logs) < 1000 {
				break
			}
		}
		if cur.Status != status {
			status = cur.Status
			if wsSend(ctx, c, map[string]string{"type": "status", "status": status}) != nil {
				return
			}
		}
		if status != "queued" && status != "in_progress" {
			wsEnd(ctx, c, map[string]string{"type": "end", "status": status, "error": cur.Error})
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-tick.C:
		}
	}
}

func (s *Server) streamServerTask(w http.ResponseWriter, r *http.Request) {
	t, ok := s.serverTask(w, r)
	if !ok {
		return
	}
	c, ctx, done, ok := s.wsAccept(w, r)
	if !ok {
		return
	}
	defer done()
	ctx = c.CloseRead(ctx)
	wake, unsub := s.Hub.Subscribe(func(e store.Event) bool {
		return e.ID == t.ID && (e.Kind == store.EventTaskOutput || e.Kind == store.EventTaskStatus)
	})
	defer unsub()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	sent, status := 0, ""
	for {
		cur, err := s.Store.ServerTaskByID(ctx, t.ID)
		if err != nil {
			wsError(ctx, c, "tâche introuvable")
			return
		}
		if len(cur.Output) < sent { // sortie tronquée (1 Mo max.) : on renvoie tout
			sent = 0
			if wsSend(ctx, c, map[string]string{"type": "reset"}) != nil {
				return
			}
		}
		if len(cur.Output) > sent {
			if wsSend(ctx, c, map[string]string{"type": "output", "data": cur.Output[sent:]}) != nil {
				return
			}
			sent = len(cur.Output)
		}
		if cur.Status != status {
			status = cur.Status
			if wsSend(ctx, c, map[string]string{"type": "status", "status": status}) != nil {
				return
			}
		}
		if status != "queued" && status != "running" {
			wsEnd(ctx, c, map[string]string{"type": "end", "status": status, "error": cur.Error})
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-tick.C:
		}
	}
}

// --- Logs des conteneurs et du proxy suivis en direct ---

func streamTail(r *http.Request) int {
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail < 0 || tail > 5000 {
		return 100
	}
	if r.URL.Query().Get("tail") == "" {
		return 100
	}
	return tail
}

// follow ouvre la connexion et y copie la sortie de run (messages binaires) jusqu'à sa fin.
func (s *Server) follow(w http.ResponseWriter, r *http.Request, run func(ctx context.Context, w wsWriter) error) {
	c, ctx, done, ok := s.wsAccept(w, r)
	if !ok {
		return
	}
	defer done()
	ctx = c.CloseRead(ctx)
	err := run(ctx, wsWriter{ctx, c})
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		wsError(ctx, c, err.Error())
		return
	}
	wsEnd(ctx, c, map[string]string{"type": "end"})
}

// resourceStream : paramètres communs (conteneur, sous-service) validés avant l'ouverture.
func resourceStream(w http.ResponseWriter, r *http.Request) (service, container string, ok bool) {
	q := r.URL.Query()
	service, container = q.Get("app"), q.Get("container")
	if service != "" && !serviceRe.MatchString(service) {
		writeError(w, http.StatusBadRequest, "app invalide")
		return "", "", false
	}
	if container != "" && !containerRe.MatchString(container) {
		writeError(w, http.StatusBadRequest, "conteneur invalide")
		return "", "", false
	}
	return service, container, true
}

func (s *Server) streamResourceLogs(w http.ResponseWriter, r *http.Request, resourceID string) {
	service, container, ok := resourceStream(w, r)
	if !ok || !s.deployer(w) {
		return
	}
	tail := streamTail(r)
	s.follow(w, r, func(ctx context.Context, out wsWriter) error {
		return s.Deployer.FollowLogs(ctx, resourceID, service, container, tail, out)
	})
}

func (s *Server) streamApplicationLogs(w http.ResponseWriter, r *http.Request) {
	if a, ok := s.teamApp(w, r, "resource.logs.read"); ok {
		s.streamResourceLogs(w, r, a.ID)
	}
}

func (s *Server) streamDatabaseLogs(w http.ResponseWriter, r *http.Request) {
	if d, ok := s.teamDatabase(w, r, "resource.logs.read"); ok {
		s.streamResourceLogs(w, r, d.ID)
	}
}

func (s *Server) streamServiceLogs(w http.ResponseWriter, r *http.Request) {
	if svc, ok := s.teamService(w, r, "resource.logs.read"); ok {
		s.streamResourceLogs(w, r, svc.ID)
	}
}

func (s *Server) streamProxyLogs(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok || !s.deployer(w) {
		return
	}
	tail := streamTail(r)
	s.follow(w, r, func(ctx context.Context, out wsWriter) error { return s.Deployer.FollowProxyLogs(ctx, sv, tail, out) })
}

// --- Conteneurs ---

func (s *Server) listContainers(w http.ResponseWriter, r *http.Request, resourceID string) {
	if !s.deployer(w) {
		return
	}
	pr, _ := strconv.Atoi(r.URL.Query().Get("pr"))
	list, err := s.Deployer.Containers(r.Context(), resourceID, pr)
	if err != nil {
		writeError(w, http.StatusBadGateway, "serveur inaccessible : "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) applicationContainers(w http.ResponseWriter, r *http.Request) {
	if a, ok := s.teamApp(w, r, "resource.read"); ok {
		s.listContainers(w, r, a.ID)
	}
}

func (s *Server) databaseContainers(w http.ResponseWriter, r *http.Request) {
	if d, ok := s.teamDatabase(w, r, "resource.read"); ok {
		s.listContainers(w, r, d.ID)
	}
}

func (s *Server) serviceContainers(w http.ResponseWriter, r *http.Request) {
	if svc, ok := s.teamService(w, r, "resource.read"); ok {
		s.listContainers(w, r, svc.ID)
	}
}

// --- Terminal web ---

type terminalMsg struct {
	Type string `json:"type"` // input | resize
	Data string `json:"data"`
	Rows int    `json:"rows"`
	Cols int    `json:"cols"`
}

// terminal relie un shell distant à la connexion : messages binaires (ou {type: input}) vers l'entrée,
// sortie renvoyée en messages binaires, {type: resize, rows, cols} pour redimensionner.
func (s *Server) terminal(w http.ResponseWriter, r *http.Request, open func(ctx context.Context, o ssh.ShellOptions) (*ssh.Shell, string, error),
	audit func(container string)) {
	q := r.URL.Query()
	rows, _ := strconv.Atoi(q.Get("rows"))
	cols, _ := strconv.Atoi(q.Get("cols"))
	if rows <= 0 || rows > 1000 || cols <= 0 || cols > 1000 {
		rows, cols = 24, 80
	}
	c, ctx, done, ok := s.wsAccept(w, r)
	if !ok {
		return
	}
	defer done()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sh, container, err := open(ctx, ssh.ShellOptions{Rows: rows, Cols: cols})
	if err != nil {
		wsError(ctx, c, "ouverture du terminal impossible : "+err.Error())
		return
	}
	defer sh.Close()
	audit(container)
	if wsSend(ctx, c, map[string]string{"type": "ready", "container": container}) != nil {
		return
	}
	go func() {
		defer cancel()
		buf := make([]byte, 32<<10)
		for {
			n, err := sh.Stdout.Read(buf)
			if n > 0 && c.Write(ctx, websocket.MessageBinary, buf[:n]) != nil {
				return
			}
			if err != nil {
				break
			}
		}
		wsEnd(ctx, c, map[string]any{"type": "exit", "code": sh.Wait()})
	}()
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		if typ == websocket.MessageBinary {
			if _, err := sh.Stdin.Write(data); err != nil {
				return
			}
			continue
		}
		var m terminalMsg
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Type {
		case "input":
			if _, err := sh.Stdin.Write([]byte(m.Data)); err != nil {
				return
			}
		case "resize":
			_ = sh.Resize(m.Rows, m.Cols)
		}
	}
}

func (s *Server) serverTerminal(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.terminal")
	if !ok || !s.deployer(w) {
		return
	}
	if !sv.IsUsable {
		writeError(w, http.StatusConflict, "serveur non validé")
		return
	}
	s.terminal(w, r, func(ctx context.Context, o ssh.ShellOptions) (*ssh.Shell, string, error) {
		sh, err := s.Deployer.ServerShell(ctx, sv, o)
		return sh, "", err
	}, func(string) { s.auditTeam(r, "server.terminal.opened", "server", sv.ID, nil) })
}

func (s *Server) resourceTerminal(w http.ResponseWriter, r *http.Request, resourceID, kind string) {
	service, container, ok := resourceStream(w, r)
	if !ok || !s.deployer(w) {
		return
	}
	s.terminal(w, r, func(ctx context.Context, o ssh.ShellOptions) (*ssh.Shell, string, error) {
		return s.Deployer.ContainerShell(ctx, resourceID, service, container, o)
	}, func(c string) {
		s.auditTeam(r, kind+".terminal.opened", kind, resourceID, map[string]string{"container": c})
	})
}

func (s *Server) applicationTerminal(w http.ResponseWriter, r *http.Request) {
	if a, ok := s.teamApp(w, r, "resource.terminal"); ok {
		s.resourceTerminal(w, r, a.ID, "application")
	}
}

func (s *Server) databaseTerminal(w http.ResponseWriter, r *http.Request) {
	if d, ok := s.teamDatabase(w, r, "resource.terminal"); ok {
		s.resourceTerminal(w, r, d.ID, "database")
	}
}

func (s *Server) serviceTerminal(w http.ResponseWriter, r *http.Request) {
	if svc, ok := s.teamService(w, r, "resource.terminal"); ok {
		s.resourceTerminal(w, r, svc.ID, "service")
	}
}
