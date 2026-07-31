// Command panel runs the AmneziaX control plane: the REST API, the web UI and
// the gRPC endpoint node agents connect to.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	nodev1 "github.com/SpecFlowdev/AmneziaX/gen/go/node/v1"
	"github.com/SpecFlowdev/AmneziaX/internal/auth"
	"github.com/SpecFlowdev/AmneziaX/internal/config"
	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/SpecFlowdev/AmneziaX/internal/httpapi"
	"github.com/SpecFlowdev/AmneziaX/internal/hub"
	"github.com/SpecFlowdev/AmneziaX/internal/storage/postgres"
	"github.com/SpecFlowdev/AmneziaX/internal/version"
	"github.com/SpecFlowdev/AmneziaX/internal/webui"
	"github.com/SpecFlowdev/AmneziaX/internal/xray"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadPanel()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)
	log.Info("starting AmneziaX panel", "version", version.Version, "commit", version.Commit)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	log.Info("database schema is up to date")

	if err := bootstrap(ctx, store, cfg, log); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	// No agent stream survives a restart, so start from a clean liveness state.
	if err := store.MarkAllNodesDisconnected(ctx); err != nil {
		log.Warn("could not reset node liveness", "error", err)
	}

	h := hub.New(store, cfg, log)
	go h.RunSyncLoop(ctx)
	go h.RunMaintenance(ctx)

	issuer := auth.NewIssuer(cfg.JWTSecret, cfg.TokenTTL)
	api := httpapi.New(store, h, issuer, cfg, log)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Router(webui.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(16<<20),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    30 * time.Second,
			Timeout: 20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	nodev1.RegisterNodeControlServer(grpcServer, h)

	errc := make(chan error, 2)

	go func() {
		lis, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			errc <- fmt.Errorf("grpc listen: %w", err)
			return
		}
		log.Info("node endpoint listening", "addr", cfg.GRPCAddr)
		if err := grpcServer.Serve(lis); err != nil {
			errc <- fmt.Errorf("grpc serve: %w", err)
		}
	}()

	go func() {
		log.Info("panel listening", "addr", cfg.HTTPAddr, "publicUrl", cfg.PublicURL)
		var err error
		if cfg.TLSCert != "" && cfg.TLSKey != "" {
			err = httpServer.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
		} else {
			err = httpServer.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("http serve: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errc:
		stop()
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	grpcServer.GracefulStop()
	return nil
}

// bootstrap makes a fresh installation usable: it creates the owner account and
// a starter configuration profile so the first node has something to run.
func bootstrap(ctx context.Context, store *postgres.Store, cfg *config.Panel, log *slog.Logger) error {
	count, err := store.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		password := cfg.BootstrapPassword
		generated := false
		if password == "" {
			password = auth.RandomSecret(12)
			generated = true
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}
		if _, err := store.CreateAdmin(ctx, cfg.BootstrapAdmin, hash, domain.RoleOwner); err != nil {
			return err
		}
		if generated {
			// Printed once, on the very first boot, because there is no other
			// channel to hand the operator their credentials.
			log.Warn("created the owner account with a generated password — store it now",
				"username", cfg.BootstrapAdmin, "password", password)
		} else {
			log.Info("created the owner account", "username", cfg.BootstrapAdmin)
		}
	}

	profiles, err := store.ListProfiles(ctx)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		raw, err := xray.DefaultTemplate(xray.TemplateOptions{InboundTag: "vless-reality", Port: 443})
		if err != nil {
			return err
		}
		inbounds, err := xray.ParseInbounds(raw)
		if err != nil {
			return err
		}
		profile, err := store.CreateProfile(ctx, "Default REALITY", raw, inbounds)
		if err != nil {
			return err
		}
		log.Info("created the starter configuration profile", "name", profile.Name)

		if len(profile.Inbounds) > 0 {
			if _, err := store.CreateSquad(ctx, "Default", "Every inbound of the starter profile",
				[]string{profile.Inbounds[0].UUID}); err != nil {
				log.Warn("could not create the starter squad", "error", err)
			}
		}
	}
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
