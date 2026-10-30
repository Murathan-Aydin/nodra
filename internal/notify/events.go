// Package notify envoie les notifications des équipes (e-mail SMTP/Resend, Discord, Slack, Telegram,
// Pushover, webhook) et les e-mails transactionnels de l'instance.
package notify

import (
	"fmt"
	"strings"
	"time"
)

// Types d'événements (alignés sur Coolify, plus les demandes d'approbation).
const (
	DeploymentSuccess    = "deployment.success"
	DeploymentFailure    = "deployment.failure"
	StatusChange         = "status.change"
	BackupSuccess        = "backup.success"
	BackupFailure        = "backup.failure"
	ScheduledTaskSuccess = "scheduled_task.success"
	ScheduledTaskFailure = "scheduled_task.failure"
	DockerCleanupSuccess = "docker_cleanup.success"
	DockerCleanupFailure = "docker_cleanup.failure"
	ServerDiskUsage      = "server.disk_usage"
	ServerReachable      = "server.reachable"
	ServerUnreachable    = "server.unreachable"
	ServerPatch          = "server.patch"
	CertificateExpiring  = "certificate.expiring"
	APITokenExpiring     = "api_token.expiring"
	ApprovalRequested    = "approval.requested"
	Test                 = "test"
)

type EventType struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Default     bool   `json:"default"` // activé par défaut sur un nouveau canal
}

var EventTypes = []EventType{
	{DeploymentSuccess, "Déploiement réussi", false},
	{DeploymentFailure, "Déploiement en échec", true},
	{StatusChange, "Ressource arrêtée ou en erreur", true},
	{BackupSuccess, "Sauvegarde réussie", false},
	{BackupFailure, "Sauvegarde en échec", true},
	{ScheduledTaskSuccess, "Tâche planifiée réussie", false},
	{ScheduledTaskFailure, "Tâche planifiée en échec", true},
	{DockerCleanupSuccess, "Nettoyage Docker effectué", false},
	{DockerCleanupFailure, "Nettoyage Docker en échec", true},
	{ServerDiskUsage, "Disque d'un serveur presque plein", true},
	{ServerReachable, "Serveur de nouveau joignable", true},
	{ServerUnreachable, "Serveur injoignable", true},
	{ServerPatch, "Mises à jour système disponibles", false},
	{CertificateExpiring, "Certificat personnalisé bientôt expiré", true},
	{APITokenExpiring, "Jeton API bientôt expiré", true},
	{ApprovalRequested, "Demande d'approbation", true},
}

func IsEventType(k string) bool {
	for _, e := range EventTypes {
		if e.Key == k {
			return true
		}
	}
	return false
}

// DefaultEvents : événements activés par défaut.
func DefaultEvents() []string {
	var out []string
	for _, e := range EventTypes {
		if e.Default {
			out = append(out, e.Key)
		}
	}
	return out
}

// Event : notification à diffuser aux canaux de l'équipe abonnés à son type.
type Event struct {
	Type    string            `json:"type"`
	TeamID  string            `json:"team_id"`
	Title   string            `json:"title"`
	Message string            `json:"message"`
	URL     string            `json:"url,omitempty"`
	Level   string            `json:"level"` // info | success | warning | error
	Fields  map[string]string `json:"fields,omitempty"`
	At      time.Time         `json:"at"`
}

// Text : rendu texte (Telegram, Pushover, e-mail).
func (e Event) Text() string {
	var b strings.Builder
	b.WriteString(e.Message)
	for _, k := range sortedKeys(e.Fields) {
		fmt.Fprintf(&b, "\n%s : %s", k, e.Fields[k])
	}
	if e.URL != "" {
		b.WriteString("\n" + e.URL)
	}
	return b.String()
}

func (e Event) emoji() string {
	return map[string]string{"success": "✅", "warning": "⚠️", "error": "❌"}[e.Level]
}

// color : couleur Discord selon le niveau (bleu pour info).
func (e Event) color() int {
	if c, ok := map[string]int{"success": 0x2ecc71, "warning": 0xf1c40f, "error": 0xe74c3c}[e.Level]; ok {
		return c
	}
	return 0x3498db
}
