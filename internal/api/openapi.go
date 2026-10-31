package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/Murathan-Aydin/nodra/docs"
	"github.com/Murathan-Aydin/nodra/internal/version"
)

// Spécification OpenAPI 3 générée à partir des routes réellement enregistrées (chi.Walk) et des tables
// de docs/API.md (résumé, permission, section) : elle suit le code sans être maintenue à la main.

type docRow struct {
	section string
	route   string // normalisée : paramètres « {} », sans requête
	cells   []string
	headers []string
}

var (
	docRouteRe = regexp.MustCompile("`([^`]+)`")
	paramRe    = regexp.MustCompile(`\{[^}]*\}`)
	methodRe   = regexp.MustCompile(`\b(GET|POST|PUT|PATCH|DELETE)\b`)
)

func normalizeRoute(p string) string {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSuffix(strings.TrimSpace(p), "/")
	return paramRe.ReplaceAllString(p, "{}")
}

// parseAPIDoc lit les tableaux « | Méthode | Route | … | » de API.md, indexés par méthode. Les routes
// relatives sont aussi rattachées au préfixe de leur section (« ## Bases (`/teams/{team}/databases`) »),
// et les tableaux sans colonne Méthode (WebSocket) sont considérés comme des GET.
func parseAPIDoc(md string) map[string][]docRow {
	out := map[string][]docRow{}
	var section, prefix string
	var headers []string
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			section = strings.TrimSpace(strings.TrimLeft(line, "#"))
			prefix = ""
			if m := docRouteRe.FindStringSubmatch(section); m != nil && strings.HasPrefix(m[1], "/") {
				prefix = strings.TrimSuffix(strings.Split(m[1], "…")[0], "/")
			}
			if i := strings.Index(section, " ("); i > 0 {
				section = section[:i]
			}
			headers = nil
			continue
		}
		if !strings.HasPrefix(line, "|") {
			headers = nil
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if headers == nil {
			headers = cells
			continue
		}
		if len(cells) < 2 || strings.HasPrefix(cells[0], "---") {
			continue
		}
		methods := methodRe.FindAllString(cells[0], -1)
		routeCell, rest, hdr := 1, cells[2:], headers[min(2, len(headers)):]
		if len(methods) == 0 && docRouteRe.MatchString(cells[0]) { // tableau sans méthode (WebSocket)
			methods, routeCell, rest, hdr = []string{"GET"}, 0, cells[1:], headers[min(1, len(headers)):]
		}
		for _, m := range docRouteRe.FindAllStringSubmatch(cells[routeCell], -1) {
			if !strings.HasPrefix(m[1], "/") {
				continue
			}
			routes := []string{m[1]}
			if prefix != "" && !strings.HasPrefix(m[1], prefix) {
				routes = append(routes, prefix+m[1])
			}
			for _, rt := range routes {
				row := docRow{section: section, route: normalizeRoute(rt), cells: rest, headers: hdr}
				for _, meth := range methods {
					out[meth] = append(out[meth], row)
				}
			}
		}
	}
	return out
}

// bestDoc : ligne dont la route est le plus long suffixe de la route réelle.
func bestDoc(rows []docRow, path string) *docRow {
	norm := normalizeRoute(path)
	var best *docRow
	for i := range rows {
		r := &rows[i]
		if r.route != "" && strings.HasSuffix(norm, r.route) && (best == nil || len(r.route) > len(best.route)) {
			best = r
		}
	}
	return best
}

func fallbackTag(path string) string {
	p := strings.TrimPrefix(path, "/api/v1")
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) >= 3 && parts[0] == "teams" {
		return parts[2]
	}
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return "racine"
}

