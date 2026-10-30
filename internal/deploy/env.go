package deploy

import (
	"net/url"
	"sort"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/envvars"
)

// Var : variable déchiffrée d'une ressource.
type Var struct {
	Key                      string
	Value                    string
	Buildtime, Runtime       bool
	Preview, Literal, Secret bool
}

// Env : variables résolues pour un déploiement.
type Env struct {
	Runtime map[string]string
	Build   map[string]string
	Literal map[string]bool
	Secrets []string // valeurs à masquer dans les logs
	Missing []string // références à des variables partagées introuvables
}

// Defaults : variables fournies par Nodra (équivalent des COOLIFY_* de Coolify).
type Defaults struct {
	ResourceID, ContainerName, Branch, Commit, Port string
	Domains                                         []string
	Nixpacks, IncludeInBuild, CommitInBuild         bool
}

// BuildEnv résout les variables : valeurs de prévisualisation prioritaires pour une PR, interpolation
// des variables partagées (sauf variables littérales), variables par défaut de Nodra.
func BuildEnv(vars []Var, shared map[string]string, preview bool, d Defaults) Env {
	e := Env{Runtime: map[string]string{}, Build: map[string]string{}, Literal: map[string]bool{}}
	chosen := map[string]Var{}
	for _, v := range vars {
		if v.Preview && !preview {
			continue
		}
		if cur, ok := chosen[v.Key]; ok && cur.Preview && !v.Preview {
			continue // la valeur de preview l'emporte
		}
		chosen[v.Key] = v
	}
	keys := make([]string, 0, len(chosen))
	for k := range chosen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := chosen[k]
		val := v.Value
		if !v.Literal {
			var missing []string
			val, missing = envvars.Interpolate(val, shared)
			e.Missing = append(e.Missing, missing...)
		} else {
			e.Literal[k] = true
		}
		if v.Runtime {
			e.Runtime[k] = val
		}
		if v.Buildtime {
			e.Build[k] = val
		}
		if v.Secret && val != "" {
			e.Secrets = append(e.Secrets, val)
		}
	}
	urls, hosts := make([]string, 0, len(d.Domains)), make([]string, 0, len(d.Domains))
	for _, dom := range d.Domains {
		if u, err := url.Parse(dom); err == nil {
			urls = append(urls, u.Scheme+"://"+u.Host)
			hosts = append(hosts, u.Host)
		}
	}
	defaults := map[string]string{
		"NODRA_RESOURCE_UUID": d.ResourceID, "NODRA_CONTAINER_NAME": d.ContainerName, "NODRA_BRANCH": d.Branch,
		"NODRA_FQDN": strings.Join(hosts, ","), "NODRA_URL": strings.Join(urls, ","), "SOURCE_COMMIT": d.Commit,
	}
	if d.Nixpacks {
		defaults["HOST"] = "0.0.0.0"
		if d.Port != "" {
			defaults["PORT"] = d.Port
		}
	}
	for k, v := range defaults {
		if _, set := e.Runtime[k]; !set { // une variable définie par l'utilisateur l'emporte
			e.Runtime[k] = v
		}
		inBuild := d.IncludeInBuild || k == "PORT" || k == "HOST"
		if k == "SOURCE_COMMIT" && !d.CommitInBuild {
			inBuild = false // éviterait d'invalider le cache de build à chaque commit
		}
		if _, set := e.Build[k]; inBuild && !set {
			e.Build[k] = v
		}
	}
	return e
}

// Redact masque les valeurs secrètes dans une ligne de log.
func Redact(line string, secrets []string) string {
	for _, s := range secrets {
		if len(s) >= 4 {
			line = strings.ReplaceAll(line, s, "********")
		}
	}
	return line
}
