package supervisor

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/goppydae/gapi/core/agentmgr"
	"github.com/goppydae/gapi/core/cgroups"
	"github.com/goppydae/gapi/core/clock"
	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/crypto"
	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/lifecycle"
	"github.com/goppydae/gapi/core/metrics"
	"github.com/goppydae/gapi/core/store"
	"github.com/goppydae/gapi/core/transport"
	"github.com/goppydae/gapi/core/version"
	"github.com/goppydae/gapi/internal/agentreg"
	"github.com/goppydae/gapi/internal/logattr"
	protopkg "github.com/goppydae/gapi/pkg/proto"
)

// Supervisor manages the GAPI runtime lifecycle.
type Supervisor struct {
	cfg           *config.Config
	logger        *slog.Logger
	manager       *agentmgr.AgentManager
	bus           *eventbus.EventBus[*anypb.Any]
	registry      *agentreg.AgentRegistry
	host          string
	metricsServer *metrics.Server
	clock         clock.Clock
}

// New creates a new Supervisor instance.
func New(cfg *config.Config) (*Supervisor, error) {
	logger := slog.Default().With(logattr.Module("gapid"))
	host, _ := os.Hostname()

	// Cgroups setup
	if err := cgroups.Setup(); err != nil {
		logger.LogAttrs(context.Background(), slog.LevelWarn, "failed to setup cgroups, resource limits will be unavailable", logattr.Err(err))
	} else {
		logger.LogAttrs(context.Background(), slog.LevelInfo, "cgroups setup complete")
	}

	// Transport
	t, err := transport.NewServerFromConfig(cfg.Transport)
	if err != nil {
		return nil, fmt.Errorf("transport init: %w", err)
	}

	bus := eventbus.NewEventBus[*anypb.Any](t)
	typedBus := lifecycle.TypedBus{}

	// Security: Verification Key (loaded before the agent manager so
	// production-mode discovery can verify signatures; review R20)
	var pubKey *ed25519.PublicKey
	// Check config first, then env
	kp := cfg.Security.VerifyKey
	if kp == "" {
		kp = os.Getenv("RUNTIME_VERIFY_KEY")
	}

	if kp != "" {
		pk, err := crypto.LoadPublic(kp)
		if err != nil {
			return nil, fmt.Errorf("failed to load verification key %q: %w", kp, err)
		}
		logger.LogAttrs(context.Background(), slog.LevelDebug, "integrity verification enabled", logattr.KeyPath(kp))
		pubKey = &pk
	}

	// Agent Manager
	pyRunner := resolvePyRunner()
	var discoveryKey ed25519.PublicKey
	if pubKey != nil {
		discoveryKey = *pubKey
	}
	manager := agentmgr.NewAgentManager(bus, &typedBus, pyRunner, cfg.Supervisor.ProductionMode, discoveryKey)

	// Store & Registry
	raw, err := store.Open(store.Hybrid)
	if err != nil {
		// The registry is not optional: without it, agent integrity verification
		// is silently skipped and later lookups panic on a nil registry. Fail
		// construction instead of returning a half-built supervisor.
		return nil, fmt.Errorf("open store: %w", err)
	}

	db, ok := raw.(store.HybridStore)
	if !ok {
		// Should we fail hard? Logic in cmd did not return error, but printed specific error inside NewAgentRegistry if cast failed?
		// Actually cmd checked cast.
		return nil, fmt.Errorf("failed to cast store to HybridStore")
	}
	registry, err := agentreg.NewAgentRegistry(db, pubKey)
	if err != nil {
		return nil, fmt.Errorf("create agent registry: %w", err)
	}

	s := &Supervisor{
		cfg:      cfg,
		logger:   logger,
		manager:  manager,
		bus:      bus,
		registry: registry,
		host:     host,
		clock:    clock.RealClock{},
	}

	// Initialize build info metrics and create server if enabled
	if cfg.Metrics.Enabled {
		metrics.BuildInfo.WithLabelValues(
			version.GAPIVersion,
			version.Commit,
			runtime.Version(),
		).Set(1)

		s.metricsServer = metrics.NewServer(cfg.Metrics.Addr, logger)
		logger.LogAttrs(context.Background(), slog.LevelDebug, "metrics enabled", logattr.Addr(cfg.Metrics.Addr))
	}

	return s, nil
}

// Bus returns the internal event bus.
func (s *Supervisor) Bus() *eventbus.EventBus[*anypb.Any] {
	return s.bus
}

// Start runs the supervisor logic. It blocks until Stop is called or an error occurs.

