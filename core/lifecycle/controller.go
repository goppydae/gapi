// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package lifecycle

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/goppydae/gapi/internal/ident"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/goppydae/gapi/core/budget"
	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/internal/logattr"
	protopkg "github.com/goppydae/gapi/pkg/proto"
)

type Action string

const (
	ActionInitialize Action = "initialize"
	ActionStart      Action = "start"
	ActionStop       Action = "stop"
	ActionRestart    Action = "restart"
	ActionReload     Action = "reload"
)

type TypedBus = eventbus.EventBus[*anypb.Any]

type statusEvt struct {
	state string
	when  time.Time
	runID string
}

type Controller struct {
	id        string
	host      string
	runner    Runner
	sm        *LifecycleStateMachine
	bus       *TypedBus
	deps      DependencyResolver
	GraceStop time.Duration
	WaitStop  time.Duration
	stateCh   chan statusEvt // single, long-lived feed

	// WaitStart WAS ONE FIELD DOING THREE JOBS, and GAPI-DIV-107 is the
	// entry for the first two. It was 10s for every agent of every
	// language, read at three call sites, and each site was bounding a
	// different phenomenon:
	//
	//   - the spawn call itself (fork, exec, pipes, sockets)
	//   - first frame to RUNNING, the agent's own start()
	//   - the post-RELOAD wait, which is not a start at all
	//
	// Splitting them is the point of the entry. Naming them separately
	// is what stops the next change from moving one and silently taking
	// the other two with it - twice in one day this project moved a
	// value to where it belonged and removed a job it had been doing in
	// secret (STARTING was also the concurrency guard; WaitStart was
	// also the silence budget).
	//
	// ReadinessBudget is per-agent and is the one a descriptor can
	// declare. SilenceBudget and SpawnBudget are supervisor policy and
	// are not declarable; see core/budget for why.
	ReadinessBudget time.Duration
	SilenceBudget   time.Duration
	SpawnBudget     time.Duration

	// startMu/starting are the re-entrancy guard for ActionStart.
	//
	// THIS EXISTS BECAUSE STATE STOPPED BEING THE GUARD (operator
	// decision 42, GAPI-DIV-104). The early transition to StateStarting
	// used to be set before the dependency walk, and the switch at the
	// top of ActionStart read it to make a concurrent start a no-op - so
	// one field was serving as both the reported state and the in-flight
	// marker. Moving the transition to after the exec, where STARTING
	// becomes an observation rather than an intention, takes the marker
	// away with it and leaves a window in which two callers could both
	// walk the tree and both spawn.
	//
	// An explicit marker is the honest form: the two facts have
	// different lifetimes and only one of them is anybody else's
	// business.
	startMu  sync.Mutex
	starting bool
}

// beginStart claims the start-in-flight marker, reporting false if
// another caller already holds it.
func (c *Controller) beginStart() bool {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	if c.starting {
		return false
	}
	c.starting = true
	return true
}

func (c *Controller) endStart() {
	c.startMu.Lock()
	c.starting = false
	c.startMu.Unlock()
}

// depCtxKey is an unexported context-key type so the dependency-cycle
// stack cannot collide with other packages' context values (SA1029).
type depCtxKey struct{}

// DepCtxKey is used to store the call stack for cycle detection
var DepCtxKey = depCtxKey{}

type DependencyResolver interface {
	DepsOf(id string) []string
	IsRunning(id string) bool
	EnsureStarted(ctx context.Context, id string) error
}

