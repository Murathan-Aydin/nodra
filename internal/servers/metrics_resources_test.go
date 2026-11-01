package servers

import (
	"testing"
	"time"
)

func TestParseContainerStats(t *testing.T) {
	out := `P|aaa|res-1|0
P|bbb|res-1|
P|ccc|res-1|12
P|ddd|res-2|0
S|aaa|12.50%|100MiB / 1GiB|1.5kB / 2MB
S|bbb|0.50%|28MiB / 1GiB|500B / 1kB
S|ccc|99.00%|1GiB / 2GiB|1MB / 1MB
S|ddd|1.00%|512KiB / 0B|0B / 0B
S|eee|5.00%|1MiB / 1MiB|0B / 0B`
	ts := time.Now()
	res := ParseContainerStats(out, ts)
	if len(res) != 2 {
		t.Fatalf("ressources : %v", res)
	}
	r1 := res["res-1"] // la prévisualisation (ccc) et le conteneur inconnu (eee) sont ignorés
	if r1.Containers != 2 || r1.CPUPercent != 13 || r1.MemUsed != 128<<20 || r1.MemLimit != 2<<30 || r1.NetRx != 2000 || r1.NetTx != 2001000 || !r1.TS.Equal(ts) {
		t.Fatalf("res-1 : %+v", r1)
	}
	if r2 := res["res-2"]; r2.MemUsed != 512<<10 || r2.Containers != 1 {
		t.Fatalf("res-2 : %+v", r2)
	}
	if ParseContainerStats("", ts) == nil || len(ParseContainerStats("", ts)) != 0 {
		t.Fatal("sortie vide : aucune ressource")
	}
	for in, want := range map[string]int64{"1.5GB": 1_500_000_000, "2KiB": 2048, "42B": 42, "0B": 0, "--": 0} {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %d, attendu %d", in, got, want)
		}
	}
}
