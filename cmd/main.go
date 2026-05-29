package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"ros-ddns/internal/config"
	"ros-ddns/internal/daemon"
	"syscall"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.SetConfig()
	if err != nil {
		log.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	var d *daemon.Daemon
	if d, err = daemon.NewDaemon(cfg, log); err != nil {
		log.Error("daemon initialization failed with error", "err", err)
		os.Exit(1)
	}

	if err := d.Run(ctx); err != nil {
		log.Error("daemon exited with error", "err", err)
		os.Exit(1)
	}
}
