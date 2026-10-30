package servers

import (
	"strings"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestParseMetrics(t *testing.T) {
	out := `cpu  100 0 100 800 0 0 0 0 0 0
cpu  150 0 150 850 0 0 0 0 0 0
MemTotal:        2000000 kB
MemAvailable:     500000 kB
/dev/sda1        10000000  2500000  7500000  25% /
0.42 0.30 0.20 1/100 1234`
	m, err := ParseMetrics(out)
	if err != nil {
		t.Fatal(err)
	}
	if int(m.CPUPercent) != 66 { // 100 actifs sur 150
		t.Errorf("cpu %.1f", m.CPUPercent)
	}
	if m.MemTotal != 2000000*1024 || m.MemUsed != 1500000*1024 {
		t.Errorf("mémoire %d/%d", m.MemUsed, m.MemTotal)
	}
	if m.DiskTotal != 10000000*1024 || m.DiskUsed != 2500000*1024 || m.Load1 < 0.41 {
		t.Errorf("disque/charge %+v", m)
	}
	if _, err := ParseMetrics("cpu 1"); err == nil {
		t.Error("sortie incomplète acceptée")
	}
}

func TestParseOSRelease(t *testing.T) {
	id, v := ParseOSRelease("NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"24.04\"\n")
	if id != "ubuntu" || v != "24.04" || !isSupported(id) || isSupported("windows") {
		t.Fatalf("%s %s", id, v)
	}
}

func TestCleanupCommands(t *testing.T) {
	base := CleanupCommands(store.DefaultServerSettings())
	if len(base) != 3 || !strings.Contains(base[0], "label!=nodra.managed=true") {
		t.Fatalf("%v", base)
	}
	s := store.DefaultServerSettings()
	s.DeleteUnusedVolumes, s.DeleteUnusedNetworks = true, true
	if len(CleanupCommands(s)) != 5 {
		t.Fatal("volumes et réseaux optionnels")
	}
}
