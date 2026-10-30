package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/api"
	"github.com/Murathan-Aydin/nodra/internal/config"
	"github.com/Murathan-Aydin/nodra/internal/crypto"
	"github.com/Murathan-Aydin/nodra/internal/jobs"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
	"github.com/Murathan-Aydin/nodra/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	if err := run(cmd, log); err != nil {
		log.Error("arrêt", "err", err)
		os.Exit(1)
	}
}

func run(cmd string, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connexion base: %w", err)
	}
	defer st.Close()

	if cmd != "migrate" && cmd != "serve" {
		return fmt.Errorf("commande inconnue %q (serve, migrate)", cmd)
	}
	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if err := jobs.Migrate(ctx, st.Pool); err != nil {
		return fmt.Errorf("migrations River: %w", err)
	}
	switch cmd {
	case "migrate":
		return nil
	case "serve":
		return serve(ctx, cfg, st, log)
	default:
		return fmt.Errorf("commande inconnue %q (serve, migrate)", cmd)
	}
}

func serve(ctx context.Context, cfg *config.Config, st *store.Store, log *slog.Logger) error {
	box, err := crypto.New(cfg.AppKey)
	if err != nil {
		return err
	}
	pool := ssh.NewPool()
	defer pool.Close()
	jq, err := jobs.New(st, &servers.Service{Store: st, Box: box, SSH: pool, Log: log}, log, jobs.Options{Periodic: true})
	if err != nil {
		return err
	}
	if err := jq.Start(ctx); err != nil {
		return fmt.Errorf("démarrage des jobs: %w", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = jq.Stop(stopCtx)
	}()
	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: (&api.Server{Store: st, Log: log, Web: web.FS(), Box: box, Jobs: jq,
			SecureCookies: !cfg.IsDev(), TrustProxy: cfg.TrustProxy}).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("écoute", "addr", cfg.Addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		log.Info("arrêt gracieux")
		return srv.Shutdown(shutCtx)
	}
}