func buildOpenAPI(r chi.Routes) map[string]any {
	doc := parseAPIDoc(docs.API)
	paths := map[string]map[string]any{}
	tagSet := map[string]bool{}
	// Routes enregistrées pour toutes les méthodes (r.Handle) : repérées pour ne garder que les méthodes documentées.
	count := map[string]int{}
	_ = chi.Walk(r, func(_, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		count[route]++
		return nil
	})
	handleAll := map[string]bool{}
	for rt, n := range count {
		if n >= 8 {
			handleAll[strings.TrimSuffix(strings.ReplaceAll(rt, "/*", ""), "/")] = true
		}
	}
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/api/v1") && !strings.HasPrefix(route, "/webhooks") {
			return nil
		}
		route = strings.TrimSuffix(strings.ReplaceAll(route, "/*", ""), "/")
		if !methodRe.MatchString(method) || len(method) > 6 {
			return nil
		}
		if handleAll[route] && bestDoc(doc[method], route) == nil {
			return nil
		}
		op := map[string]any{"responses": map[string]any{"default": map[string]any{"description": "Voir la description (erreurs : {\"error\": \"message\"})"}}}
		tag := fallbackTag(route)
		if d := bestDoc(doc[method], route); d != nil {
			tag = d.section
			var desc []string
			for i, c := range d.cells {
				if c != "" && c != "—" {
					op["summary"] = shorten(stripMD(c), 120)
				}
				if i < len(d.headers) && c != "" && c != "—" {
					desc = append(desc, "**"+d.headers[i]+"** : "+c)
				}
			}
			op["description"] = strings.Join(desc, "\n\n")
		}
		op["tags"] = []string{tag}
		tagSet[tag] = true
		var params []map[string]any
		for _, p := range paramRe.FindAllString(route, -1) {
			params = append(params, map[string]any{"name": strings.Trim(p, "{}"), "in": "path", "required": true, "schema": map[string]string{"type": "string"}})
		}
		if params != nil {
			op["parameters"] = params
		}
		if method == "POST" || method == "PUT" || method == "PATCH" {
			op["requestBody"] = map[string]any{"required": false, "content": map[string]any{"application/json": map[string]any{"schema": map[string]string{"type": "object"}}}}
		}
		if paths[route] == nil {
			paths[route] = map[string]any{}
		}
		paths[route][strings.ToLower(method)] = op
		return nil
	})
	tags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	tagList := make([]map[string]string, len(tags))
	for i, t := range tags {
		tagList[i] = map[string]string{"name": t}
	}
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "API Nodra",
			"version":     version.Version,
			"description": "Générée à partir des routes du serveur et de docs/API.md. Authentification : cookie de session (navigateur) ou `Authorization: Bearer ndt_…`.",
		},
		"servers": []map[string]string{{"url": "/"}},
		"tags":    tagList,
		"paths":   paths,
		"components": map[string]any{"securitySchemes": map[string]any{
			"session": map[string]string{"type": "apiKey", "in": "cookie", "name": "nodra_session"},
			"jeton":   map[string]string{"type": "http", "scheme": "bearer"},
		}},
		"security": []map[string][]string{{"session": {}}, {"jeton": {}}},
	}
}

func stripMD(s string) string { return strings.NewReplacer("`", "", "**", "").Replace(s) }

func shorten(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// openAPIHandler sert la spécification (calculée une fois) et la page Swagger UI.
func openAPIHandlers(r chi.Routes) (spec, ui http.HandlerFunc) {
	var once sync.Once
	var body []byte
	spec = func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() { body, _ = json.Marshal(buildOpenAPI(r)) })
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
	ui = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(swaggerHTML))
	}
	return spec, ui
}

// Swagger UI (CDN) : les requêtes « Try it out » passent par le cookie de session, avec l'en-tête anti-CSRF.
const swaggerHTML = `<!doctype html>
<html lang="fr">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>API Nodra — Swagger</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css">
</head>
<body>
<div id="swagger"></div>
<script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
<script>
SwaggerUIBundle({
  url: "/api/openapi.json",
  dom_id: "#swagger",
  deepLinking: true,
  docExpansion: "none",
  filter: true,
  persistAuthorization: true,
  requestInterceptor: (req) => { req.headers["X-Requested-With"] = "nodra"; return req },
})
</script>
</body>
</html>`
