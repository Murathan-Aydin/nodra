package deploy

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Murathan-Aydin/nodra/internal/envvars"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const ManagedLabel = "nodra.managed=true"

// AppSpec : tout ce qu'il faut pour générer le compose d'un conteneur d'application.
type AppSpec struct {
	App           *store.Application
	DeploymentID  string
	ContainerName string
	Image         string
	Network       string
	ProxyType     string
	Domains       []string          // domaines effectifs (ceux de la preview pour une PR)
	Env           map[string]string // variables d'exécution déjà résolues
	Literal       map[string]bool   // clés à protéger de l'interpolation compose
	Storages      []*store.Storage
	PR            int
	CaddyCerts    map[string][2]string
}

// ManagementLabels : labels permettant à Nodra de retrouver les conteneurs d'une ressource.
func ManagementLabels(a *store.Application, deploymentID string, pr int) []string {
	return []string{ManagedLabel, "nodra.resource=" + a.ID, "nodra.resource.type=application", "nodra.team=" + a.TeamID,
		"nodra.deployment=" + deploymentID, "nodra.pr=" + strconv.Itoa(pr)}
}

// firstPort renvoie le premier port exposé.
func firstPort(exposes string) string {
	p, _, _ := strings.Cut(exposes, ",")
	return strings.TrimSpace(p)
}

// RouteFor construit les options de routage d'une application.
func RouteFor(a *store.Application, domains []string, proxyType, network, name string, certs map[string][2]string) RouteOptions {
	st := a.Settings
	return RouteOptions{Name: name, Domains: domains, Port: firstPort(a.PortsExposes), ProxyType: proxyType, Network: network,
		ForceHTTPS: st.ForceHTTPS, Gzip: st.Gzip, StripPrefix: st.StripPrefix, Redirect: a.Redirect, PermanentRedirect: st.PermanentRedirect,
		BasicAuthUser: map[bool]string{true: st.BasicAuthUsername}[st.BasicAuthEnabled], BasicAuthHash: st.BasicAuthPasswordHash,
		CertResolver: st.CertificateResolver, CaddyCerts: certs}
}

// mergeLabels ajoute les labels personnalisés (une ligne clé=valeur) ; ils remplacent les labels générés de même clé.
func mergeLabels(generated []string, custom string) []string {
	idx := map[string]int{}
	out := make([]string, 0, len(generated))
	add := func(l string) {
		k, _, _ := strings.Cut(l, "=")
		if i, ok := idx[k]; ok {
			out[i] = l
			return
		}
		idx[k] = len(out)
		out = append(out, l)
	}
	for _, l := range generated {
		add(l)
	}
	for _, l := range strings.Split(custom, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") && strings.Contains(l, "=") {
			add(l)
		}
	}
	return out
}

// escapeLabels protège les $ des labels (ex. hash bcrypt) de l'interpolation compose.
func escapeLabels(labels []string) []string {
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = envvars.EscapeCompose(l)
	}
	return out
}

func healthcheck(h store.HealthCheck, exposes string) map[string]any {
	if !h.Enabled {
		return nil
	}
	test := h.Command
	if h.Type == "http" {
		port := h.Port
		if port == "" {
			port = firstPort(exposes)
		}
		target := fmt.Sprintf("%s://%s:%s%s", h.Scheme, h.Host, port, h.Path)
		test = fmt.Sprintf("curl -s -X %s -f %s > /dev/null || wget -q -O- %s > /dev/null || exit 1", h.Method, target, target)
		if h.Response != "" {
			q := strings.ReplaceAll(h.Response, "'", `'"'"'`)
			test = fmt.Sprintf("(curl -s -X %s %s || wget -q -O- %s) | grep -q '%s' || exit 1", h.Method, target, target, q)
		}
	}
	return map[string]any{
		"test": []string{"CMD-SHELL", envvars.EscapeCompose(test)}, "interval": fmt.Sprintf("%ds", h.Interval),
		"timeout": fmt.Sprintf("%ds", h.Timeout), "retries": h.Retries, "start_period": fmt.Sprintf("%ds", h.StartPeriod),
	}
}

func applyLimits(svc map[string]any, l store.Limits) {
	if l.Memory != "0" && l.Memory != "" {
		svc["mem_limit"] = l.Memory
	}
	if l.MemorySwap != "0" && l.MemorySwap != "" {
		svc["memswap_limit"] = l.MemorySwap
	}
	if l.MemoryReservation != "0" && l.MemoryReservation != "" {
		svc["mem_reservation"] = l.MemoryReservation
	}
	svc["mem_swappiness"] = l.MemorySwappiness
	if l.CPUs != "0" && l.CPUs != "" {
		svc["cpus"] = l.CPUs
	}
	if l.CPUSet != "" {
		svc["cpuset"] = l.CPUSet
	}
	svc["cpu_shares"] = l.CPUShares
}

