package jobs

import (
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/store"
	"github.com/Murathan-Aydin/nodra/internal/version"
)

func TestAutoUpdateDue(t *testing.T) {
	saved := version.Version
	defer func() { version.Version = saved }()
	version.Version = "v1.0.0"
	i := &store.InstanceSettings{AutoUpdateEnabled: true, LatestVersion: "v1.1.0", AutoUpdateFrequency: "0 3 * * *", Timezone: "Europe/Paris"}
	paris, _ := time.LoadLocation("Europe/Paris")
	at := func(h, m int) time.Time { return time.Date(2026, 11, 2, h, m, 0, 0, paris) }
	for name, c := range map[string]struct {
		now  time.Time
		mod  func(*store.InstanceSettings)
		want bool
	}{
		"dans la fenêtre":     {at(3, 5), nil, true},
		"après la fenêtre":    {at(3, 20), nil, false},
		"avant l'échéance":    {at(2, 58), nil, false},
		"désactivée":          {at(3, 5), func(i *store.InstanceSettings) { i.AutoUpdateEnabled = false }, false},
		"déjà à jour":         {at(3, 5), func(i *store.InstanceSettings) { i.LatestVersion = "v1.0.0" }, false},
		"fréquence illisible": {at(3, 5), func(i *store.InstanceSettings) { i.AutoUpdateFrequency = "souvent" }, false},
	} {
		c2 := *i
		if c.mod != nil {
			c.mod(&c2)
		}
		if got := AutoUpdateDue(&c2, c.now); got != c.want {
			t.Errorf("%s : %v", name, got)
		}
	}
	version.Version = "dev"
	if AutoUpdateDue(i, at(3, 5)) {
		t.Error("une version de développement ne se met jamais à jour")
	}
}
