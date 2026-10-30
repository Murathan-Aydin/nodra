// Package realtime relaie les événements PostgreSQL (LISTEN nodra_events) aux connexions WebSocket.
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Hub diffuse les événements reçus aux abonnés. Un abonné trop lent perd des événements : les flux
// relisent de toute façon l'état en base, l'événement ne sert qu'à réveiller la lecture.
type Hub struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger

	mu    sync.Mutex
	subs  map[*sub]struct{}
	ready chan struct{}
	once  sync.Once
}

type sub struct {
	ch    chan store.Event
	match func(store.Event) bool
}

func New(pool *pgxpool.Pool, log *slog.Logger) *Hub {
	return &Hub{Pool: pool, Log: log, subs: map[*sub]struct{}{}, ready: make(chan struct{})}
}

// Ready est fermé dès que l'écoute est active (tests).
func (h *Hub) Ready() <-chan struct{} { return h.ready }

// Run écoute jusqu'à l'annulation du contexte, en se reconnectant après une erreur.
func (h *Hub) Run(ctx context.Context) {
	for ctx.Err() == nil {
		err := h.listen(ctx)
		if ctx.Err() != nil {
			return
		}
		h.Log.Warn("temps réel : écoute interrompue", "err", err)
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
}

func (h *Hub) listen(ctx context.Context) error {
	c, err := h.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	conn := c.Hijack() // connexion dédiée : elle ne retourne jamais au pool avec un LISTEN actif
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, "LISTEN "+store.EventsChannel); err != nil {
		return err
	}
	h.once.Do(func() { close(h.ready) })
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var e store.Event
		if json.Unmarshal([]byte(n.Payload), &e) == nil {
			h.broadcast(e)
		}
	}
}

func (h *Hub) broadcast(e store.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if s.match != nil && !s.match(e) {
			continue
		}
		select {
		case s.ch <- e:
		default:
		}
	}
}

// Subscribe renvoie les événements acceptés par match (tous si nil) et la fonction de désabonnement.
// Sur un hub nil, le canal ne reçoit jamais rien (les flux se contentent alors de relire périodiquement).
func (h *Hub) Subscribe(match func(store.Event) bool) (<-chan store.Event, func()) {
	if h == nil {
		return nil, func() {}
	}
	s := &sub{ch: make(chan store.Event, 256), match: match}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s.ch, func() {
		h.mu.Lock()
		delete(h.subs, s)
		h.mu.Unlock()
	}
}