func volumesFor(storages []*store.Storage, filesDir string) (mounts []string, named map[string]any) {
	named = map[string]any{}
	for _, s := range storages {
		switch s.Kind {
		case "volume":
			mounts = append(mounts, s.Name+":"+s.MountPath)
			named[s.Name] = map[string]any{"name": s.Name, "labels": []string{ManagedLabel}}
		case "bind":
			mounts = append(mounts, s.HostPath+":"+s.MountPath)
		case "file":
			mounts = append(mounts, filesDir+"/"+s.ID+":"+s.MountPath)
		}
	}
	return mounts, named
}

// AppCompose génère le docker-compose d'une application à conteneur unique.
// filesDir : dossier du serveur où sont écrits les stockages de type fichier.
func AppCompose(s AppSpec, filesDir string) (string, error) {
	a := s.App
	labels := ManagementLabels(a, s.DeploymentID, s.PR)
	routeName := a.ID
	if s.PR > 0 {
		routeName = fmt.Sprintf("%s-pr-%d", a.ID, s.PR)
	}
	routing, err := Labels(RouteFor(a, s.Domains, s.ProxyType, s.Network, routeName, s.CaddyCerts))
	if err != nil {
		return "", err
	}
	labels = escapeLabels(mergeLabels(append(labels, routing...), a.CustomLabels))

	env := map[string]string{}
	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := s.Env[k]
		if s.Literal[k] {
			v = envvars.EscapeCompose(v)
		}
		env[k] = v
	}
	alias := a.ID
	if a.Settings.CustomInternalName != "" {
		alias = a.Settings.CustomInternalName
	}
	if s.PR > 0 {
		alias = fmt.Sprintf("%s-pr-%d", alias, s.PR)
	}
	svc := map[string]any{
		"image": s.Image, "container_name": s.ContainerName, "restart": "unless-stopped",
		"networks":          map[string]any{s.Network: map[string]any{"aliases": []string{alias}}},
		"labels":            labels,
		"stop_grace_period": fmt.Sprintf("%ds", a.Settings.StopGracePeriod),
		"extra_hosts":       []string{"host.docker.internal:host-gateway"},
	}
	if len(env) > 0 {
		svc["environment"] = env
	}
	if a.PortsMappings != "" && s.PR == 0 {
		var ports []string
		for _, p := range strings.Split(a.PortsMappings, ",") {
			ports = append(ports, strings.TrimSpace(p))
		}
		svc["ports"] = ports
	}
	if hc := healthcheck(a.HealthCheck, a.PortsExposes); hc != nil {
		svc["healthcheck"] = hc
	}
	applyLimits(svc, a.Limits)
	mounts, named := volumesFor(s.Storages, filesDir)
	if len(mounts) > 0 {
		svc["volumes"] = mounts
	}
	if a.Settings.GPUEnabled {
		dev := map[string]any{"driver": "nvidia", "capabilities": []string{"gpu"}}
		if a.Settings.GPUDriver != "" {
			dev["driver"] = a.Settings.GPUDriver
		}
		switch {
		case a.Settings.GPUDeviceIDs != "":
			dev["device_ids"] = strings.Split(a.Settings.GPUDeviceIDs, ",")
		case a.Settings.GPUCount != "" && a.Settings.GPUCount != "all":
			n, _ := strconv.Atoi(a.Settings.GPUCount)
			dev["count"] = n
		default:
			dev["count"] = "all"
		}
		svc["deploy"] = map[string]any{"resources": map[string]any{"reservations": map[string]any{"devices": []any{dev}}}}
	}
	if err := ApplyRunOptions(svc, a.CustomDockerRunOptions); err != nil {
		return "", err
	}
	doc := map[string]any{
		"services": map[string]any{"app": svc},
		"networks": map[string]any{s.Network: map[string]any{"external": true, "name": s.Network}},
	}
	if len(named) > 0 {
		doc["volumes"] = named
	}
	out, err := yaml.Marshal(doc)
	return string(out), err
}

