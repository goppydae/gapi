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
	bus     *eventbus.EventBus
}

func NewLifecycleStateMachine(agentID, hostname string, bus *eventbus.EventBus) *LifecycleStateMachine {
	sm := state.NewBaseStateMachine("initialize", map[string][]string{
		"initialize": {"stopped"},
		"stopped":    {"starting"},
		"starting":   {"running", "error"},
		"running":    {"stopping", "reloading"},
		"reloading":  {"running", "error"},
		"stopping":   {"stopped", "error"},
		"error":      {"stopped"},
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
