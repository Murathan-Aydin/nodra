package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // fuseaux horaires disponibles même dans une image minimale

	"github.com/Murathan-Aydin/nodra/internal/api"
	"github.com/Murathan-Aydin/nodra/internal/backup"
	"github.com/Murathan-Aydin/nodra/internal/config"
	"github.com/Murathan-Aydin/nodra/internal/crypto"
	"github.com/Murathan-Aydin/nodra/internal/deploy"
	"github.com/Murathan-Aydin/nodra/internal/jobs"
	"github.com/Murathan-Aydin/nodra/internal/notify"
	"github.com/Murathan-Aydin/nodra/internal/realtime"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
	"github.com/Murathan-Aydin/nodra/internal/version"
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
	switch cmd { // commandes sans base de données
	case "version":
		fmt.Println(version.Version)
		return nil
	case "healthcheck":
		return healthcheck()
	}
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
		return fmt.Errorf("commande inconnue %q (serve, migrate, version, healthcheck)", cmd)
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
	svc := &servers.Service{Store: st, Box: box, SSH: pool, Log: log}
	runner := &deploy.Runner{Store: st, Box: box, SSH: pool, Servers: svc, Log: log, AppsDir: cfg.ServerDataDir + "/applications",
		DatabasesDir: cfg.ServerDataDir + "/databases", ServicesDir: cfg.ServerDataDir + "/services"}
	backups := &backup.Service{Store: st, Box: box, SSH: pool, Log: log, Dir: cfg.ServerDataDir + "/backups"}
	svc.Tasks = runner.Tasks()
	for k, v := range backups.Tasks() {
		svc.Tasks[k] = v
	}
	baseURL := func(ctx context.Context) string { return instanceURL(ctx, st, cfg) }
	dispatcher := &notify.Dispatcher{Store: st, Box: box, Log: log, BaseURL: baseURL}
	apiSrv := &api.Server{Store: st, Log: log, Web: web.FS(), Box: box, Deployer: runner, Backups: backups, Targets: svc,
		Notifier: dispatcher, Mailer: &notify.Mailer{D: dispatcher, BaseURL: baseURL},
		Hub: realtime.New(st.Pool, log), AllowedOrigins: cfg.AllowedOrigins,
		Localhost:     api.LocalhostConfig{KeyFile: cfg.LocalhostKeyFile, Host: cfg.LocalhostHost, User: cfg.LocalhostUser},
		SecureCookies: !cfg.IsDev(), TrustProxy: cfg.TrustProxy}
	// avant le démarrage des workers
	runner.OnFinish = apiSrv.DeploymentFinished
	backups.OnResult = apiSrv.BackupFinished
	installDir := ""
	if cfg.LocalhostKeyFile != "" { // installation par install.sh : update.sh dans <dossier>/source
		installDir = cfg.ServerDataDir + "/source"
	}
	apiSrv.InstallDir = installDir
	jq, err := jobs.New(st, svc, runner, log, jobs.Options{Periodic: true, Notifier: dispatcher, InstallDir: installDir})
	if err != nil {
		return err
	}
	svc.Notify, apiSrv.Notify = jq, jq
	if err := jq.Start(ctx); err != nil {
		return fmt.Errorf("démarrage des jobs: %w", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = jq.Stop(stopCtx)
	}()
	apiSrv.Jobs = jq
	go apiSrv.Hub.Run(ctx)
	if err := apiSrv.BootstrapLocalhost(ctx); err != nil {
		log.Error("serveur localhost", "err", err)
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           apiSrv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	srv.RegisterOnShutdown(apiSrv.CloseStreams) // WebSocket : connexions reprises, non suivies par Shutdown
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

// instanceURL : URL publique de l'instance (paramètre FQDN, sinon NODRA_PUBLIC_URL) pour les liens des e-mails et notifications.
func instanceURL(ctx context.Context, st *store.Store, cfg *config.Config) string {
	if i, err := st.InstanceSettings(ctx); err == nil && i.FQDN != nil && *i.FQDN != "" {
		u := strings.TrimRight(*i.FQDN, "/")
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}
		return u
	}
	return strings.TrimRight(cfg.PublicURL, "/")
}

// healthcheck interroge /api/v1/health du processus local (HEALTHCHECK de l'image, sans curl).
func healthcheck() error {
	addr := os.Getenv("NODRA_ADDR")
	if addr == "" {
		addr = ":8000"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get("http://" + addr + "/api/v1/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("santé : HTTP %d", resp.StatusCode)
	}
	return nil
}
