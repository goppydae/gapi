package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/store"
	"github.com/goppydae/gapi/core/version"
	"github.com/goppydae/gapi/internal/agentmgr"
	"github.com/goppydae/gapi/internal/agentreg"
	"github.com/goppydae/gapi/internal/cgroups"
	"github.com/goppydae/gapi/internal/eventbus"
	"github.com/goppydae/gapi/internal/lifecycle"
	"github.com/goppydae/gapi/internal/logging/logcore"
	"github.com/goppydae/gapi/internal/logging/logevent"
	protopkg "github.com/goppydae/gapi/internal/proto"
	"github.com/goppydae/gapi/internal/transport"
)

// busAdapter implements agentmgr.Bus using our eventbus. Keep it decoupled;
// you can enhance this to forward payloads later if you like.
type busAdapter struct {
	eb *eventbus.EventBus[*anypb.Any]
}

func (b busAdapter) Publish(topic string, payload any) {
	_ = topic
	_ = payload
}

// Resolve path to the Python ADK runner.
func resolvePyRunner() string {
	if v := os.Getenv("GAPI_PY_RUNNER"); v != "" {
		return v
	}
	// Try alongside the binary
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		cand := filepath.Join(dir, "adk", "python", "agent", "runner.py")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	// Repo-relative during dev
	return filepath.Join("adk", "python", "agent", "runner.py")
}

// Resolve agents directory in this order:
// 1) GAPI_AGENTS_DIR env
// 2) cfg.Agents.Dir (if present in your config)
// 3) ./agents
// 4) ./gapi/agents
func resolveAgentsDir(cfg *config.Config) string {
	if v := os.Getenv("GAPI_AGENTS_DIR"); v != "" {
		return v
	}
	// Try config if your struct has it (ignore if absent)
	type agentsDirGetter interface{ AgentsDir() string }
	if g, ok := any(cfg).(agentsDirGetter); ok {
		if dir := g.AgentsDir(); dir != "" {
			return dir
		}
	}
	if _, err := os.Stat("agents"); err == nil {
		return "agents"
	}
	return filepath.Join("gapi", "agents")
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func mapStateToProto(s string) protopkg.AgentState {
	switch s {
	case lifecycle.StateInitializing:
		return protopkg.AgentState_AGENT_STATE_INITIALIZED
	case lifecycle.StateStarting:
		return protopkg.AgentState_AGENT_STATE_STARTING
	case lifecycle.StateRunning:
		return protopkg.AgentState_AGENT_STATE_RUNNING
	case lifecycle.StateStopping:
		return protopkg.AgentState_AGENT_STATE_STOPPING
	case lifecycle.StateStopped:
		return protopkg.AgentState_AGENT_STATE_STOPPED
	case lifecycle.StateReloading:
		return protopkg.AgentState_AGENT_STATE_RELOADING
	case lifecycle.StateRestarting:
		// if you don’t have a distinct enum, pick the closest:
		return protopkg.AgentState_AGENT_STATE_STARTING
	case lifecycle.StateError:
		return protopkg.AgentState_AGENT_STATE_FAILED
	default:
		return protopkg.AgentState_AGENT_STATE_UNKNOWN
	}
}

// Case-insensitive agent lookup helper. Uses exact match first,
// then falls back to lowercase comparison across all registered agents.
func getAgentCI(mgr *agentmgr.AgentManager, id string) interface {
	Controller() *lifecycle.Controller
	Describe() map[string]string
} {
	if id == "" {
		return nil
	}
	if ag := mgr.Get(id); ag != nil {
		return ag
	}
	idLower := strings.ToLower(id)
	for k, ag := range mgr.All() {
		if strings.ToLower(k) == idLower {
			return ag
		}
		// Some describe() may expose the canonical ID differently—check that too.
		if desc := ag.Describe(); strings.ToLower(desc["id"]) == idLower {
			return ag
		}
	}
	return nil
}

// consider states that are still “in motion”
func isInFlight(state string) bool {
	s := strings.ToUpper(strings.TrimSpace(state))
	switch s {
	case "PENDING", "STARTING", "STOPPING", "RELOADING", "INITIALIZING", "":
		return true
	default:
		return false
	}
}

var rootCmd = &cobra.Command{
	Use:   "gapid",
	Short: "GAPI Supervisor Daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSupervisor()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Summary())
	},
}

