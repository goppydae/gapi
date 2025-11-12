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
	"github.com/goppydae/gapi/internal/lifecycle"
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

	t, err := transport.NewServerFromConfig[*anypb.Any](cfg.Transport)
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

	host, _ := os.Hostname()
	agentFSMs := make(map[string]*lifecycle.LifecycleStateMachine)

	setupAgents := func() {
		logger.Info().Msg("performing agent discovery and FSM setup")

		discovered, err := manager.DiscoverFromPath("agents")
		if err != nil {
			logger.Error().Err(err).Msg("Agent discovery failed")
			return
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
			fsm := lifecycle.NewLifecycleStateMachine(ad.ID, host, bus)
			fmt.Printf("FSM candidate ID: %q (from desc: %+v)\n", ad.ID, desc)
			agentFSMs[ad.ID] = fsm
		}

		logger.Info().Int("fsm_count", len(agentFSMs)).Msg("agent setup complete")
	}

	setupAgents()
	controller := lifecycle.NewLifecycleController(agentFSMs, manager, bus, host)

	bus.SubscribePrefix("system", "system/ping", func(e eventbus.Event[*anypb.Any]) {
		logger.Info().
			Str("event", "handling_ping").
			Str("event_id", e.ID).
			Msg("received ping, preparing pong")

		logevent.Lifecycle(logger, "gapid", "handle_ping", "gapid", version.BinaryVersion())

		pong := &protopkg.PingStatus{Status: "pong"}
		anyPayload, err := anypb.New(pong)
		if err != nil {
			logger.Error().Err(err).Msg("failed to pack pong payload")
			return
		}

		response := eventbus.NewEvent("system", "system/pong", "gapid", anyPayload, true)
		_ = bus.Publish(response)
	})

	bus.SubscribePrefix("system", "system/agents/", func(e eventbus.Event[*anypb.Any]) {
		logger.Info().Str("event_id", e.ID).Msg("received agent status request")

		entries, err := registry.List()
		if err != nil {
			logger.Error().Err(err).Msg("failed to list agents")
			return
		}

		var agentStatuses []*protopkg.AgentStatus
		for _, entry := range entries {
			state := protopkg.AgentState_AGENT_STATE_UNKNOWN
			if fsm, ok := agentFSMs[entry.ID]; ok {
				state = fsm.CurrentProtoState()
			}
			agentStatuses = append(agentStatuses, &protopkg.AgentStatus{
				Id:    entry.ID,
				Type:  entry.Type,
				State: state,
			})
		}

		reply := &protopkg.AgentStatusResponse{Agents: agentStatuses}
		anyPayload, err := anypb.New(reply)
		if err != nil {
			logger.Error().Err(err).Msg("failed to pack agent status response")
			return
		}

		response := eventbus.NewEvent("system", "system/agents.reply", "gapid", anyPayload, true)
		_ = bus.Publish(response)
	})

	bus.Subscribe("system", "system/agent.reload", func(e eventbus.Event[*anypb.Any]) {
		logger.Info().Str("event_id", e.ID).Msg("received agent reload request")

		agentFSMs = make(map[string]*lifecycle.LifecycleStateMachine)
		setupAgents()
	})

	bus.SubscribePrefix("system", "agent/lifecycle.control", func(e eventbus.Event[*anypb.Any]) {
		logger.Info().
			Str("event_id", e.ID).
			Str("topic", e.Topic).
			Msg("received lifecycle control event")

		var cmd protopkg.LifecycleControl
		if err := e.UnmarshalPayload(&cmd); err != nil {
			logger.Error().Err(err).Msg("failed to decode LifecycleControl")
			return
		}

		controller.Handle(&cmd)
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
