// Package envvars lit les fichiers .env et résout les variables des ressources
// (références aux variables partagées {{team.X}}, {{project.X}}, {{environment.X}}, {{server.X}}).
package envvars

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
)

var KeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,254}$`)

type Pair struct {
	Key, Value string
	Multiline  bool
}

// ParseDotenv lit un contenu au format .env : commentaires, `export`, guillemets simples
// (littéral) ou doubles (échappements \n, \", valeurs sur plusieurs lignes).
func ParseDotenv(content string) ([]Pair, error) {
	var out []Pair
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		raw = strings.TrimPrefix(raw, "export ")
		key, val, ok := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if !ok || !KeyRe.MatchString(key) {
			return nil, fmt.Errorf("ligne %d : clé invalide", line)
		}
		val = strings.TrimLeft(val, " \t")
		p := Pair{Key: key}
		switch {
		case strings.HasPrefix(val, `"`):
			body := val[1:]
			for !closedDouble(body) {
				if !sc.Scan() {
					return nil, fmt.Errorf("ligne %d : guillemet non fermé", line)
				}
				line++
				body += "\n" + sc.Text()
				p.Multiline = true
			}
			end := closingIndex(body)
			p.Value = unescape(body[:end])
		case strings.HasPrefix(val, `'`):
			body := val[1:]
			for !strings.Contains(body, "'") {
				if !sc.Scan() {
					return nil, fmt.Errorf("ligne %d : apostrophe non fermée", line)
				}
				line++
				body += "\n" + sc.Text()
				p.Multiline = true
			}
			p.Value = body[:strings.Index(body, "'")]
		default:
			if i := strings.Index(val, " #"); i >= 0 {
				val = val[:i]
			}
			p.Value = strings.TrimSpace(val)
		}
		out = append(out, p)
	}
	return out, sc.Err()
}

func closingIndex(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '"' {
			return i
		}
	}
	return -1
}

func closedDouble(s string) bool { return closingIndex(s) >= 0 }

func unescape(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\"`, `"`, `\\`, `\`, `\t`, "\t")
	return r.Replace(s)
}

var refRe = regexp.MustCompile(`\{\{\s*(team|project|environment|server)\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// Interpolate remplace les références aux variables partagées. shared est indexé par "team.CLÉ"…
// Les références introuvables sont remplacées par une chaîne vide et renvoyées dans missing.
func Interpolate(value string, shared map[string]string) (string, []string) {
	var missing []string
	out := refRe.ReplaceAllStringFunc(value, func(m string) string {
		sub := refRe.FindStringSubmatch(m)
		v, ok := shared[sub[1]+"."+sub[2]]
		if !ok {
			missing = append(missing, sub[1]+"."+sub[2])
		}
		return v
	})
	return out, missing
}

// EscapeCompose protège les $ d'une valeur littérale pour Docker Compose ($ → $$).
func EscapeCompose(v string) string { return strings.ReplaceAll(v, "$", "$$") }
