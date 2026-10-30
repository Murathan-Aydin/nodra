package realtime

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestHub(t *testing.T) {
	url := os.Getenv("NODRA_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("NODRA_TEST_DATABASE_URL non défini")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := New(st.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go h.Run(ctx)
	<-h.Ready()

	all, unsubAll := h.Subscribe(nil)
	defer unsubAll()
	only, unsub := h.Subscribe(func(e store.Event) bool { return e.Kind == store.EventTaskOutput })
	st.Emit(ctx, store.Event{Team: "t", Kind: store.EventResourceStatus, ID: "r1", Status: "running"})
	st.Emit(ctx, store.Event{Team: "t", Kind: store.EventTaskOutput, ID: "k1"})
	for _, want := range []string{"r1", "k1"} {
		select {
		case e := <-all:
			if e.ID != want {
				t.Fatalf("événement %+v, attendu %s", e, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("événement non reçu")
		}
	}
	select {
	case e := <-only:
		if e.ID != "k1" {
			t.Fatalf("filtre : %+v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("événement filtré non reçu")
	}
	unsub()
	st.Emit(ctx, store.Event{Kind: store.EventTaskOutput, ID: "k2"})
	<-all
	select {
	case e := <-only:
		t.Fatalf("désabonné mais reçu %+v", e)
	default:
	}
	var nilHub *Hub
	if ch, stop := nilHub.Subscribe(nil); ch != nil {
		t.Fatal("hub nil : canal nil attendu")
	} else {
		stop()
	}
}