// Note: In a real library, Start might be non-blocking or accept a context.
// For now, we mirror the existing blocking behavior but allow external control via context cancellation if needed?
// The original used `runSupervisor()` which blocked on signal.
// We'll expose `Run()` which sets up handlers and blocks.
func (s *Supervisor) Run(ctx context.Context) error {
	s.logger.LogAttrs(context.Background(), slog.LevelInfo, "starting gapid supervisor")

	// Setup Agents
	s.setupAgents()

	// Register Event Handlers
	s.registerHandlers()

	s.logger.LogAttrs(context.Background(), slog.LevelInfo, "supervisor running", logattr.Host(s.host))

	// Start periodic metrics collection if enabled
	var metricsTicker *time.Ticker
	if s.cfg.Metrics.Enabled {
		metricsTicker = time.NewTicker(15 * time.Second)
		defer metricsTicker.Stop()

		startTime := s.clock.Now()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-metricsTicker.C:
					s.collectMetrics(startTime)
				}
			}
		}()
		s.logger.LogAttrs(context.Background(), slog.LevelInfo, "metrics collection started")

		// Start metrics HTTP server
		if s.metricsServer != nil {
			go func() {
				if err := s.metricsServer.Start(); err != nil {
					s.logger.LogAttrs(ctx, slog.LevelError, "metrics server failed", logattr.Err(err))
				}
			}()
			s.logger.LogAttrs(context.Background(), slog.LevelInfo, "metrics server started", logattr.Addr(s.cfg.Metrics.Addr))
		}
	}

	// Wait for context done
	<-ctx.Done()

	s.logger.LogAttrs(context.Background(), slog.LevelWarn, "received shutdown signal via context")

	// Shutdown metrics server if running
	if s.metricsServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), config.SupervisorShutdownTimeout)
		defer cancel()
		if err := s.metricsServer.Stop(shutdownCtx); err != nil {
			s.logger.LogAttrs(context.Background(), slog.LevelError, "metrics server shutdown failed", logattr.Err(err))
		} else {
			s.logger.LogAttrs(context.Background(), slog.LevelInfo, "metrics server stopped")
		}
	}

	if err := s.manager.StopAll(); err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "graceful stop of all agents failed", logattr.Err(err))
	}

	s.logger.LogAttrs(context.Background(), slog.LevelInfo, "lifecycle event",
		logattr.Event("lifecycle"), logattr.Source("gapid"), logattr.Action("stop"),
		logattr.AgentID("gapid"), logattr.Version(version.BinaryVersion()))
	s.logger.LogAttrs(context.Background(), slog.LevelInfo, "exited cleanly")
	return nil
}

