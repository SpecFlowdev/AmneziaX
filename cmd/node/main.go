// Command node runs the AmneziaX node agent: it keeps a control stream to the
// panel open and supervises the local xray-core process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/SpecFlowdev/AmneziaX/internal/config"
	"github.com/SpecFlowdev/AmneziaX/internal/nodeagent"
	"github.com/SpecFlowdev/AmneziaX/internal/version"
)

func main() {
	cfg, err := config.LoadAgent()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	log.Info("starting AmneziaX node agent",
		"version", version.Version, "panel", cfg.PanelAddr, "node", cfg.NodeUUID)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := nodeagent.New(cfg, log).Run(ctx); err != nil {
		log.Error("agent stopped", "error", err)
		os.Exit(1)
	}
	log.Info("agent stopped")
}