func NewController(id, host string, r Runner, bus *TypedBus, deps DependencyResolver) *Controller {
	c := &Controller{
		id:        id,
		host:      host,
		runner:    r,
		sm:        NewLifecycleStateMachine(id, host, bus),
		bus:       bus,
		deps:      deps,
		GraceStop: 3 * time.Second,
		WaitStop:  5 * time.Second,
		stateCh:   make(chan statusEvt, 64),

		// THE CONTROLLER DOES NOT KNOW THE AGENT'S LANGUAGE, so these
		// are the derivation's answer for a language it has never
		// measured - the most generous one it has. A runner that knows
		// its language narrows them immediately after construction
		// (core/agentmgr), and discovery narrows ReadinessBudget again
		// if the descriptor declared one.
		//
		// Generous rather than absent: a zero here would be a deadline
		// that fires instantly, and a controller built by a caller that
		// forgets to narrow them should be slow, not broken.
		ReadinessBudget: budget.DefaultReadinessBudget(""),
		SilenceBudget:   budget.SilenceBudget(""),
		SpawnBudget:     budget.Spawn,
	}

	_ = c.bus.Subscribe("system", "", eventbus.TopicAgentLifecycleStatus, func(ev eventbus.Event[*anypb.Any]) {
		if ev.Payload == nil {
			return
		}
		var st protopkg.LifecycleStatus
		if err := ev.Payload.UnmarshalTo(&st); err != nil {
			return
		}
		if st.AgentId != c.id {
			return
		}
		got := strings.ToLower(strings.TrimSpace(st.State))
		if got == "" {
			got = strings.ToLower(strings.TrimSpace(st.GetState()))
		}
		if got == "" {
			return
		}
		when := time.Now()
		if st.Time != nil {
			when = st.Time.AsTime()
		}
		// Structural only (R16): every in-tree producer populates the
		// run_id field; the legacy message-text parse is gone.
		runID := strings.TrimSpace(st.GetRunId())

		evt := statusEvt{state: got, when: when, runID: runID}
		select {
		case c.stateCh <- evt:
		default:
			// Channel full: evict the oldest event to make room for the newest,
			// so a fresh running/terminal status is never lost behind a backlog
			// of stale events during rapid start/stop churn (which previously
			// caused awaitRunning to time out on a healthy agent).
			select {
			case <-c.stateCh:
			default:
			}
			select {
			case c.stateCh <- evt:
			default:
			}
		}
	})

	return c
}

func (c *Controller) State() string { return c.sm.GetState() }

// dependencyClasses splits this controller's dependencies into hard (Requires)
// and soft (Wants). Hard failures block startup; soft failures only warn. If the
// resolver does not distinguish the two, every dependency is treated as hard so
// existing behavior is preserved.
func (c *Controller) dependencyClasses() (hard, soft []string) {
	type classified interface {
		HardDepsOf(id string) []string
		SoftDepsOf(id string) []string
	}
	if cr, ok := c.deps.(classified); ok {
		return cr.HardDepsOf(c.id), cr.SoftDepsOf(c.id)
	}
	return c.deps.DepsOf(c.id), nil
}

func (c *Controller) Apply(a Action) error {
	return c.ApplyWithContext(context.Background(), a)
}

