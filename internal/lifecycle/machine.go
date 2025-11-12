package lifecycle

import (
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/goppydae/gapi/internal/eventbus"
	protopkg "github.com/goppydae/gapi/internal/proto"
	"github.com/goppydae/gapi/internal/state"
)

type LifecycleStateMachine struct {
	sm      *state.BaseStateMachine
	agentID string
	host    string
	bus     *eventbus.EventBus[*anypb.Any]
}

func NewLifecycleStateMachine(agentID, hostname string, bus *eventbus.EventBus[*anypb.Any]) *LifecycleStateMachine {
	sm := state.NewBaseStateMachine("initializing", map[string][]string{
		"initializing": {"restarting", "starting", "stopped", "error"},
		"stopped":      {"starting", "error"},
		"starting":     {"running", "error"},
		"running":      {"stopping", "reloading", "restarting", "error"},
		"reloading":    {"running", "error"},
		"stopping":     {"stopped", "error"},
		"restarting":   {"running", "error"},
		"error":        {"stopping", "stopped", "starting", "restarting"},
	})

	lsm := &LifecycleStateMachine{
		sm:      sm,
		agentID: agentID,
		host:    hostname,
		bus:     bus,
	}

	sm.OnTransition(func(from, to string) {
		lsm.emitLifecycleEvent(from, to)
	})

	return lsm
}

func (lsm *LifecycleStateMachine) emitLifecycleEvent(from, to string) {
	msg := &protopkg.LifecycleTransition{
		AgentId:  lsm.agentID,
		From:     from,
		To:       to,
		Hostname: lsm.host,
		Time:     timestamppb.Now(),
	}

	anyMsg, err := anypb.New(msg)
	if err != nil {
		return // optionally log
	}

	event := eventbus.NewEvent(
		"system",
		"lifecycle.transition",
		lsm.agentID,
		anyMsg,
		true,
	)

	lsm.bus.Publish(event)
}

func (lsm *LifecycleStateMachine) TransitionTo(newState string) error {
	return lsm.sm.TransitionTo(newState)
}

func (lsm *LifecycleStateMachine) GetState() string {
	return lsm.sm.GetState()
}

func (lsm *LifecycleStateMachine) CurrentProtoState() protopkg.AgentState {
	switch lsm.GetState() {
	case "initializing":
		return protopkg.AgentState_AGENT_STATE_INIT
	case "starting":
		return protopkg.AgentState_AGENT_STATE_STARTING
	case "running":
		return protopkg.AgentState_AGENT_STATE_RUNNING
	case "stopping":
		return protopkg.AgentState_AGENT_STATE_STOPPING
	case "stopped":
		return protopkg.AgentState_AGENT_STATE_STOPPED
	case "failed":
		return protopkg.AgentState_AGENT_STATE_FAILED
	case "reloading":
		return protopkg.AgentState_AGENT_STATE_RELOADING
	default:
		return protopkg.AgentState_AGENT_STATE_UNKNOWN
	}
}