func (s *Supervisor) setupAgents() {
	s.logger.LogAttrs(context.Background(), slog.LevelInfo, "performing agent discovery and setup")

	// Use new search path system
	discovered, err := s.manager.DiscoverFromPaths()
	if err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "Agent discovery failed", logattr.Err(err))
		return
	}

	// Register with DB
	for _, desc := range discovered {
		id := desc["id"]
		ad := &agentreg.AgentDescription{
			ID:           id,
			Path:         desc["path"],
			Type:         desc["type"],
			Language:     desc["language"],
			Version:      desc["version"],
			Hash:         desc["hash"],
			Tags:         splitCSV(desc["tags"]),
			Requires:     splitCSV(desc["requires"]),
			Wants:        splitCSV(desc["wants"]),
			WantedBy:     splitCSV(desc["wanted_by"]),
			RequiredBy:   splitCSV(desc["required_by"]),
			Capabilities: splitCSV(desc["capabilities"]),
		}
		if len(ad.Requires) == 0 && desc["deps"] != "" {
			ad.Requires = splitCSV(desc["deps"])
		}
		if err := s.registry.Register(ad); err != nil {
			s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to register discovered agent", logattr.Err(err), logattr.AgentID(ad.ID))
		}
	}

	// Topological startup
	sortedIDs, err := s.manager.TopologicalSort()
	if err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelWarn, "topological sort failed, falling back to random order", logattr.Err(err))
		allAgents := s.manager.All()
		ids := make([]string, 0, len(allAgents))
		for id := range allAgents {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		sortedIDs = append(sortedIDs, ids...)
	}

	// Track successfully started/armed agents for dependency resolution
	startedAgents := make(map[string]bool)

	if len(sortedIDs) == 0 {
		s.logger.LogAttrs(context.Background(), slog.LevelWarn, "no agents registered in manager")
	} else {
		for _, id := range sortedIDs {
			// Integrity check
			if _, err := s.registry.Lookup(id); err != nil {
				s.logger.LogAttrs(context.Background(), slog.LevelWarn, "skipping startup of unregistered agent (integrity failure?)", logattr.AgentID(id))
				continue
			}

			ag := s.manager.Get(id)
			if ag == nil {
				continue
			}

			// Dependency Check
			// We only enforce 'Requires'. 'Wants' are advisory.
			missingReq := ""
			for _, req := range ag.Requires() {
				if !startedAgents[req] {
					missingReq = req
					break
				}
			}
			if missingReq != "" {
				s.logger.LogAttrs(context.Background(), slog.LevelWarn, "skipping start due to missing or failed dependency", logattr.AgentID(id), logattr.MissingDependency(missingReq))
				continue
			}

			s.logger.LogAttrs(context.Background(), slog.LevelDebug, "registered agent", logattr.AgentID(id))

			desc := ag.Describe()
			started := false

			// lazy Activation
			if desc["listen_stream"] != "" {
				if armable, ok := ag.(interface {
					Arm() error
					SetTrafficHandler(func())
					Controller() *lifecycle.Controller
				}); ok {
					ctrl := armable.Controller()
					armable.SetTrafficHandler(func() {
						s.logger.LogAttrs(context.Background(), slog.LevelInfo, "traffic detected, triggering lazy start", logattr.AgentID(id))
						if err := ctrl.Apply(lifecycle.ActionStart); err != nil {
							s.logger.LogAttrs(context.Background(), slog.LevelError, "lazy start failed", logattr.Err(err), logattr.AgentID(id))
						}
					})
					if err := armable.Arm(); err != nil {
						s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to arm lazy activation", logattr.Err(err), logattr.AgentID(id))
					} else {
						s.logger.LogAttrs(context.Background(), slog.LevelInfo, "armed lazy activation", logattr.AgentID(id))
						started = true
					}
				}
			}

			// Timer auto-start
			if desc["type"] == "timer" {
				ctrl := ag.Controller()
				if err := ctrl.Apply(lifecycle.ActionStart); err != nil {
					s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to start timer agent", logattr.Err(err), logattr.AgentID(id))
				} else {
					s.logger.LogAttrs(context.Background(), slog.LevelInfo, "timer agent started", logattr.AgentID(id))
					started = true
				}
			}

			// Standard Service/Oneshot auto-start (if not lazy/timer)
			// (We assume 'service' or 'oneshot' type and no listen_stream means it should start immediately)
			if (desc["type"] == "service" || desc["type"] == "oneshot") && desc["listen_stream"] == "" {
				ctrl := ag.Controller()
				// We should verify enabled? implicit enabled for now.
				if err := ctrl.Apply(lifecycle.ActionStart); err != nil {
					s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to start agent", logattr.Err(err), logattr.AgentID(id))
				} else {
					s.logger.LogAttrs(context.Background(), slog.LevelInfo, "agent started", logattr.AgentID(id))
					started = true
				}
			}

			if started {
				startedAgents[id] = true
			}
		}
	}

	s.logger.LogAttrs(context.Background(), slog.LevelInfo, "agent setup complete")
}

