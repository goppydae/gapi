package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/supervisor"
	"github.com/goppydae/gapi/internal/logattr"
)

func main() {
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "standalone gapi library example starting")

	// 1. Programmatic Config
	// In a real embed, you might read this from your own app's config
	cfg := &config.Config{
		Transport: config.TransportConfig{
			Type: "quic", // Use default transport
		},
	}

	// Create agents dir if it doesn't exist to avoid errors
	_ = os.MkdirAll("agents", 0750)

	// 2. Initialize Supervisor as library
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "initializing supervisor")
	sup, err := supervisor.New(cfg)
	if err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelError, "failed to init supervisor", logattr.Err(err))
		os.Exit(1)
	}

	// 3. Run with a timeout context to demonstrate lifecycle control
	// This shows we can start/stop the supervisor programmatically
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "running supervisor for 5s")
	if err := sup.Run(ctx); err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelWarn, "supervisor exited with error", logattr.Err(err))
	}

	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "standalone example finished")
}