func runSupervisor() error {
	logger := logcore.With().Str("module", "gapid").Logger()
	logger.Info().Msg("starting gapid supervisor")

	// Setup Cgroups (Rootless Evacuation)
	if err := cgroups.Setup(); err != nil {
		// Log warning but don't fail, we might be in container or basic env
		logger.Warn().Err(err).Msg("failed to setup cgroups, resource limits will be unavailable")
	} else {
		logger.Info().Msg("cgroups setup complete")
	}

	// Config and transport
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	t, err := transport.NewServerFromConfig(cfg.Transport)
	if err != nil {
		return fmt.Errorf("transport init: %w", err)
	}

	bus := eventbus.NewEventBus[*anypb.Any](t)
	typedBus := lifecycle.TypedBus{}

	// Agent manager: bus adapter + python runner path
	pyRunner := resolvePyRunner()
	manager := agentmgr.NewAgentManager(bus, &typedBus, pyRunner)

	// Persistent store + registry
	raw, err := store.Open(store.Hybrid)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to open database")
	}
	db, ok := raw.(store.HybridStore)
	if !ok {
		return fmt.Errorf("failed to cast store to HybridStore")
	}
	registry, err := agentreg.NewAgentRegistry(db)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create agent registry")
	}

	host, _ := os.Hostname()

	// --- Discovery & registration ---
	setupAgents := func() {
		logger.Info().Msg("performing agent discovery and setup")

		agentsDir := resolveAgentsDir(cfg)
		discovered, err := manager.DiscoverFromPath(agentsDir)
		if err != nil {
			logger.Error().Err(err).Str("dir", agentsDir).Msg("Agent discovery failed")
			return
		}

		// Register with the DB and log what we actually found
		var foundIDs []string
		for _, desc := range discovered {
			id := desc["id"]
			foundIDs = append(foundIDs, id)
			ad := &agentreg.AgentDescription{
				ID:         id,
				Path:       desc["path"],
				Type:       desc["type"],
				Language:   desc["language"],
				Version:    desc["version"],
				Hash:       desc["hash"],
				Tags:       splitCSV(desc["tags"]),
				Requires:   splitCSV(desc["requires"]),
				Wants:      splitCSV(desc["wants"]),
				WantedBy:   splitCSV(desc["wanted_by"]),
				RequiredBy: splitCSV(desc["required_by"]),
			}
			if len(ad.Requires) == 0 && desc["deps"] != "" {
				ad.Requires = splitCSV(desc["deps"])
			}
			if err := registry.Register(ad); err != nil {
				logger.Error().Err(err).
					Str("agent_id", ad.ID).
					Msg("failed to register discovered agent")
			}
		}

		// Extra visibility: print the manager's registered agents (source of truth for control)
		if len(manager.All()) == 0 {
			logger.Warn().Str("dir", agentsDir).Msg("no agents registered in manager")
		} else {
			for id, ag := range manager.All() {
				logger.Info().Str("agent_id", id).Msg("registered agent")

				// Lazy Activation Setup
				desc := ag.Describe()
				if desc["listen_stream"] != "" {
					if armable, ok := ag.(interface {
						Arm() error
						SetTrafficHandler(func())
						Controller() *lifecycle.Controller
					}); ok {
						ctrl := armable.Controller()
						// Handler triggers Start
						armable.SetTrafficHandler(func() {
							logger.Info().Str("agent_id", id).Msg("traffic detected, triggering lazy start")
							if err := ctrl.Apply(lifecycle.ActionStart); err != nil {
								logger.Error().Err(err).Str("agent_id", id).Msg("lazy start failed")
							}
						})
						// Arm the watcher
						if err := armable.Arm(); err != nil {
							logger.Error().Err(err).Str("agent_id", id).Msg("failed to arm lazy activation")
						} else {
							logger.Info().Str("agent_id", id).Msg("armed lazy activation")
						}
					}
				}
			}
		}

		logger.Info().Msg("agent setup complete")
	}

	setupAgents()

	// --- Event handlers ---

	// Ping/Pong
	bus.SubscribePrefix("system", "ping", func(e eventbus.Event[*anypb.Any]) {
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

		response := eventbus.NewEvent("system", "pong", "gapid", anyPayload, true)
		_ = bus.Publish(response)
	})

	// Agent status: use controllers for current state
	bus.SubscribePrefix("system", "agents/", func(e eventbus.Event[*anypb.Any]) {
		logger.Info().Str("event_id", e.ID).Msg("received agent status request")

		entries, err := registry.List()
		if err != nil {
			logger.Error().Err(err).Msg("failed to list agents")
			return
		}

		var agentStatuses []*protopkg.AgentStatus
		for _, entry := range entries {
			st := protopkg.AgentState_AGENT_STATE_UNKNOWN
			if ag := getAgentCI(manager, entry.ID); ag != nil {
				st = mapStateToProto(ag.Controller().State())
			}
			deps, err := registry.GetDependencies(entry.ID)
			if err != nil {
				// Fallback or log?
				deps = entry.Requires // Fallback to struct
				logger.Warn().Err(err).Str("agent", entry.ID).Msg("failed to resolve graph deps")
			}

			agentStatuses = append(agentStatuses, &protopkg.AgentStatus{
				Id:           entry.ID,
				Type:         entry.Type,
				State:        st,
				Dependencies: deps,
			})
		}

		reply := &protopkg.AgentStatusResponse{Agents: agentStatuses}
		anyPayload, err := anypb.New(reply)
		if err != nil {
			logger.Error().Err(err).Msg("failed to pack agent status response")
			return
		}

		response := eventbus.NewEvent("system", "agents.reply", "gapid", anyPayload, true)
		_ = bus.Publish(response)
	})

	// Agent reload: re-run discovery/registration
	bus.Subscribe("system", "agent.reload", func(e eventbus.Event[*anypb.Any]) {
		logger.Info().Str("event_id", e.ID).Msg("received agent reload request")
		setupAgents()
	})

	bus.SubscribePrefix("system", "agent/lifecycle.action", func(e eventbus.Event[*anypb.Any]) {
		logger.Info().
			Str("event_id", e.ID).
			Str("topic", e.Topic).
			Msg("received lifecycle control event")

		var cmd protopkg.LifecycleControl
		if err := eventbus.UnmarshalAnyPayload(e, &cmd); err != nil {
			logger.Error().Err(err).Msg("failed to decode LifecycleControl")

			// Inline FAILED reply on decode error
			if anyPayload, err2 := anypb.New(&protopkg.LifecycleStatus{
				AgentId:  cmd.GetAgentId(),
				State:    "FAILED",
				Message:  "decode error: " + err.Error(),
				Time:     timestamppb.Now(),
				Hostname: host,
			}); err2 == nil {
				resp := eventbus.NewEvent("system", "agent/lifecycle.status", "gapid", anyPayload, true)
				_ = bus.Publish(resp)
			}
			return
		}

		targetID := strings.TrimSpace(cmd.GetAgentId())
		ag := getAgentCI(manager, targetID)
		if ag == nil {
			logger.Warn().Str("agent_id", targetID).Msg("unknown agent in lifecycle control")

			// Inline FAILED reply on unknown agent
			if anyPayload, err := anypb.New(&protopkg.LifecycleStatus{
				AgentId:  targetID,
				State:    "FAILED",
				Message:  "unknown agent",
				Time:     timestamppb.Now(),
				Hostname: host,
			}); err == nil {
				resp := eventbus.NewEvent("system", "agent/lifecycle.status", "gapid", anyPayload, true)
				_ = bus.Publish(resp)
			}
			return
		}

		// Immediate ACK: PENDING (unblock the client)
		if anyPayload, err := anypb.New(&protopkg.LifecycleStatus{
			AgentId:  targetID,
			State:    "PENDING",
			Message:  "accepted",
			Time:     timestamppb.Now(),
			Hostname: host,
		}); err == nil {
			resp := eventbus.NewEvent("system", "agent/lifecycle.status", "gapid", anyPayload, true)
			_ = bus.Publish(resp)
		}

		// Apply action
		action := actionFromEnum(cmd.GetAction())
		var applyErr error
		switch action {
		case "initialize":
			applyErr = ag.Controller().Apply(lifecycle.ActionInitialize)
		case "start":
			applyErr = ag.Controller().Apply(lifecycle.ActionStart)
		case "stop":
			applyErr = ag.Controller().Apply(lifecycle.ActionStop)
		case "reload":
			applyErr = ag.Controller().Apply(lifecycle.ActionReload)
		case "restart":
			applyErr = ag.Controller().Apply(lifecycle.ActionRestart)
		default:
			applyErr = fmt.Errorf("unknown action %q", action)
		}

		if applyErr != nil {
			// Don’t knee-jerk FAILED. If the controller is already at/heading toward the
			// desired state (e.g., "invalid transition: starting → starting"), prefer truth.
			finalState := ag.Controller().State()

			// If we asked for start and we're already running or starting, treat as success-ish.
			// If we asked for stop and we're already stopped or stopping, also success-ish.
			// Otherwise, report FAILED with the error.
			wanted := action // "start", "stop", ...
			isOkStart := (wanted == "start") && (strings.EqualFold(finalState, lifecycle.StateRunning) || strings.EqualFold(finalState, lifecycle.StateStarting))
			isOkStop := (wanted == "stop") && (strings.EqualFold(finalState, lifecycle.StateStopped) || strings.EqualFold(finalState, lifecycle.StateStopping))

			state := finalState
			msg := "ok"
			if !(isOkStart || isOkStop) {
				state = lifecycle.StateError
				msg = applyErr.Error()
			}

			if anyPayload, err := anypb.New(&protopkg.LifecycleStatus{
				AgentId:  targetID,
				State:    state,
				Message:  msg,
				Time:     timestamppb.Now(),
				Hostname: host,
			}); err == nil {
				resp := eventbus.NewEvent("system", "agent/lifecycle.status", "gapid", anyPayload, true)
				_ = bus.Publish(resp)
			}

			logger.Error().Err(applyErr).
				Str("agent_id", targetID).
				Str("action", action).
				Str("state", finalState).
				Msg("lifecycle apply returned error; replied with observed state")
			return
		}

		// Wait briefly for a stable (terminal) state before publishing final.
		deadline := time.Now().Add(20 * time.Second) // tune as needed
		finalState := ag.Controller().State()

		for isInFlight(finalState) && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			finalState = ag.Controller().State()
		}

		msg := "ok"
		if isInFlight(finalState) {
			// we gave it time but it hasn't settled; report what we have
			msg = "finalize timeout; current state=" + finalState
		}

		// Final reply with (settled or best-known) state
		if anyPayload, err := anypb.New(&protopkg.LifecycleStatus{
			AgentId:  targetID,
			State:    finalState,
			Message:  msg,
			Time:     timestamppb.Now(),
			Hostname: host,
		}); err == nil {
			resp := eventbus.NewEvent("system", "agent/lifecycle.status", "gapid", anyPayload, true)
			_ = bus.Publish(resp)
		}

		logger.Info().
			Str("agent_id", targetID).
			Str("action", action).
			Str("state", finalState).
			Msg("lifecycle applied (final)")

		logger.Info().
			Str("agent_id", targetID).
			Str("action", action).
			Str("state", finalState).
			Msg("lifecycle applied")
	})

	logger.Info().Str("host", host).Msg("supervisor running")

	// Wait for shutdown signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigs

	logger.Warn().Str("signal", sig.String()).Msg("received shutdown signal")
	if err := manager.StopAll(); err != nil {
		logger.Error().Err(err).Msg("graceful stop of all agents failed")
	}

	logevent.Lifecycle(logger, "gapid", "stop", "gapid", version.BinaryVersion())
	logger.Info().Msg("exited cleanly")
	return nil
}

func actionFromEnum(act protopkg.LifecycleControl_Action) string {
	switch act {
	case protopkg.LifecycleControl_START:
		return "start"
	case protopkg.LifecycleControl_STOP:
		return "stop"
	case protopkg.LifecycleControl_RELOAD:
		return "reload"
	case protopkg.LifecycleControl_RESTART:
		return "restart"
	default:
		return "initialize" // default safe fallback
	}
}