func (s *Supervisor) registerHandlers() {
	// Ping/Pong
	err := s.bus.SubscribePrefix("system", "", "ping", func(e eventbus.Event[*anypb.Any]) {
		s.logger.LogAttrs(context.Background(), slog.LevelDebug, "received ping, preparing pong", logattr.Event("handling_ping"), logattr.EventID(e.ID))
		s.logger.LogAttrs(context.Background(), slog.LevelInfo, "lifecycle event",
			logattr.Event("lifecycle"), logattr.Source("gapid"), logattr.Action("handle_ping"),
			logattr.AgentID("gapid"), logattr.Version(version.BinaryVersion()))

		pong := &protopkg.PingStatus{Status: "pong"}
		anyPayload, err := anypb.New(pong)
		if err != nil {
			s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to pack pong payload", logattr.Err(err))
			return
		}

		response := eventbus.NewEvent("system", "", "pong", "gapid", anyPayload, true)
		response.ID = e.ID // correlate reply to the originating request
		_ = s.bus.Publish(response)
	})
	if err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to subscribe to ping event", logattr.Err(err))
	}

	// Agent Status
	err = s.bus.SubscribePrefix("system", "", "agents/", func(e eventbus.Event[*anypb.Any]) {
		s.logger.LogAttrs(context.Background(), slog.LevelDebug, "received agent status request", logattr.EventID(e.ID))

		entries, err := s.registry.List()
		if err != nil {
			s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to list agents", logattr.Err(err))
			return
		}

		var agentStatuses []*protopkg.AgentStatus
		for _, entry := range entries {
			st := protopkg.AgentState_AGENT_STATE_UNSPECIFIED
			if ag := getAgentCI(s.manager, entry.ID); ag != nil {
				st = mapStateToProto(ag.Controller().State())
			}
			deps, err := s.registry.GetDependencies(entry.ID)
			if err != nil {
				deps = entry.Requires
				s.logger.LogAttrs(context.Background(), slog.LevelWarn, "failed to resolve graph deps", logattr.Err(err), logattr.AgentID(entry.ID))
			}

			// Collect metrics from cgroups if available
			var cpuUsage float64
			var memUsage uint64
			cgName := fmt.Sprintf("gapid-%s", entry.ID)
			if stats, err := cgroups.GetStats(cgName); err == nil {
				cpuUsage = stats.CPUUsage
				if stats.MemoryUsage > 0 {
					memUsage = uint64(stats.MemoryUsage)
				}
			}

			// Calculate uptime
			var uptimeNs int64 = 0
			if uptimeable, ok := getAgentCI(s.manager, entry.ID).(interface{ Uptime() time.Duration }); ok {
				uptimeNs = int64(uptimeable.Uptime())
			}

			agentStatuses = append(agentStatuses, &protopkg.AgentStatus{
				Id:           entry.ID,
				Type:         entry.Type,
				State:        st,
				Dependencies: deps,
				Capabilities: entry.Capabilities,
				CpuUsage:     cpuUsage,
				MemoryUsage:  memUsage,
				UptimeNs:     uptimeNs,
			})
		}

		reply := &protopkg.AgentStatusResponse{Agents: agentStatuses}
		anyPayload, err := anypb.New(reply)
		if err != nil {
			s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to pack agent status response", logattr.Err(err))
			return
		}

		response := eventbus.NewEvent("system", "", "agents.reply", "gapid", anyPayload, true)
		response.ID = e.ID // correlate reply to the originating request
		_ = s.bus.Publish(response)
	})
	if err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to subscribe to agents event", logattr.Err(err))
	}

	// Reload
	err = s.bus.Subscribe("system", "", "agent.reload", func(e eventbus.Event[*anypb.Any]) {
		s.logger.LogAttrs(context.Background(), slog.LevelDebug, "received agent reload request", logattr.EventID(e.ID))
		s.setupAgents()
	})
	if err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to subscribe to reload event", logattr.Err(err))
	}

	// Lifecycle Actions
	err = s.bus.Subscribe("system", "", eventbus.TopicAgentLifecycleAction, func(e eventbus.Event[*anypb.Any]) {
		s.handleLifecycleAction(e)
	})
	if err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to subscribe to lifecycle event", logattr.Err(err))
	}
}

