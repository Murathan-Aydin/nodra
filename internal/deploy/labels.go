// Package deploy génère la configuration Docker des ressources (docker-compose, labels de routage
// Traefik/Caddy, variables d'environnement) et orchestre leurs déploiements.
package deploy

import (
	"fmt"
	"net/url"
	"strings"
)

// RouteOptions décrit le routage HTTP d'un conteneur derrière le proxy.
type RouteOptions struct {
	Name              string   // identifiant unique des routeurs (ex. id de la ressource, + service)
	Domains           []string // URL normalisées ; un port dans l'URL désigne le port du conteneur
	Port              string   // port du conteneur par défaut
	ProxyType         string   // traefik | caddy | none
	Network           string
	ForceHTTPS        bool
	Gzip              bool
	StripPrefix       bool
	Redirect          string // both | www | non-www
	PermanentRedirect bool
	BasicAuthUser     string
	BasicAuthHash     string // bcrypt
	CertResolver      string
	CaddyCerts        map[string][2]string // hôte → (certificat, clé) dans /certs
}

type domain struct {
	scheme, host, port, path string
}

func parseDomain(raw string) (domain, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return domain{}, fmt.Errorf("domaine invalide : %s", raw)
	}
	p := u.Path
	if p == "" {
		p = "/"
	}
	return domain{scheme: u.Scheme, host: strings.ToLower(u.Hostname()), port: u.Port(), path: p}, nil
}

// Labels renvoie les labels de routage pour le proxy du serveur.
func Labels(o RouteOptions) ([]string, error) {
	switch o.ProxyType {
	case "traefik":
		return traefikLabels(o)
	case "caddy":
		return caddyLabels(o)
	}
	return nil, nil
}

