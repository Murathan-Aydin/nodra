package servers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/notify"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// metricsCommand : deux lectures de /proc/stat à 1 s d'intervalle, mémoire, disque racine et charge.
// La dernière ligne donne l'état du conteneur du proxy.
const metricsCommand = `head -1 /proc/stat; sleep 1; head -1 /proc/stat; grep -E '^(MemTotal|MemAvailable):' /proc/meminfo; df -Pk / | tail -1; cat /proc/loadavg; docker inspect -f '{{.State.Status}}' ` + ProxyContainer + ` 2>/dev/null || echo absent`

// ParseMetrics analyse la sortie de metricsCommand.
func ParseMetrics(out string) (store.ServerMetric, error) {
	var m store.ServerMetric
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 6 {
		return m, fmt.Errorf("sortie de métriques incomplète (%d lignes)", len(lines))
	}
	idle1, total1, err := cpuTimes(lines[0])
	if err != nil {
		return m, err
	}
	idle2, total2, err := cpuTimes(lines[1])
	if err != nil {
		return m, err
	}
	if dt := total2 - total1; dt > 0 {
		m.CPUPercent = float32(100 * (1 - float64(idle2-idle1)/float64(dt)))
	}
	for _, l := range lines[2:4] {
		f := strings.Fields(l)
		if len(f) < 2 {
			return m, fmt.Errorf("ligne meminfo invalide : %q", l)
		}
		kb, _ := strconv.ParseInt(f[1], 10, 64)
		switch f[0] {
		case "MemTotal:":
			m.MemTotal = kb * 1024
		case "MemAvailable:":
			m.MemUsed = -kb * 1024 // complété ci-dessous
		}
	}
	m.MemUsed += m.MemTotal
	df := strings.Fields(lines[4])
	if len(df) < 4 {
		return m, fmt.Errorf("ligne df invalide : %q", lines[4])
	}
	total, _ := strconv.ParseInt(df[1], 10, 64)
	used, _ := strconv.ParseInt(df[2], 10, 64)
	m.DiskTotal, m.DiskUsed = total*1024, used*1024
	if la := strings.Fields(lines[5]); len(la) > 0 {
		l, _ := strconv.ParseFloat(la[0], 32)
		m.Load1 = float32(l)
	}
	return m, nil
}

func cpuTimes(line string) (idle, total uint64, err error) {
	f := strings.Fields(line)
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0, fmt.Errorf("ligne /proc/stat invalide : %q", line)
	}
	for i, v := range f[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		total += n
		if i == 3 || i == 4 { // idle + iowait
			idle += n
		}
	}
	return idle, total, nil
}

// CollectMetrics relève les métriques de tous les serveurs utilisables (10 en parallèle)
// et met à jour leur joignabilité.
func (s *Service) CollectMetrics(ctx context.Context) error {
	servers, err := s.Store.UsableServers(ctx)
	if err != nil {
		return err
	}
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for _, sv := range servers {
		wg.Add(1)
		sem <- struct{}{}
		go func(sv *store.Server) {
			defer wg.Done()
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			err := s.collectOne(cctx, sv)
			if err != nil {
				s.Log.Warn("métriques", "server", sv.ID, "err", err)
			}
			_ = s.Store.SetServerReachable(ctx, sv.ID, err == nil)
			s.reachability(ctx, sv, err)
		}(sv)
	}
	wg.Wait()
	return nil
}

func (s *Service) collectOne(ctx context.Context, sv *store.Server) error {
	t, err := s.Target(ctx, sv)
	if err != nil {
		return err
	}
	if err := s.syncContainerStatuses(ctx, t, sv); err != nil {
		return err
	}
	if !sv.Settings.MetricsEnabled {
		return nil
	}
	out, err := s.SSH.Output(ctx, t, metricsCommand)
	if err != nil {
		return err
	}
	m, err := ParseMetrics(out)
	if err != nil {
		return err
	}
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) > 6 && sv.Proxy.Type != ProxyNone {
		status := strings.TrimSpace(lines[6])
		if status == "absent" {
			status = "stopped"
		}
		if status != sv.Proxy.Status {
			_ = s.Store.SetProxyStatus(ctx, sv.ID, status)
		}
	}
	m.TS = time.Now().Truncate(time.Second)
	s.diskUsage(ctx, sv, m)
	return s.Store.InsertServerMetric(ctx, sv.ID, m)
}

