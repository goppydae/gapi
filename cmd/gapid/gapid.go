package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/version"
	"github.com/goppydae/gapi/internal/daemonmgr"
	"github.com/goppydae/gapi/internal/eventbus"
	"github.com/goppydae/gapi/internal/logging/logcore"
	"github.com/goppydae/gapi/internal/logging/logevent"
	"github.com/goppydae/gapi/internal/transport"
)

var rootCmd = &cobra.Command{
	Use:   "gapid",
	Short: "GAPI Supervisor Daemon",
	Run: func(cmd *cobra.Command, args []string) {
		runSupervisor()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		logcore.Info().Str("module", "gapid").Msg(version.Summary())
	},
}

func runSupervisor() {
	logger := logcore.With().Str("module", "gapid").Logger()

	logger.Info().Msg("initializing supervisor")

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}

	t, err := transport.NewServerFromConfig(cfg.Transport)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize transport")
	}

	bus := eventbus.NewEventBus(t)
	manager := daemonmgr.NewDaemonManager(bus)

	bus.SubscribePrefix("user", "system/ping", func(e eventbus.Event) {
		logevent.Lifecycle(logger, "gapid", "handle_ping", "gapid", version.BinaryVersion())
		response := eventbus.NewEvent("user", "system/pong", "gapid", map[string]string{"status": "pong"}, false)
		_ = bus.Publish(response)
	})

	bus.SubscribePrefix("user", "example/", func(e eventbus.Event) {
		logcore.Info().
			Str("event", "user_example").
			Str("topic", e.Topic).
			Interface("payload", e.Payload).
			Msg("received example event")
	})

	logger.Info().Msg("supervisor running")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigs
	logger.Warn().Str("signal", sig.String()).Msg("received shutdown signal")

	manager.StopAll()

	logevent.Lifecycle(logger, "gapid", "stop", "gapid", version.BinaryVersion())
	logger.Info().Msg("exited cleanly")
}
