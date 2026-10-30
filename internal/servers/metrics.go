package servers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

// metricsCommand : deux lectures de /proc/stat à 1 s d'intervalle, mémoire, disque racine et charge.
const metricsCommand = `head -1 /proc/stat; sleep 1; head -1 /proc/stat; grep -E '^(MemTotal|MemAvailable):' /proc/meminfo; df -Pk / | tail -1; cat /proc/loadavg`

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
	if !sv.Settings.MetricsEnabled {
		_, err := s.SSH.Output(ctx, t, "true") // simple test de joignabilité
		return err
	}
	out, err := s.SSH.Output(ctx, t, metricsCommand)
	if err != nil {
		return err
	}
	m, err := ParseMetrics(out)
	if err != nil {
		return err
	}
	m.TS = time.Now().Truncate(time.Second)
	return s.Store.InsertServerMetric(ctx, sv.ID, m)
}
