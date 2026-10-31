package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMCP(t *testing.T) {
	f := newAppFixture(t)
	alice, base := f.alice, f.base
	var app map[string]any
	expect(t, "application", alice.do("POST", base+"/applications", f.app(nil), &app), 201)
	appID := app["id"].(string)
	var tok struct {
		Token string `json:"token"`
	}
	expect(t, "jeton", alice.do("POST", "/me/tokens", map[string]any{"name": "mcp", "team_id": f.team, "scopes": []string{"read"}}, &tok), 201)
	mcp := newClient(t, f.e.srv)
	mcp.bearer = tok.Token
	rpc := func(method string, params any) (map[string]any, int) {
		t.Helper()
		var out map[string]any
		code := mcp.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}, &out)
		return out, code
	}
	if _, code := rpc("initialize", nil); code != 404 {
		t.Fatalf("désactivé par défaut : %d", code)
	}
	expect(t, "activation", alice.do("PATCH", "/admin/settings", map[string]any{"mcp_enabled": true}, nil), 200)
	expect(t, "session refusée", alice.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}, nil), 401)

	init, code := rpc("initialize", map[string]any{"protocolVersion": "2025-06-18"})
	if code != 200 || init["result"].(map[string]any)["serverInfo"].(map[string]any)["name"] != "nodra" {
		t.Fatalf("initialize : %d %v", code, init)
	}
	expect(t, "notification", mcp.do("POST", "/mcp", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}, nil), 202)
	list, _ := rpc("tools/list", nil)
	tools := list["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 8 || tools[0].(map[string]any)["inputSchema"] == nil {
		t.Fatalf("outils : %v", list)
	}
	call := func(name string, args map[string]any) (string, bool) {
		t.Helper()
		out, code := rpc("tools/call", map[string]any{"name": name, "arguments": args})
		if code != 200 || out["error"] != nil {
			t.Fatalf("%s : %d %v", name, code, out)
		}
		res := out["result"].(map[string]any)
		return res["content"].([]any)[0].(map[string]any)["text"].(string), res["isError"].(bool)
	}
	text, isErr := call("list_resources", map[string]any{"type": "application"})
	var res []map[string]any
	if isErr || json.Unmarshal([]byte(text), &res) != nil || len(res) != 1 || res[0]["id"] != appID {
		t.Fatalf("list_resources : %s", text)
	}
	if text, isErr := call("get_resource", map[string]any{"id": appID}); isErr || !strings.Contains(text, appID) {
		t.Fatalf("get_resource : %s", text)
	}
	// Jeton en lecture seule : le déploiement est refusé par la route REST.
	if text, isErr := call("resource_action", map[string]any{"id": appID, "action": "restart"}); !isErr || !strings.Contains(text, "error") {
		t.Fatalf("scope read : %s", text)
	}
	if text, isErr := call("get_resource", map[string]any{"id": "pas-un-uuid"}); !isErr {
		t.Fatalf("argument invalide : %s", text)
	}
	if out, _ := rpc("tools/call", map[string]any{"name": "rm_rf"}); out["error"] == nil {
		t.Fatal("outil inconnu accepté")
	}
	if out, _ := rpc("resources/list", nil); out["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatalf("méthode inconnue : %v", out)
	}
}
