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
	bus  *eventbus.EventBus
	host string
}

func NewLifecycleController(fsms map[string]*LifecycleStateMachine, mgr AgentLookup, bus *eventbus.EventBus, host string) *LifecycleController {
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

	switch ctrl.Action {
	case protopkg.LifecycleControl_START:
		transition = "starting"
		err = agent.Start()
	case protopkg.LifecycleControl_STOP:
		transition = "stopping"
		err = agent.Stop()
	case protopkg.LifecycleControl_RESTART:
		transition = "restarting"
		_ = agent.Stop()
		err = agent.Start()
	case protopkg.LifecycleControl_RELOAD:
		transition = "reloading"
		err = agent.Reload()
	case protopkg.LifecycleControl_ACTION_UNSPECIFIED:
		lc.sendStatus(ctrl.AgentId, fsm.GetState(), "queried via ACTION_UNSPECIFIED")
		return
	default:
		lc.sendStatus(ctrl.AgentId, fsm.GetState(), "unsupported action")
		return
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

	event := eventbus.NewEvent("user", "agent/lifecycle.status", "gapid", payload, true)
	_ = lc.bus.Publish(event)
}
