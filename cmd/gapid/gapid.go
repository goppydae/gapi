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
	"github.com/goppydae/gapi/core/shutdown"
	"github.com/goppydae/gapi/core/supervisor"
	"github.com/goppydae/gapi/internal/logattr"
	"github.com/goppydae/gapi/pkg/cli"
)

func newRootCmd() (*cobra.Command, *cli.DaemonFlags, *cli.GapidStartFlags) {
	// Declared before construction so the start closure can capture them:
	// the flag structs are what NewGapidRoot RETURNS, and the start action
	// needs both. The closure resolves them when start runs, always after
	// the assignment below.
	var daemonFlags *cli.DaemonFlags
	var startFlags *cli.GapidStartFlags

	root, d, sf := cli.NewGapidRoot(func(cmd *cobra.Command, args []string) error {
		return run(daemonFlags, startFlags)
	})
	daemonFlags, startFlags = d, sf
	return root, d, sf
}

func main() {
	root, _, _ := newRootCmd()
	if err := cli.RunRoot(root, os.Args[1:]); err != nil {
		os.Exit(1)
	}
}

func run(flags *cli.DaemonFlags, startFlags *cli.GapidStartFlags) (err error) {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Apply --log-level override (flag beats config), then build the
	// process logger from configuration. This sets the level and wires the
	// file output after flag parsing rather than hardcoding Info at
	// init() time.
	if flags.LogLevel != "" {
		cfg.Logging.Level = flags.LogLevel
	}
	if flags.LogFormat != "" {
		cfg.Logging.Format = flags.LogFormat
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
	if startFlags.ListenAddr != "" {
		cfg.Transport.Address = startFlags.ListenAddr
	}

	// Flag overrides for PID-1 mode (flags beat config)
	if startFlags.Pid1Mode {
		cfg.Supervisor.Pid1Mode = true
	}
	if startFlags.NoEarlyMounts {
		cfg.Supervisor.NoEarlyMounts = true
	}

	// Initialize Supervisor
	sup, err := supervisor.New(cfg)
	if err != nil {
		return fmt.Errorf("supervisor init: %w", err)
	}

	// Setup context with cancellation on signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if cfg.Supervisor.Pid1Mode {
		// PID-1 mode: Phase 0 first; the pid1 signal handlers own
		// SIGTERM/SIGINT (an init has no implicit defaults), and the
		// teardown completes with sync/umount/reboot after the runtime
		// stops. Bus-initiated shutdown (gapictl shutdown) arrives on
		// the same request channel.
		complete, perr := sup.EnablePid1(ctx)
		if perr != nil {
			return fmt.Errorf("enable pid1: %w", perr)
		}
		action := shutdown.PowerOff
		go func() {
			action = <-sup.ShutdownRequests()
			logger.LogAttrs(context.Background(), slog.LevelWarn, "pid1 shutdown requested")
			cancel()
		}()
		if err := sup.Run(ctx); err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "supervisor exited with error", logattr.Err(err))
			return err
		}
		complete(action)
		return nil
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigs:
			logger.LogAttrs(context.Background(), slog.LevelWarn, "received shutdown signal", logattr.Signal(sig.String()))
		case <-sup.ShutdownRequests():
			logger.LogAttrs(context.Background(), slog.LevelWarn, "shutdown requested via bus")
		}
		cancel()
	}()

	// Run Supervisor (blocking)
	if err := sup.Run(ctx); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "supervisor exited with error", logattr.Err(err))
		return err
	}

	return nil
}
