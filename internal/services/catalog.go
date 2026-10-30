// Package services gère les services « one-click » : catalogue de modèles docker-compose (repris de
// Coolify) et analyse des variables magiques (SERVICE_URL_*, SERVICE_PASSWORD_*…) qui génèrent
// domaines et secrets à la création.
package services

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed templates/*.yaml
var templatesFS embed.FS

type Template struct {
	Slug          string   `json:"slug"`
	Name          string   `json:"name"`
	Slogan        string   `json:"slogan"`
	Documentation string   `json:"documentation"`
	Category      string   `json:"category"`
	Tags          []string `json:"tags"`
	Logo          string   `json:"logo"`
	Port          string   `json:"port"`
	Ignore        bool     `json:"-"`
	Compose       string   `json:"compose,omitempty"`
}

// ParseTemplate lit l'en-tête de commentaires (# clé: valeur) puis le docker-compose.
func ParseTemplate(slug, content string) Template {
	t := Template{Slug: slug, Name: humanize(slug)}
	var body []string
	header := true
	for _, line := range strings.Split(content, "\n") {
		if header && strings.HasPrefix(line, "#") {
			k, v, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "#")), ":")
			if !ok {
				continue
			}
			v = strings.Trim(strings.TrimSpace(v), `"`)
			switch strings.TrimSpace(k) {
			case "documentation":
				t.Documentation = v
			case "slogan":
				t.Slogan = v
			case "category":
				t.Category = v
			case "tags":
				for _, tag := range strings.Split(v, ",") {
					if tag = strings.TrimSpace(tag); tag != "" {
						t.Tags = append(t.Tags, tag)
					}
				}
			case "logo":
				t.Logo = v
			case "port":
				t.Port = v
			case "ignore":
				t.Ignore, _ = strconv.ParseBool(v)
			}
			continue
		}
		if strings.TrimSpace(line) != "" {
			header = false
		}
		if !header {
			body = append(body, line)
		}
	}
	t.Compose = strings.TrimSpace(strings.Join(body, "\n")) + "\n"
	return t
}

func humanize(slug string) string {
	words := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

var (
	catalogOnce sync.Once
	catalog     []Template
	bySlug      map[string]Template
)

func load() {
	bySlug = map[string]Template{}
	entries, _ := fs.ReadDir(templatesFS, "templates")
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := templatesFS.ReadFile("templates/" + e.Name())
		if err != nil {
			continue
		}
		t := ParseTemplate(strings.TrimSuffix(e.Name(), ".yaml"), string(b))
		if t.Ignore {
			continue
		}
		if _, err := Analyze(t.Compose); err != nil {
			continue // YAML que docker compose refuserait aussi (ex. fusion d'une liste dans une table)
		}
		bySlug[t.Slug] = t
		catalog = append(catalog, t)
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].Slug < catalog[j].Slug })
}

// Catalog renvoie les modèles disponibles (sans le contenu compose), filtrés par recherche et catégorie.
func Catalog(query, category string) []Template {
	catalogOnce.Do(load)
	query = strings.ToLower(strings.TrimSpace(query))
	out := []Template{}
	for _, t := range catalog {
		if category != "" && !strings.EqualFold(t.Category, category) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(t.Slug+" "+t.Name+" "+t.Slogan+" "+strings.Join(t.Tags, " ")), query) {
			continue
		}
		t.Compose = ""
		out = append(out, t)
	}
	return out
}

// Get renvoie un modèle complet.
func Get(slug string) (Template, error) {
	catalogOnce.Do(load)
	t, ok := bySlug[slug]
	if !ok {
		return Template{}, fmt.Errorf("modèle inconnu : %s", slug)
	}
	return t, nil
}

// Categories renvoie les catégories du catalogue.
func Categories() []string {
	catalogOnce.Do(load)
	seen := map[string]bool{}
	var out []string
	for _, t := range catalog {
		if t.Category != "" && !seen[t.Category] {
			seen[t.Category] = true
			out = append(out, t.Category)
		}
	}
	sort.Strings(out)
	return out
}