func traefikLabels(o RouteOptions) ([]string, error) {
	if len(o.Domains) == 0 {
		return nil, nil
	}
	labels := []string{"traefik.enable=true", "traefik.docker.network=" + o.Network}
	if o.Gzip {
		labels = append(labels, "traefik.http.middlewares.gzip.compress=true",
			"traefik.http.middlewares.gzip.compress.excludedcontenttypes=text/event-stream")
	}
	labels = append(labels, "traefik.http.middlewares.redirect-to-https.redirectscheme.scheme=https")
	if o.PermanentRedirect {
		labels = append(labels, "traefik.http.middlewares.redirect-to-https.redirectscheme.permanent=true")
	}
	auth := ""
	if o.BasicAuthUser != "" && o.BasicAuthHash != "" {
		auth = "auth-" + o.Name
		labels = append(labels, fmt.Sprintf("traefik.http.middlewares.%s.basicauth.users=%s:%s", auth, o.BasicAuthUser, o.BasicAuthHash))
	}
	toWWW, toNonWWW := "to-www-"+o.Name, "to-non-www-"+o.Name
	switch o.Redirect {
	case "www":
		labels = append(labels,
			"traefik.http.middlewares."+toWWW+".redirectregex.regex=^(http|https)://(?:www\\.)?(.+)",
			"traefik.http.middlewares."+toWWW+".redirectregex.replacement=${1}://www.${2}",
			fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.permanent=%t", toWWW, o.PermanentRedirect))
	case "non-www":
		labels = append(labels,
			"traefik.http.middlewares."+toNonWWW+".redirectregex.regex=^(http|https)://www\\.(.+)",
			"traefik.http.middlewares."+toNonWWW+".redirectregex.replacement=${1}://${2}",
			fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.permanent=%t", toNonWWW, o.PermanentRedirect))
	}
	for i, raw := range o.Domains {
		d, err := parseDomain(raw)
		if err != nil {
			return nil, err
		}
		port := o.Port
		if d.port != "" {
			port = d.port
		}
		rule := fmt.Sprintf("Host(`%s`)", d.host)
		if d.path != "/" {
			rule += fmt.Sprintf(" && PathPrefix(`%s`)", d.path)
		}
		// Middlewares appliqués au routeur qui sert le trafic.
		var mws []string
		if d.path != "/" && o.StripPrefix {
			sp := fmt.Sprintf("strip-%d-%s", i, o.Name)
			labels = append(labels, fmt.Sprintf("traefik.http.middlewares.%s.stripprefix.prefixes=%s", sp, d.path))
			mws = append(mws, sp)
		}
		if o.Gzip {
			mws = append(mws, "gzip")
		}
		if o.Redirect == "www" && !strings.HasPrefix(d.host, "www.") {
			mws = append(mws, toWWW)
		}
		if o.Redirect == "non-www" && strings.HasPrefix(d.host, "www.") {
			mws = append(mws, toNonWWW)
		}
		if auth != "" {
			mws = append(mws, auth)
		}
		httpR, httpsR := fmt.Sprintf("http-%d-%s", i, o.Name), fmt.Sprintf("https-%d-%s", i, o.Name)
		svc := httpsR
		if d.scheme == "http" {
			svc = httpR
		}
		if port != "" {
			labels = append(labels, fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%s", svc, port))
		}
		labels = append(labels, "traefik.http.routers."+httpR+".rule="+rule, "traefik.http.routers."+httpR+".entryPoints=http")
		if d.scheme == "https" {
			labels = append(labels,
				"traefik.http.routers."+httpsR+".rule="+rule,
				"traefik.http.routers."+httpsR+".entryPoints=https",
				"traefik.http.routers."+httpsR+".tls=true",
				"traefik.http.routers."+httpsR+".service="+svc)
			if o.CertResolver != "" {
				labels = append(labels, "traefik.http.routers."+httpsR+".tls.certresolver="+o.CertResolver)
			}
			if len(mws) > 0 {
				labels = append(labels, "traefik.http.routers."+httpsR+".middlewares="+strings.Join(mws, ","))
			}
			if o.ForceHTTPS {
				labels = append(labels, "traefik.http.routers."+httpR+".middlewares=redirect-to-https")
				continue
			}
		}
		labels = append(labels, "traefik.http.routers."+httpR+".service="+svc)
		if len(mws) > 0 {
			labels = append(labels, "traefik.http.routers."+httpR+".middlewares="+strings.Join(mws, ","))
		}
	}
	return labels, nil
}

func caddyLabels(o RouteOptions) ([]string, error) {
	if len(o.Domains) == 0 {
		return nil, nil
	}
	labels := []string{"caddy_ingress_network=" + o.Network}
	code := ""
	if o.PermanentRedirect {
		code = " permanent"
	}
	for i, raw := range o.Domains {
		d, err := parseDomain(raw)
		if err != nil {
			return nil, err
		}
		port := o.Port
		if d.port != "" {
			port = d.port
		}
		site := fmt.Sprintf("caddy_%d", i)
		labels = append(labels, fmt.Sprintf("%s=%s://%s", site, d.scheme, d.host))
		handle := "handle"
		if d.path != "/" && o.StripPrefix {
			handle = "handle_path"
		}
		upstream := "{{upstreams}}"
		if port != "" {
			upstream = "{{upstreams " + port + "}}"
		}
		labels = append(labels,
			fmt.Sprintf("%s.%s=%s*", site, handle, strings.TrimSuffix(d.path, "/")),
			fmt.Sprintf("%s.%s.%d_reverse_proxy=%s", site, handle, i, upstream))
		if o.Gzip {
			labels = append(labels, site+".encode=zstd gzip")
		}
		if o.Redirect == "www" && !strings.HasPrefix(d.host, "www.") {
			labels = append(labels, fmt.Sprintf("%s.redir=%s://www.%s{uri}%s", site, d.scheme, d.host, code))
		}
		if o.Redirect == "non-www" && strings.HasPrefix(d.host, "www.") {
			labels = append(labels, fmt.Sprintf("%s.redir=%s://%s{uri}%s", site, d.scheme, strings.TrimPrefix(d.host, "www."), code))
		}
		if o.BasicAuthUser != "" && o.BasicAuthHash != "" {
			labels = append(labels, fmt.Sprintf("%s.basic_auth.%s=%s", site, o.BasicAuthUser, o.BasicAuthHash))
		}
		if c, ok := o.CaddyCerts[d.host]; ok && d.scheme == "https" {
			labels = append(labels, fmt.Sprintf("%s.tls=%s %s", site, c[0], c[1]))
		}
	}
	return labels, nil
}
