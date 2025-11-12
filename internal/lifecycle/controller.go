package lifecycle

import (
	"fmt"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/goppydae/gapi/internal/eventbus"
	protopkg "github.com/goppydae/gapi/internal/proto"
)

// AgentLookup is the minimal interface needed from AgentManager
type AgentLookup interface {
	Get(id string) Agent
}

type LifecycleController struct {
	fsms map[string]*LifecycleStateMachine
	mgr  AgentLookup
	bus  *eventbus.EventBus[*anypb.Any]
	host string
}

func NewLifecycleController(fsms map[string]*LifecycleStateMachine, mgr AgentLookup, bus *eventbus.EventBus[*anypb.Any], host string) *LifecycleController {
	return &LifecycleController{
		fsms: fsms,
		mgr:  mgr,
		bus:  bus,
		host: host,
	}
}

func (lc *LifecycleController) Handle(ctrl *protopkg.LifecycleControl) {
	fsm, ok := lc.fsms[ctrl.AgentId]
	if !ok {
		lc.sendStatus(ctrl.AgentId, "unknown", "agent not registered")
		return
	}

	agent := lc.mgr.Get(ctrl.AgentId)
	if agent == nil {
		lc.sendStatus(ctrl.AgentId, fsm.GetState(), "agent not found")
		return
	}

	var err error
	var transition string

	current := fsm.GetState()

	switch ctrl.GetAction() {

	case protopkg.LifecycleControl_START:
		// No-op if we're already up or headed up.
		if current == "running" || current == "starting" {
			lc.sendStatus(ctrl.AgentId, fsm.GetState(), "noop: already running")
			return
		}

		// First START from "initialize" is now legal via relaxed FSM.
		if err := fsm.TransitionTo("starting"); err != nil {
			lc.sendStatus(ctrl.AgentId, fsm.GetState(), fmt.Sprintf("transition(starting) failed: %v", err))
			return
		}

		if err := agent.Start(); err != nil {
			_ = fsm.TransitionTo("error")
			lc.sendStatus(ctrl.AgentId, fsm.GetState(), fmt.Sprintf("start failed: %v", err))
			return
		}

		_ = fsm.TransitionTo("running")
		lc.sendStatus(ctrl.AgentId, fsm.GetState(), "started")

	case protopkg.LifecycleControl_STOP:
		// Idempotent stop.
		if current == "stopped" || current == "stopping" {
			lc.sendStatus(ctrl.AgentId, fsm.GetState(), "noop: already stopped")
			return
		}

		if err := fsm.TransitionTo("stopping"); err != nil {
			lc.sendStatus(ctrl.AgentId, fsm.GetState(), fmt.Sprintf("transition(stopping) failed: %v", err))
			return
		}

		if err := agent.Stop(); err != nil {
			_ = fsm.TransitionTo("error")
			lc.sendStatus(ctrl.AgentId, fsm.GetState(), fmt.Sprintf("stop failed: %v", err))
			return
		}

		_ = fsm.TransitionTo("stopped")
		lc.sendStatus(ctrl.AgentId, fsm.GetState(), "stopped")

	case protopkg.LifecycleControl_RESTART:
		// Dedicated restart path. This does NOT treat START as an implicit stop.
		// It only runs on the explicit RESTART action.
		if current != "restarting" {
			if err := fsm.TransitionTo("restarting"); err != nil {
				lc.sendStatus(ctrl.AgentId, fsm.GetState(), fmt.Sprintf("transition(restarting) failed: %v", err))
				return
			}
		}

		// Best-effort stop only if we were actually up/starting/reloading.
		if current == "running" || current == "starting" || current == "reloading" {
			if err := agent.Stop(); err != nil {
				_ = fsm.TransitionTo("error")
				lc.sendStatus(ctrl.AgentId, fsm.GetState(), fmt.Sprintf("restart/stop failed: %v", err))
				return
			}
		}

		if err := agent.Start(); err != nil {
			_ = fsm.TransitionTo("error")
			lc.sendStatus(ctrl.AgentId, fsm.GetState(), fmt.Sprintf("restart/start failed: %v", err))
			return
		}

		_ = fsm.TransitionTo("running")
		lc.sendStatus(ctrl.AgentId, fsm.GetState(), "restarted")

	case protopkg.LifecycleControl_RELOAD:
		// Only meaningful from running; otherwise, keep policy simple: no-op.
		if current != "running" {
			lc.sendStatus(ctrl.AgentId, fsm.GetState(), "noop: reload allowed only from running")
			return
		}

		if err := fsm.TransitionTo("reloading"); err != nil {
			lc.sendStatus(ctrl.AgentId, fsm.GetState(), fmt.Sprintf("transition(reloading) failed: %v", err))
			return
		}

		if err := agent.Reload(); err != nil {
			_ = fsm.TransitionTo("error")
			lc.sendStatus(ctrl.AgentId, fsm.GetState(), fmt.Sprintf("reload failed: %v", err))
			return
		}

		_ = fsm.TransitionTo("running")
		lc.sendStatus(ctrl.AgentId, fsm.GetState(), "reloaded")

	default:
		lc.sendStatus(ctrl.AgentId, fsm.GetState(), "unsupported action")
	}

	if err != nil {
		lc.sendStatus(ctrl.AgentId, fsm.GetState(), fmt.Sprintf("action failed: %v", err))
		return
	}

	_ = fsm.TransitionTo(transition)
	lc.sendStatus(ctrl.AgentId, fsm.GetState(), "transition OK")
}

func (lc *LifecycleController) sendStatus(agentID, state, message string) {
	reply := &protopkg.LifecycleStatus{
		AgentId:  agentID,
		State:    state,
		Message:  message,
		Time:     timestamppb.Now(),
		Hostname: lc.host,
	}

	payload, err := anypb.New(reply)
	if err != nil {
		return
	}

	event := eventbus.NewEvent("system", "agent/lifecycle.status", "gapid", payload, true)
	_ = lc.bus.Publish(event)
}