func (c *Controller) ApplyWithContext(ctx context.Context, a Action) error {
	// Cycle Detection
	stack, _ := ctx.Value(DepCtxKey).([]string)
	for _, visited := range stack {
		if visited == c.id {
			return fmt.Errorf("dependency cycle detected: %s -> ... -> %s", strings.Join(stack, " -> "), c.id)
		}
	}
	stack = append(stack, c.id)
	ctx = context.WithValue(ctx, DepCtxKey, stack)

	switch a {
	case ActionInitialize:
		return c.sm.TransitionTo(StateInitializing)

	case ActionStart:
		switch c.sm.GetState() {
		case StateStarting, StateRunning, StateReloading:
			return nil
		}
		// The state check above is necessary and no longer sufficient:
		// STARTING is now set after the exec, so between here and there
		// a second caller sees a startable state. See beginStart.
		if !c.beginStart() {
			return nil
		}
		defer c.endStart()
		if c.deps != nil {
			hard, soft := c.dependencyClasses()
			for _, dep := range hard {
				if err := c.deps.EnsureStarted(ctx, dep); err != nil {
					return fmt.Errorf("dependency %q failed to start: %w", dep, err)
				}
			}
			// Soft (Wants) dependencies are advisory: a failure is logged and
			// startup continues, rather than blocking like a hard dependency.
			for _, dep := range soft {
				if err := c.deps.EnsureStarted(ctx, dep); err != nil {
					slog.Default().LogAttrs(context.Background(), slog.LevelWarn, "soft (wants) dependency failed to start; continuing", logattr.Module("lifecycle"), logattr.AgentID(c.id), logattr.Dependency(dep), logattr.Err(err))
				}
			}
		}
		c.publishControl(protopkg.LifecycleControl_ACTION_START)

		runID := ident.NewV7String()
		if s, ok := c.runner.(RunIDSetter); ok {
			s.SetRunID(runID)
		}

		// drain stale events
	Drain:
		for {
			select {
			case <-c.stateCh:
			default:
				break Drain
			}
		}
		cutover := time.Now()

		// spawn
		{
			// Derive from the caller's context so cancellation, deadlines, and
			// tracing propagate into the runner instead of being discarded.
			//
			// This context bounds the Start *call* and nothing else. cancel is
			// invoked as soon as Start returns rather than deferred to this
			// function's exit, so the window in which it could reach a started
			// process is as small as the language allows - and Runner
			// implementations must not tie a spawned process to it at all
			// (GAPI-DIV-028). Readiness has its own deadline below.
			//
			// SPAWN, NOT READINESS, AND DELIBERATELY NOT THE DECLARED
			// BUDGET. This site read WaitStart, but the comment above it
			// already said what it was for - "this context bounds the
			// Start call and nothing else" - so WaitStart was doing a
			// second job here, and repointing it at the per-agent
			// readiness budget would have handed a descriptor the power
			// to time out its own fork/exec. An agent declaring 500ms is
			// asking for its start() to be judged sooner, not for the
			// kernel to be given less time to exec it, and the failure
			// it would have produced is a raw context deadline from
			// Start rather than a StartTimeout - a different error shape
			// out of a field move nobody asked for.
			startCtx, cancel := context.WithTimeout(ctx, c.SpawnBudget)
			err := c.runner.Start(startCtx)
			cancel()
			if err != nil {
				_ = c.sm.TransitionTo(StateError)
				return err
			}
		}

		// STARTING NOW, AND NOT BEFORE (operator decision 42). Start
		// returned without error, so a child exists. The runner
		// published the observation with the run id at the instant of
		// the exec; this is the state machine agreeing with it.
		if err := c.sm.TransitionTo(StateStarting); err != nil {
			return err
		}

		// THE READINESS BUDGET IN PLACE OF WaitStart (GAPI-DIV-107).
		// This is the only one of the three sites that was bounding
		// first-frame-to-RUNNING, which is what the entry is about and
		// what a descriptor may declare. The silence budget rides along
		// because it bounds a strictly earlier part of the same wait.
		if err := c.awaitRunningWithRunIDSince(c.ReadinessBudget, c.SilenceBudget, runID, cutover); err != nil {
			_ = c.sm.TransitionTo(StateError)
			// A DEADLINE THAT EXPIRES IS REPORTED, NOT ONLY RETURNED.
			// The caller gets the error, but nothing on the bus said the
			// start failed - so an operator watching the status topic saw
			// STARTING and then nothing at all (GAPI-DIV-104).
			c.publishFailed(runID, err.Error())
			return fmt.Errorf("start: %w", err)
		}
		return c.sm.TransitionTo(StateRunning)

	case ActionStop:
		if c.sm.GetState() == StateStopped {
			return nil
		}

		if c.sm.GetState() != StateStopping {
			c.publishControl(protopkg.LifecycleControl_ACTION_STOP)
			if err := c.sm.TransitionTo(StateStopping); err != nil {
				return err
			}
		}

		// Graceful first, then kill by runner implementation.
		stopCtx, stopCancel := context.WithTimeout(context.Background(), c.GraceStop)
		_ = c.runner.Stop(stopCtx) // ignore exact error; we take ownership
		stopCancel()

		// Always cleanup OS/process & transport handles.
		if x, ok := c.runner.(interface{ Reset() }); ok {
			x.Reset()
		}

		// Supervisor-owned STOPPED so clients don't hang.
		c.publishStatus(protopkg.AgentState_AGENT_STATE_STOPPED, "process exited (supervisor-confirmed)")
		_ = c.sm.TransitionTo(StateStopped)
		return nil

	case ActionReload:
		if c.sm.GetState() != StateRunning {
			return c.ApplyWithContext(ctx, ActionStart)
		}
		c.publishControl(protopkg.LifecycleControl_ACTION_RELOAD)
		if err := c.sm.TransitionTo(StateReloading); err != nil {
			return err
		}
		// reloadCtx is for the actual reload operation, not cycle detection
		reloadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.runner.Reload(reloadCtx); err != nil {
			_ = c.sm.TransitionTo(StateError)
			return err
		}
		// A RELOAD IS NOT A START, and this site was the third job
		// WaitStart was doing. The interval is genuinely the readiness
		// one - an agent re-reading its config is running its own code,
		// which is exactly what the readiness budget is generous for -
		// so the declared budget is the right answer here. What it is
		// NOT is the silence budget: the child has been speaking for as
		// long as it has been running, so there is no silence question
		// to ask and awaitTarget is not asked it.
		if err := c.awaitTarget(c.ReadinessBudget, anyEnumToString(protopkg.AgentState_AGENT_STATE_RUNNING)); err != nil {
			_ = c.sm.TransitionTo(StateError)
			return fmt.Errorf("reload: %w", err)
		}
		return c.sm.TransitionTo(StateRunning)

	case ActionRestart:
		c.publishControl(protopkg.LifecycleControl_ACTION_RESTART)
		if err := c.ApplyWithContext(ctx, ActionStop); err != nil {
			return fmt.Errorf("restart/stop: %w", err)
		}
		return c.ApplyWithContext(ctx, ActionStart)
	}
	return fmt.Errorf("unknown action %q", a)
}

