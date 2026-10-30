package jobs

import (
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestCleanupDue(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	sv := &store.Server{Settings: store.DefaultServerSettings(), CreatedAt: now.Add(-2 * time.Hour)}
	last := now.Add(-2 * time.Hour)
	sv.LastCleanupAt = &last
	if CleanupDue(sv, 10, now) {
		t.Fatal("quotidien à minuit : pas dû à midi après un nettoyage à 10 h")
	}
	yesterday := now.Add(-13 * time.Hour)
	sv.LastCleanupAt = &yesterday
	if !CleanupDue(sv, 10, now) {
		t.Fatal("minuit est passé depuis le dernier nettoyage")
	}
	sv.LastCleanupAt = &last
	if !CleanupDue(sv, 85, now) {
		t.Fatal("disque à 85 % > seuil 80 %")
	}
	recent := now.Add(-30 * time.Minute)
	sv.LastCleanupAt = &recent
	if CleanupDue(sv, 95, now) {
		t.Fatal("au plus un nettoyage par heure sur seuil")
	}
	if _, err := ParseCron("*/5 * * * *"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCron("pas un cron"); err == nil {
		t.Fatal("cron invalide accepté")
	}
}
