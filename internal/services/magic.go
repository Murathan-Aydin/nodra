package services

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Types de valeurs générées (mêmes noms que Coolify).
var generators = map[string]func() string{
	"PASSWORD":               func() string { return randomString(24, alnum) },
	"PASSWORD_64":            func() string { return randomString(64, alnum) },
	"PASSWORDWITHSYMBOLS":    func() string { return randomString(24, alnum+"!#%&*+-=?@^_") },
	"PASSWORDWITHSYMBOLS_64": func() string { return randomString(64, alnum+"!#%&*+-=?@^_") },
	"BASE64":                 func() string { return randomString(32, alnum) }, // chaîne aléatoire, pas du base64 (comme Coolify)
	"BASE64_32":              func() string { return randomString(32, alnum) },
	"BASE64_64":              func() string { return randomString(64, alnum) },
	"BASE64_128":             func() string { return randomString(128, alnum) },
	"REALBASE64":             func() string { return realBase64(32) },
	"REALBASE64_32":          func() string { return realBase64(32) },
	"REALBASE64_64":          func() string { return realBase64(64) },
	"REALBASE64_128":         func() string { return realBase64(128) },
	"HEX_32":                 func() string { return randomHex(16) },
	"HEX_64":                 func() string { return randomHex(32) },
	"HEX_128":                func() string { return randomHex(64) },
	"USER":                   func() string { return randomString(16, alnum) },
	"LOWERCASEUSER":          func() string { return strings.ToLower(randomString(16, alnum)) },
}

const alnum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(n int, chars string) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[idx.Int64()]
	}
	return string(b)
}

func realBase64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// kinds triés du plus long au plus court pour reconnaître SERVICE_PASSWORD_64_X avant SERVICE_PASSWORD_X.
var kinds = func() []string {
	k := []string{"URL", "FQDN", "SUPABASEANON", "SUPABASESERVICE"}
	for g := range generators {
		k = append(k, g)
	}
	sort.Slice(k, func(i, j int) bool { return len(k[i]) > len(k[j]) || (len(k[i]) == len(k[j]) && k[i] < k[j]) })
	return k
}()

// MagicVar : variable magique d'un compose.
type MagicVar struct {
	Name     string // nom complet, ex. SERVICE_PASSWORD_64_PG
	Kind     string // URL, FQDN, PASSWORD_64…
	ID       string // identifiant, ex. PG
	Port     string // port du conteneur (URL/FQDN), facultatif
	Declared string // service qui déclare la variable sans valeur (service exposé)
}

// ParseMagic reconnaît une variable magique ; ok=false sinon.
func ParseMagic(name string) (MagicVar, bool) {
	rest, ok := strings.CutPrefix(name, "SERVICE_")
	if !ok {
		return MagicVar{}, false
	}
	for _, k := range kinds {
		if id, ok := strings.CutPrefix(rest, k+"_"); ok && id != "" {
			v := MagicVar{Name: name, Kind: k, ID: id}
			if k == "URL" || k == "FQDN" {
				if i := strings.LastIndex(id, "_"); i > 0 {
					if _, err := strconv.Atoi(id[i+1:]); err == nil {
						v.ID, v.Port = id[:i], id[i+1:]
					}
				}
			}
			return v, true
		}
	}
	return MagicVar{}, false
}

// Variable configurable (non magique) référencée par ${VAR} ou ${VAR:-défaut}.
type UserVar struct {
	Key      string
	Default  string
	Required bool
}

// Analysis : résultat de l'analyse d'un compose de service.
type Analysis struct {
	Services []string
	Images   map[string]string
	Magic    map[string]MagicVar // par nom complet
	User     []UserVar
	Exposed  map[string]MagicVar // service → variable URL/FQDN qui l'expose
	Excluded map[string]bool     // services marqués exclude_from_hc (exclus du statut)
}

var nameRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)`)

var refRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?])([^}]*))?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// Analyze parcourt le compose : services, variables magiques et variables configurables.
func Analyze(raw string) (*Analysis, error) {
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("docker-compose invalide : %w", err)
	}
	if len(doc.Services) == 0 {
		return nil, fmt.Errorf("docker-compose sans services")
	}
	a := &Analysis{Images: map[string]string{}, Magic: map[string]MagicVar{}, Exposed: map[string]MagicVar{}, Excluded: map[string]bool{}}
	for name, svc := range doc.Services {
		a.Services = append(a.Services, name)
		if ex, _ := svc["exclude_from_hc"].(bool); ex {
			a.Excluded[name] = true
		}
		if img, ok := svc["image"].(string); ok {
			a.Images[name] = img
		}
		// Variables déclarées sans valeur dans l'environnement du service.
		for _, key := range bareEnvKeys(svc["environment"]) {
			if m, ok := ParseMagic(key); ok && (m.Kind == "URL" || m.Kind == "FQDN") {
				m.Declared = name
				a.Magic[m.Name] = m
				if _, set := a.Exposed[name]; !set {
					a.Exposed[name] = m
				}
			}
		}
	}
	sort.Strings(a.Services)
	user := map[string]UserVar{}
	for _, idx := range refRe.FindAllStringSubmatchIndex(raw, -1) {
		if idx[0] > 0 && raw[idx[0]-1] == '$' {
			continue // $$VAR : échappement compose, variable lue dans le conteneur
		}
		sub := func(n int) string {
			if idx[2*n] < 0 {
				return ""
			}
			return raw[idx[2*n]:idx[2*n+1]]
		}
		key, op, def := sub(1), sub(2), sub(3)
		if key == "" {
			key = sub(4)
		}
		if mv, ok := ParseMagic(key); ok {
			if _, known := a.Magic[mv.Name]; !known {
				a.Magic[mv.Name] = mv
			}
			continue
		}
		u := user[key]
		u.Key = key
		switch op {
		case ":-", "-":
			if !strings.Contains(def, "$") { // défaut imbriqué (${A:-${B}}) : laissé à docker compose
				u.Default = def
			}
		case ":?", "?":
			u.Required = true
		}
		user[key] = u
	}
	// Variables magiques imbriquées dans une valeur par défaut (${A:-${SERVICE_URL_X}}).
	for _, idx := range nameRe.FindAllStringSubmatchIndex(raw, -1) {
		if idx[0] > 0 && raw[idx[0]-1] == '$' {
			continue
		}
		if mv, ok := ParseMagic(raw[idx[2]:idx[3]]); ok {
			if _, known := a.Magic[mv.Name]; !known {
				a.Magic[mv.Name] = mv
			}
		}
	}
	// Une variable URL non déclarée expose le service du même nom (comme Coolify).
	for _, mv := range a.Magic {
		if (mv.Kind != "URL" && mv.Kind != "FQDN") || mv.Declared != "" {
			continue
		}
		for _, s := range a.Services {
			if envID(s) == mv.ID {
				if _, set := a.Exposed[s]; !set {
					mv.Declared = s
					a.Exposed[s] = mv
					a.Magic[mv.Name] = mv
				}
			}
		}
	}
	for _, u := range user {
		a.User = append(a.User, u)
	}
	sort.Slice(a.User, func(i, j int) bool { return a.User[i].Key < a.User[j].Key })
	return a, nil
}

// envID : nom de service converti en identifiant de variable (my-app → MY_APP).
func envID(service string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(service))
}

// bareEnvKeys renvoie les clés d'environnement sans valeur (« - KEY » ou « KEY: »).
func bareEnvKeys(env any) []string {
	var out []string
	switch e := env.(type) {
	case []any:
		for _, item := range e {
			if s, ok := item.(string); ok && !strings.Contains(s, "=") {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case map[string]any:
		for k, v := range e {
			if v == nil || v == "" {
				out = append(out, k)
			}
		}
	}
	return out
}

// Generate produit la valeur d'une variable magique secrète (les URL sont calculées au déploiement).
// existing permet de signer les JWT Supabase avec SERVICE_PASSWORD_JWT.
func Generate(m MagicVar, existing map[string]string) (string, bool) {
	if g, ok := generators[m.Kind]; ok {
		return g(), true
	}
	if m.Kind == "SUPABASEANON" || m.Kind == "SUPABASESERVICE" {
		secret := existing["SERVICE_PASSWORD_JWT"]
		if secret == "" {
			return "", false
		}
		role := map[string]string{"SUPABASEANON": "anon", "SUPABASESERVICE": "service_role"}[m.Kind]
		return supabaseJWT(secret, role, time.Now()), true
	}
	return "", false
}

func supabaseJWT(secret, role string, now time.Time) string {
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"role": role, "iss": "supabase", "iat": now.Unix(), "exp": now.AddDate(100, 0, 0).Unix()})
	signing := header + "." + enc.EncodeToString(claims)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	return signing + "." + enc.EncodeToString(mac.Sum(nil))
}

// SortedMagic renvoie les variables magiques à générer, celles dont dépendent les JWT en premier.
func (a *Analysis) SortedMagic() []MagicVar {
	out := make([]MagicVar, 0, len(a.Magic))
	for _, m := range a.Magic {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		ji := strings.HasPrefix(out[i].Kind, "SUPABASE")
		jj := strings.HasPrefix(out[j].Kind, "SUPABASE")
		if ji != jj {
			return !ji
		}
		return out[i].Name < out[j].Name
	})
	return out
}
