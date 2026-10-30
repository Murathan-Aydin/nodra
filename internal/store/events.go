package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgconn"
)

// EventsChannel : canal PostgreSQL (NOTIFY) des événements temps réel, relayés aux WebSocket par realtime.Hub.
// Les notifications émises dans une transaction ne partent qu'à sa validation.
const EventsChannel = "nodra_events"

// Event : changement d'état signalé aux clients connectés ; ceux-ci relisent l'état via l'API
// (charge utile minimale, sous la limite de 8000 octets de NOTIFY).
type Event struct {
	Team     string `json:"team"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Resource string `json:"resource,omitempty"`
	Server   string `json:"server,omitempty"`
	Status   string `json:"status,omitempty"`
}

const (
	EventDeploymentLog    = "deployment.log"
	EventDeploymentStatus = "deployment.status"
	EventTaskOutput       = "task.output"
	EventTaskStatus       = "task.status"
	EventResourceStatus   = "resource.status"
	EventServerStatus     = "server.status"
	EventProxyStatus      = "proxy.status"
)

// Emit diffuse un événement. Les erreurs sont ignorées : le temps réel n'est qu'une accélération,
// les flux relisent aussi l'état périodiquement.
func (s *Store) Emit(ctx context.Context, e Event) {
	b, _ := json.Marshal(e)
	_, _ = s.Pool.Exec(ctx, `SELECT pg_notify($1, $2)`, EventsChannel, string(b))
}

// execer : pool ou transaction (les notifications d'une transaction partent à sa validation).
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (s *Store) emitDeployment(ctx context.Context, q execer, id, kind string) {
	_, _ = q.Exec(ctx, `SELECT pg_notify($1, json_build_object('team', team_id::text, 'kind', $3::text, 'id', id::text,
		'resource', application_id::text, 'server', server_id::text, 'status', status)::text) FROM deployments WHERE id = $2`,
		EventsChannel, id, kind)
}

func (s *Store) emitTask(ctx context.Context, q execer, id, kind string) {
	_, _ = q.Exec(ctx, `SELECT pg_notify($1, json_build_object('team', team_id::text, 'kind', $3::text, 'id', id::text,
		'server', server_id::text, 'status', status)::text) FROM server_tasks WHERE id = $2`, EventsChannel, id, kind)
}

// emitResource : statut d'une ressource (table : constante applications, databases ou services).
func (s *Store) emitResource(ctx context.Context, table, id string) {
	_, _ = s.Pool.Exec(ctx, `SELECT pg_notify($1, json_build_object('team', team_id::text, 'kind', $3::text, 'id', id::text,
		'status', status)::text) FROM `+table+` WHERE id = $2`, EventsChannel, id, EventResourceStatus)
}
