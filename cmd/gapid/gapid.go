package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/logging"
	"github.com/goppydae/gapi/core/supervisor"
	"github.com/goppydae/gapi/core/version"
	"github.com/goppydae/gapi/internal/logattr"
)

var rootCmd = &cobra.Command{
	Use:   "gapid",
	Short: "GAPI Supervisor Daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return run()
	},
}

var (
	runtimeAddr string
	logLevel    string
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Summary())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.Flags().StringVar(&runtimeAddr, "runtime-addr", "", "Runtime bind address (default: 127.0.0.1:14242)")
	rootCmd.Flags().StringVar(&logLevel, "log-level", "", "Log level: trace, debug, info, warn, error (overrides config)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run() (err error) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Apply --log-level override (flag beats config), then build the
	// process logger from configuration. This sets the level and wires the
	// file output after flag parsing rather than hardcoding Info at
	// init() time.
	if logLevel != "" {
		cfg.Logging.Level = logLevel
	}
	rootLogger, logCloser, err := logging.Build(&cfg.Logging)
	if err != nil {
		return fmt.Errorf("init logging: %w", err)
	}
	defer func() {
		if cerr := logCloser.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close log sink: %w", cerr)
		}
	}()
	slog.SetDefault(rootLogger)

	logger := rootLogger.With(logattr.Module("gapid"))

	// Override from flag
	if runtimeAddr != "" {
		cfg.Transport.Address = runtimeAddr
	}

	// Initialize Supervisor
	sup, err := supervisor.New(cfg)
	if err != nil {
		return fmt.Errorf("supervisor init: %w", err)
	}

	// Setup context with cancellation on signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		logger.LogAttrs(context.Background(), slog.LevelWarn, "received shutdown signal", logattr.Signal(sig.String()))
		cancel()
	}()

	// Run Supervisor (blocking)
	if err := sup.Run(ctx); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "supervisor exited with error", logattr.Err(err))
		return err
	}

	return nil
}
