package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/version"
)

// Serveur MCP (Model Context Protocol, transport « Streamable HTTP », réponses JSON) : outils pour
// assistants IA, utilisables avec un jeton API (équipe du jeton). Chaque outil rejoue la route REST
// correspondante pour le même appelant : droits, scopes du jeton, approbations et audit identiques.

const mcpProtocol = "2025-06-18"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// mcpTool : outil exposé, traduit en requête REST (chemin relatif à l'équipe du jeton).
type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	call        func(s *Server, ctx context.Context, args map[string]any, team string) (method, path string, body any, err error)
}

func schema(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func strArg(args map[string]any, k string) string {
	v, _ := args[k].(string)
	return strings.TrimSpace(v)
}

// idArg : identifiant obligatoire (UUID) d'un argument.
func idArg(args map[string]any, k string) (string, error) {
	v := strArg(args, k)
	if !isUUID(v) {
		return "", fmt.Errorf("%s : identifiant (UUID) requis", k)
	}
	return v, nil
}

var mcpTools = []mcpTool{
	{Name: "list_projects", Description: "Liste les projets de l'équipe et leurs environnements.", InputSchema: schema(map[string]any{}),
		call: func(_ *Server, _ context.Context, _ map[string]any, team string) (string, string, any, error) {
			return "GET", "/teams/" + team + "/projects", nil, nil
		}},
	{Name: "list_servers", Description: "Liste les serveurs de l'équipe (état, IP, proxy).", InputSchema: schema(map[string]any{}),
		call: func(_ *Server, _ context.Context, _ map[string]any, team string) (string, string, any, error) {
			return "GET", "/teams/" + team + "/servers", nil, nil
		}},
	{Name: "list_resources", Description: "Liste les applications, bases de données et services (filtres facultatifs).",
		InputSchema: schema(map[string]any{
			"type": map[string]any{"type": "string", "enum": []string{"application", "database", "service"}},
			"tag":  map[string]any{"type": "string", "description": "nom de tag"},
		}),
		call: func(_ *Server, _ context.Context, args map[string]any, team string) (string, string, any, error) {
			q := url.Values{}
			for _, k := range []string{"type", "tag"} {
				if v := strArg(args, k); v != "" {
					q.Set(k, v)
				}
			}
			return "GET", "/teams/" + team + "/resources?" + q.Encode(), nil, nil
		}},
	{Name: "get_resource", Description: "Détail d'une ressource (configuration, domaines, statut).",
		InputSchema: schema(map[string]any{"id": map[string]any{"type": "string"}}, "id"),
		call: func(s *Server, ctx context.Context, args map[string]any, team string) (string, string, any, error) {
			p, err := s.mcpResourcePath(ctx, args, team)
			return "GET", p, nil, err
		}},
	{Name: "deploy", Description: "Déploie des applications ou démarre des bases et services, par identifiants et/ou tags.",
		InputSchema: schema(map[string]any{
			"resources":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"tags":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"force_rebuild": map[string]any{"type": "boolean"},
		}),
		call: func(_ *Server, _ context.Context, args map[string]any, team string) (string, string, any, error) {
			return "POST", "/teams/" + team + "/deploy", args, nil
		}},
	{Name: "resource_action", Description: "Démarre, arrête ou redémarre une ressource.",
		InputSchema: schema(map[string]any{"id": map[string]any{"type": "string"},
			"action": map[string]any{"type": "string", "enum": []string{"start", "stop", "restart"}}}, "id", "action"),
		call: func(s *Server, ctx context.Context, args map[string]any, team string) (string, string, any, error) {
			action := strArg(args, "action")
			if action != "start" && action != "stop" && action != "restart" {
				return "", "", nil, fmt.Errorf("action : start, stop ou restart")
			}
			p, err := s.mcpResourcePath(ctx, args, team)
			return "POST", p + "/" + action, nil, err
		}},
	{Name: "container_logs", Description: "Dernières lignes de logs des conteneurs d'une ressource.",
		InputSchema: schema(map[string]any{"id": map[string]any{"type": "string"}, "lines": map[string]any{"type": "integer"}}, "id"),
		call: func(s *Server, ctx context.Context, args map[string]any, team string) (string, string, any, error) {
			p, err := s.mcpResourcePath(ctx, args, team)
			lines := 100
			if v, ok := args["lines"].(float64); ok && v > 0 && v <= 5000 {
				lines = int(v)
			}
			return "GET", p + "/logs?lines=" + strconv.Itoa(lines), nil, err
		}},
	{Name: "list_deployments", Description: "Déploiements récents de l'équipe (statut facultatif).",
		InputSchema: schema(map[string]any{"status": map[string]any{"type": "string",
			"enum": []string{"queued", "in_progress", "finished", "failed", "cancelled"}}}),
		call: func(_ *Server, _ context.Context, args map[string]any, team string) (string, string, any, error) {
			return "GET", "/teams/" + team + "/deployments?status=" + url.QueryEscape(strArg(args, "status")), nil, nil
		}},
	{Name: "deployment_logs", Description: "Logs d'un déploiement d'application.",
		InputSchema: schema(map[string]any{"application_id": map[string]any{"type": "string"},
			"deployment_id": map[string]any{"type": "string"}}, "application_id", "deployment_id"),
		call: func(_ *Server, _ context.Context, args map[string]any, team string) (string, string, any, error) {
			app, err := idArg(args, "application_id")
			if err != nil {
				return "", "", nil, err
			}
			dep, err := idArg(args, "deployment_id")
			return "GET", "/teams/" + team + "/applications/" + app + "/deployments/" + dep + "/logs", nil, err
		}},
}

// mcpResourcePath : chemin REST d'une ressource de l'équipe d'après son type.
func (s *Server) mcpResourcePath(ctx context.Context, args map[string]any, team string) (string, error) {
	id, err := idArg(args, "id")
	if err != nil {
		return "", err
	}
	typ, err := s.Store.TeamResourceType(ctx, team, id)
	if err != nil {
		return "", fmt.Errorf("ressource introuvable")
	}
	return "/teams/" + team + "/" + typ + "s/" + id, nil
}

func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	tok := tokenFrom(r.Context())
	if tok == nil || tok.TeamID == nil {
		writeError(w, http.StatusUnauthorized, "serveur MCP : jeton API rattaché à une équipe requis")
		return
	}
	if inst, err := s.instance(r.Context()); err != nil || !inst.MCPEnabled {
		writeError(w, http.StatusNotFound, "serveur MCP désactivé (réglage mcp_enabled de l'instance)")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "flux SSE non proposé : envoyez les messages JSON-RPC en POST")
		return
	}
	var req rpcRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.JSONRPC != "2.0" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"jsonrpc": "2.0", "error": rpcError{-32700, "JSON-RPC 2.0 attendu"}})
		return
	}
	if len(req.ID) == 0 { // notification (ex. notifications/initialized) : pas de réponse
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result, rerr := s.mcpDispatch(r, &req, *tok.TeamID)
	resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	if rerr != nil {
		resp["error"] = rerr
	} else {
		resp["result"] = result
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) mcpDispatch(r *http.Request, req *rpcRequest, team string) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{"protocolVersion": mcpProtocol, "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo":   map[string]any{"name": "nodra", "version": version.Version},
			"instructions": "Outils Nodra : ressources, serveurs, déploiements et logs de l'équipe du jeton API."}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools}, nil
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "paramètres invalides"}
		}
		for _, t := range mcpTools {
			if t.Name != p.Name {
				continue
			}
			if p.Arguments == nil {
				p.Arguments = map[string]any{}
			}
			method, path, body, err := t.call(s, r.Context(), p.Arguments, team)
			if err != nil {
				return mcpText(err.Error(), true), nil
			}
			var raw []byte
			if body != nil {
				raw, _ = json.Marshal(body)
			}
			rec := s.subRequest(r, method, "/api/v1"+path, raw)
			return mcpText(rec.Body.String(), rec.Code >= 400), nil
		}
		return nil, &rpcError{-32602, "outil inconnu : " + p.Name}
	}
	return nil, &rpcError{-32601, "méthode inconnue : " + req.Method}
}

func mcpText(text string, isError bool) map[string]any {
	return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}, "isError": isError}
}
