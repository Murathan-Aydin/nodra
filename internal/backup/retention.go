package backup

import (
	"time"

	"github.com/robfig/cron/v3"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Expired renvoie les exécutions à supprimer : au-delà des keep plus récentes (0 = illimité)
// ou plus anciennes que days jours (0 = illimité). list est triée de la plus récente à la plus ancienne.
func Expired(list []*store.BackupExecution, keep, days int, now time.Time) []*store.BackupExecution {
	var out []*store.BackupExecution
	for i, e := range list {
		if (keep > 0 && i >= keep) || (days > 0 && now.Sub(e.CreatedAt) > time.Duration(days)*24*time.Hour) {
			out = append(out, e)
		}
	}
	return out
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ParseCron valide une fréquence (5 champs ou @daily…).
func ParseCron(expr string) error { _, err := cronParser.Parse(expr); return err }

// Due indique si une planification doit s'exécuter : échéance cron dépassée depuis la dernière exécution.
func Due(b *store.BackupSchedule, tz string, now time.Time) bool {
	sched, err := cronParser.Parse(b.Frequency)
	if err != nil {
		return false
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	last := b.CreatedAt
	if b.LastRunAt != nil {
		last = *b.LastRunAt
	}
	return !sched.Next(last.In(loc)).After(now)
}
