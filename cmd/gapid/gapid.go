package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/supervisor"
	"github.com/goppydae/gapi/core/version"
	"github.com/goppydae/gapi/internal/logging/logcore"
)

var rootCmd = &cobra.Command{
	Use:   "gapid",
	Short: "GAPI Supervisor Daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return run()
	},
}

var runtimeAddr string

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Summary())
	},
}

func init() {
	logcore.Init(zerolog.InfoLevel)
	rootCmd.AddCommand(versionCmd)
	rootCmd.Flags().StringVar(&runtimeAddr, "runtime-addr", "", "Runtime bind address (default: 127.0.0.1:14242)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	logger := logcore.With().Str("module", "gapid").Logger()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

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
		logger.Warn().Str("signal", sig.String()).Msg("received shutdown signal")
		cancel()
	}()

	// Run Supervisor (blocking)
	if err := sup.Run(ctx); err != nil {
		logger.Error().Err(err).Msg("supervisor exited with error")
		return err
	}

	return nil
}
