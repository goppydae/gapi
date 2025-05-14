package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/store"
	"github.com/goppydae/gapi/core/version"
	"github.com/goppydae/gapi/internal/agentmgr"
	"github.com/goppydae/gapi/internal/agentreg"
	"github.com/goppydae/gapi/internal/eventbus"
	"github.com/goppydae/gapi/internal/logging/logcore"
	"github.com/goppydae/gapi/internal/logging/logevent"
	protopkg "github.com/goppydae/gapi/internal/proto"
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
		fmt.Print(version.Summary())
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
	manager := agentmgr.NewAgentManager(bus)
	raw, err := store.Open(store.Hybrid)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to open database")
	}

	db, ok := raw.(store.HybridStore)
	if !ok {
		logger.Fatal().Msg("Failed to cast store to HybridStore")
	}

	registry, err := agentreg.NewAgentRegistry(db)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create agent registry")
	}

	discovered, err := manager.DiscoverFromPath("agents")
	if err != nil {
		logger.Error().Err(err).Msg("Agent discovery failed")
	}
	for _, desc := range discovered {
		ad := &agentreg.AgentDescription{
			ID:       desc["id"],
			Path:     desc["path"],
			Type:     desc["type"],
			Language: desc["language"],
			Version:  desc["version"],
			Hash:     desc["hash"],
			Tags:     splitCSV(desc["tags"]),
			Requires: splitCSV(desc["requires"]),
			Wants:    splitCSV(desc["wants"]),
		}
		if err := registry.Register(ad); err != nil {
			logger.Error().Err(err).
				Str("agent_id", ad.ID).
				Msg("failed to register discovered agent")
		}
	}

	bus.SubscribePrefix("user", "system/ping", func(e eventbus.Event) {
		logger.Info().
			Str("event", "handling_ping").
			Str("event_id", e.ID).
			Msg("received ping, preparing pong")

		logevent.Lifecycle(logger, "gapid", "handle_ping", "gapid", version.BinaryVersion())

		pong := &protopkg.PingStatus{Status: "pong"}
		anyPayload, err := anypb.New(pong)
		if err != nil {
			logger.Error().Err(err).Msg("failed to pack pong payload as Any")
			return
		}

		response := eventbus.NewEvent("user", "system/pong", "gapid", anyPayload, true)

		if err := bus.Publish(response); err != nil {
			logger.Error().
				Err(err).
				Str("topic", response.Topic).
				Str("event_id", response.ID).
				Msg("failed to publish pong event")
		} else {
			logger.Info().
				Str("event", "pong_sent").
				Str("event_id", response.ID).
				Msg("pong published")
		}
	})

	bus.SubscribePrefix("user", "system/agents", func(e eventbus.Event) {
		logger.Info().Str("event_id", e.ID).Msg("received agent status request")

		entries, err := registry.List()
		if err != nil {
			logger.Error().Err(err).Msg("failed to list agents")
			return
		}

		var agentStatuses []*protopkg.AgentStatus
		for _, entry := range entries {
			agentStatuses = append(agentStatuses, &protopkg.AgentStatus{
				Id:   entry.ID,
				Type: entry.Type,
			})
		}

		reply := &protopkg.AgentStatusResponse{Agents: agentStatuses}
		anyPayload, err := anypb.New(reply)
		if err != nil {
			logger.Error().Err(err).Msg("failed to pack agent status response")
			return
		}

		response := eventbus.NewEvent("user", "system/agents.reply", "gapid", anyPayload, true)

		if err := bus.Publish(response); err != nil {
			logger.Error().Err(err).Str("event_id", e.ID).Msg("failed to publish agent status")
		}
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

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