func (s *Supervisor) handleLifecycleAction(e eventbus.Event[*anypb.Any]) {
	s.logger.LogAttrs(context.Background(), slog.LevelDebug, "received lifecycle control event", logattr.EventID(e.ID), logattr.Topic(e.Topic))

	var cmd protopkg.LifecycleControl
	if err := eventbus.UnmarshalAnyPayload(e, &cmd); err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to decode LifecycleControl", logattr.Err(err))
		s.replyStatus(cmd.GetAgentId(), "FAILED", "decode error: "+err.Error())
		return
	}

	targetID := strings.TrimSpace(cmd.GetAgentId())
	ag := getAgentCI(s.manager, targetID)
	if ag == nil {
		s.logger.LogAttrs(context.Background(), slog.LevelWarn, "unknown agent in lifecycle control", logattr.AgentID(targetID))
		s.replyStatus(targetID, "FAILED", "unknown agent")
		return
	}

	// ACK
	s.replyStatus(targetID, "PENDING", "accepted")

	action := actionFromEnum(cmd.GetAction())
	desc := ag.Describe()
	var applyErr error
	switch action {
	case "initialize":
		applyErr = ag.Controller().Apply(lifecycle.ActionInitialize)
	case "start":
		applyErr = ag.Controller().Apply(lifecycle.ActionStart)
		if applyErr == nil {
			// Record successful start
			metrics.RecordAgentStart(targetID, desc["type"])
		}
	case "stop":
		applyErr = ag.Controller().Apply(lifecycle.ActionStop)
		if applyErr == nil {
			// Record successful stop
			metrics.RecordAgentStop(targetID, desc["type"])
		}
	case "reload":
		applyErr = ag.Controller().Apply(lifecycle.ActionReload)
	case "restart":
		applyErr = ag.Controller().Apply(lifecycle.ActionRestart)
		if applyErr == nil {
			// Record restart as stop + start
			metrics.RecordAgentStop(targetID, desc["type"])
			metrics.RecordAgentStart(targetID, desc["type"])
		}
	default:
		applyErr = fmt.Errorf("unknown action %q", action)
	}

	if applyErr != nil {
		finalState := ag.Controller().State()
		wanted := action
		isOkStart := (wanted == "start") && (strings.EqualFold(finalState, lifecycle.StateRunning) || strings.EqualFold(finalState, lifecycle.StateStarting))
		isOkStop := (wanted == "stop") && (strings.EqualFold(finalState, lifecycle.StateStopped) || strings.EqualFold(finalState, lifecycle.StateStopping))

		state := finalState
		msg := "ok"
		if !isOkStart && !isOkStop {
			state = lifecycle.StateError
			msg = applyErr.Error()
			// Record failure
			metrics.RecordAgentFailure(targetID, desc["type"], action)
		}

		s.replyStatus(targetID, state, msg)
		s.logger.LogAttrs(context.Background(), slog.LevelError, "lifecycle apply returned error; replied with observed state", logattr.Err(applyErr), logattr.AgentID(targetID), logattr.Action(action), slog.String("state", finalState))
		return
	}

	// Controller.Apply is synchronous: it only returns once the agent has
	// reached a terminal state (it internally awaits running/stopped via the
	// event bus). So the observed state is already settled here — no busy poll
	// loop is needed. If a future async runner leaves it in flight, surface that
	// rather than silently sleeping.
	finalState := ag.Controller().State()

	msg := "ok"
	if isInFlight(finalState) {
		msg = "still settling; current state=" + finalState
	}

	s.replyStatus(targetID, finalState, msg)
	s.logger.LogAttrs(context.Background(), slog.LevelInfo, "lifecycle applied (final)", logattr.AgentID(targetID), logattr.Action(action), slog.String("state", finalState))
}

func (s *Supervisor) replyStatus(agentID, state, msg string) {
	if anyPayload, err := anypb.New(&protopkg.LifecycleStatus{
		AgentId:  agentID,
		State:    state,
		Message:  msg,
		Time:     timestamppb.Now(),
		Hostname: s.host,
	}); err == nil {
		resp := eventbus.NewEvent("system", "", eventbus.TopicAgentLifecycleStatus, "gapid", anyPayload, true)
		_ = s.bus.Publish(resp)
	}
}

// Helpers duplicated from original but now unexported helpers of Supervisor or package
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
		return protopkg.AgentState_AGENT_STATE_STARTING
	case lifecycle.StateError:
		return protopkg.AgentState_AGENT_STATE_FAILED
	default:
		return protopkg.AgentState_AGENT_STATE_UNSPECIFIED
	}
}

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
	allAgents := mgr.All()
	ids := make([]string, 0, len(allAgents))
	for k := range allAgents {
		ids = append(ids, k)
	}
	sort.Strings(ids)

	for _, k := range ids {
		ag := allAgents[k]
		if strings.ToLower(k) == idLower {
			return ag
		}
		if desc := ag.Describe(); strings.ToLower(desc["id"]) == idLower {
			return ag
		}
	}
	return nil
}

func isInFlight(state string) bool {
	s := strings.ToUpper(strings.TrimSpace(state))
	switch s {
	case "PENDING", "STARTING", "STOPPING", "RELOADING", "INITIALIZING", "":
		return true
	default:
		return false
	}
}

func resolvePyRunner() string {
	if v := os.Getenv("RUNTIME_PY_RUNNER"); v != "" {
		return v
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		cand := filepath.Join(dir, "adk", "python", "agent", "runner.py")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return filepath.Join("adk", "python", "agent", "runner.py")
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func actionFromEnum(act protopkg.LifecycleControl_Action) string {
	switch act {
	case protopkg.LifecycleControl_ACTION_START:
		return "start"
	case protopkg.LifecycleControl_ACTION_STOP:
		return "stop"
	case protopkg.LifecycleControl_ACTION_RELOAD:
		return "reload"
	case protopkg.LifecycleControl_ACTION_RESTART:
		return "restart"
	default:
		return "initialize"
	}
}