func (c *Controller) publishControl(a protopkg.LifecycleControl_Action) {
	msg := &protopkg.LifecycleControl{AgentId: c.id, Action: a}
	anyMsg, _ := anypb.New(msg)
	// Advisory observability event: a publish failure is logged loudly but
	// must not abort the lifecycle action itself (aborting stop/start on a
	// closed bus would invert priorities during shutdown).
	if err := c.bus.Publish(eventbus.NewEvent("system", "", eventbus.TopicAgentLifecycleControl, c.id, anyMsg)); err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelError, "failed to publish lifecycle control event", logattr.Module("lifecycle"), logattr.AgentID(c.id), logattr.Err(err))
	}
}

func (c *Controller) publishStatus(state protopkg.AgentState, message string) {
	st := &protopkg.LifecycleStatus{
		AgentId:  c.id,
		State:    state.String(),
		Message:  message,
		Time:     timestamppb.Now(),
		Hostname: c.host,
	}
	anyMsg, _ := anypb.New(st)
	// Advisory observability event; see publishControl for the no-abort rationale.
	if err := c.bus.Publish(eventbus.NewEvent("system", "", eventbus.TopicAgentLifecycleStatus, c.id, anyMsg)); err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelError, "failed to publish lifecycle status event", logattr.Module("lifecycle"), logattr.AgentID(c.id), logattr.Err(err))
	}
}

// publishFailed announces a supervisor-observed failure on the status
// topic, carrying the run id so it can be told from the restart behind
// it. Advisory observability event; see publishControl.
func (c *Controller) publishFailed(runID, message string) {
	st := &protopkg.LifecycleStatus{
		AgentId:  c.id,
		State:    "FAILED",
		Message:  message,
		Time:     timestamppb.Now(),
		Hostname: c.host,
		RunId:    runID,
	}
	anyMsg, _ := anypb.New(st)
	if err := c.bus.Publish(eventbus.NewEvent("system", "", eventbus.TopicAgentLifecycleStatus, c.id, anyMsg)); err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelError, "failed to publish start-failure status", logattr.Module("lifecycle"), logattr.AgentID(c.id), logattr.Err(err))
	}
}

func anyEnumToString(e protopkg.AgentState) string {
	s := strings.TrimPrefix(e.String(), "AGENT_STATE_")
	return strings.ToLower(s)
}