// reachability publie « serveur injoignable » (au plus toutes les 6 h tant qu'il le reste) et « de nouveau joignable ».
func (s *Service) reachability(ctx context.Context, sv *store.Server, err error) {
	key := "server.unreachable:" + sv.ID
	if err != nil {
		if send, terr := s.Store.Throttle(ctx, key, 6*time.Hour); terr == nil && send {
			s.publish(ctx, notify.Event{Type: notify.ServerUnreachable, TeamID: sv.TeamID, Title: "Serveur injoignable",
				Message: sv.Name + " (" + sv.IP + ") ne répond pas : " + err.Error(), Level: "error", URL: "/servers/" + sv.ID})
		}
		return
	}
	if !sv.IsReachable {
		_ = s.Store.ClearThrottle(ctx, key)
		s.publish(ctx, notify.Event{Type: notify.ServerReachable, TeamID: sv.TeamID, Title: "Serveur de nouveau joignable",
			Message: sv.Name + " (" + sv.IP + ")", Level: "success", URL: "/servers/" + sv.ID})
	}
}

// diskUsage publie une alerte (au plus une par jour) quand le disque dépasse le seuil de nettoyage du serveur.
func (s *Service) diskUsage(ctx context.Context, sv *store.Server, m store.ServerMetric) {
	threshold := sv.Settings.DockerCleanupThreshold
	if threshold <= 0 || m.DiskTotal == 0 {
		return
	}
	pct := 100 * float64(m.DiskUsed) / float64(m.DiskTotal)
	if pct < float64(threshold) {
		return
	}
	if send, err := s.Store.Throttle(ctx, "server.disk:"+sv.ID, 24*time.Hour); err == nil && send {
		s.publish(ctx, notify.Event{Type: notify.ServerDiskUsage, TeamID: sv.TeamID, Title: "Disque presque plein",
			Message: fmt.Sprintf("%s : disque utilisé à %.0f %% (seuil %d %%)", sv.Name, pct, threshold), Level: "warning",
			URL: "/servers/" + sv.ID})
	}
}

// ContainerStatuses agrège l'état des conteneurs gérés par ressource (hors prévisualisations) :
// running (tous démarrés), degraded (en partie), restarting, exited.
func ContainerStatuses(out string) map[string]string {
	states := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "|")
		if len(f) < 3 || f[0] == "" || (f[1] != "" && f[1] != "0") || (len(f) > 3 && f[3] == "ignore") {
			continue // prévisualisations et conteneurs exclus du statut (nodra.status=ignore)
		}
		states[f[0]] = append(states[f[0]], f[2])
	}
	res := map[string]string{}
	for id, list := range states {
		running, restarting := 0, false
		for _, st := range list {
			switch st {
			case "running":
				running++
			case "restarting":
				restarting = true
			}
		}
		switch {
		case restarting:
			res[id] = "restarting"
		case running == len(list):
			res[id] = "running"
		case running > 0:
			res[id] = "degraded"
		default:
			res[id] = "exited"
		}
	}
	return res
}

func (s *Service) syncContainerStatuses(ctx context.Context, t ssh.Target, sv *store.Server) error {
	out, err := s.SSH.Output(ctx, t, `docker ps -a --filter label=nodra.managed=true --format '{{.Label "nodra.resource"}}|{{.Label "nodra.pr"}}|{{.State}}'`)
	if err != nil {
		return err
	}
	changes, err := s.Store.SetResourceStatuses(ctx, sv.ID, ContainerStatuses(out))
	if err != nil {
		return err
	}
	for _, c := range changes {
		// Ressource qui était en marche et ne l'est plus (hors arrêt demandé, qui passe directement à stopped).
		if c.Old != "running" || c.New == "running" {
			continue
		}
		if send, terr := s.Store.Throttle(ctx, "status:"+c.ID, 15*time.Minute); terr == nil && send {
			s.publish(ctx, notify.Event{Type: notify.StatusChange, TeamID: c.TeamID, Title: "Ressource arrêtée ou en erreur",
				Message: fmt.Sprintf("%s est passé de %s à %s sur %s", c.Name, c.Old, c.New, sv.Name), Level: "error",
				URL: "/" + c.Type + "s/" + c.ID})
		}
	}
	return nil
}