// TransformCompose adapte le docker-compose fourni par l'utilisateur : labels de gestion et de routage
// (services ayant des domaines), réseau de la destination, redémarrage automatique.
// dataDir : dossier stable du serveur où sont redirigés les montages relatifs (./data → dataDir/data).
func TransformCompose(raw string, a *store.Application, deploymentID, network, proxyType, dataDir string, certs map[string][2]string) (string, error) {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return "", fmt.Errorf("docker-compose invalide : %w", err)
	}
	services, ok := doc["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return "", fmt.Errorf("docker-compose sans services")
	}
	for name, v := range services {
		svc, ok := v.(map[string]any)
		if !ok {
			return "", fmt.Errorf("service %s invalide", name)
		}
		labels := normalizeList(svc["labels"], "=")
		labels = append(labels, ManagementLabels(a, deploymentID, 0)...)
		labels = append(labels, "nodra.service="+name)
		if domains := a.DockerComposeDomains[name]; len(domains) > 0 {
			opts := RouteFor(a, domains, proxyType, network, a.ID+"-"+name, certs)
			opts.Port = ""
			if p, ok := firstExposed(svc); ok {
				opts.Port = p
			}
			routing, err := Labels(opts)
			if err != nil {
				return "", err
			}
			labels = append(labels, escapeLabels(routing)...)
		}
		svc["labels"] = labels
		attachService(svc, network, dataDir)
	}
	attachNetwork(doc, network)
	declareVolumes(doc)
	out, err := yaml.Marshal(doc)
	return string(out), err
}

// attachService branche un service compose sur le réseau de la destination (sauf network_mode), active
// le redémarrage automatique et redirige les montages relatifs (./x) vers dataDir.
func attachService(svc map[string]any, network, dataDir string) {
	if _, ok := svc["network_mode"]; !ok {
		switch n := svc["networks"].(type) {
		case nil:
			svc["networks"] = []string{network, "default"}
		case []any:
			svc["networks"] = append(n, network)
		case map[string]any:
			n[network] = map[string]any{}
		}
	}
	if _, ok := svc["restart"]; !ok {
		svc["restart"] = "unless-stopped"
	}
	if vols, ok := svc["volumes"].([]any); ok && dataDir != "" {
		for i, v := range vols {
			switch x := v.(type) {
			case string:
				if strings.HasPrefix(x, "./") {
					vols[i] = dataDir + "/" + strings.TrimPrefix(x, "./")
				}
			case map[string]any:
				if src, _ := x["source"].(string); x["type"] == "bind" && strings.HasPrefix(src, "./") {
					x["source"] = dataDir + "/" + strings.TrimPrefix(src, "./")
				}
			}
		}
	}
}

// declareVolumes déclare au niveau du compose les volumes nommés utilisés par les services mais non déclarés
// (les modèles de Coolify les omettent, docker compose les refuserait) ; « memos/:/data » devient « memos:/data ».
func declareVolumes(doc map[string]any) {
	vols, _ := doc["volumes"].(map[string]any)
	if vols == nil {
		vols = map[string]any{}
	}
	services, _ := doc["services"].(map[string]any)
	for _, v := range services {
		svc, _ := v.(map[string]any)
		list, _ := svc["volumes"].([]any)
		for i, item := range list {
			name := ""
			switch x := item.(type) {
			case string:
				src, rest, ok := strings.Cut(x, ":")
				if ok && src != "" && !strings.ContainsAny(src[:1], "/.~$") {
					name = strings.TrimRight(src, "/")
					list[i] = name + ":" + rest
				}
			case map[string]any:
				if src, _ := x["source"].(string); x["type"] == "volume" && src != "" {
					name = src
				}
			}
			if _, declared := vols[name]; name != "" && !declared {
				vols[name] = map[string]any{"labels": []string{ManagedLabel}}
			}
		}
	}
	if len(vols) > 0 {
		doc["volumes"] = vols
	}
}

// attachNetwork déclare le réseau de la destination comme réseau externe du compose.
func attachNetwork(doc map[string]any, network string) {
	nets, _ := doc["networks"].(map[string]any)
	if nets == nil {
		nets = map[string]any{}
	}
	nets[network] = map[string]any{"external": true, "name": network}
	doc["networks"] = nets
}

// normalizeList convertit une liste ou une table compose (labels, environment) en liste "clé=valeur".
func normalizeList(v any, sep string) []string {
	var out []string
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			out = append(out, fmt.Sprint(e))
		}
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, k+sep+fmt.Sprint(x[k]))
		}
	}
	return out
}

// firstExposed renvoie le premier port exposé d'un service compose (expose ou ports).
func firstExposed(svc map[string]any) (string, bool) {
	for _, key := range []string{"expose", "ports"} {
		if list, ok := svc[key].([]any); ok && len(list) > 0 {
			p := fmt.Sprint(list[0])
			if i := strings.LastIndex(p, ":"); i >= 0 {
				p = p[i+1:]
			}
			p, _, _ = strings.Cut(p, "/")
			return p, true
		}
	}
	return "", false
}
