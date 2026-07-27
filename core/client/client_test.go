package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
	protopkg "github.com/goppydae/gapi/pkg/proto"
)

// newTestDaemon wires responder handlers onto an in-proc bus that mimic the
// supervisor's request/reply behavior: every reply echoes the request's
// event ID (the correlation contract, review R15).
func newTestDaemon(t *testing.T) *eventbus.EventBus[*anypb.Any] {
	t.Helper()
	bus := eventbus.NewInprocBus[*anypb.Any]()
	t.Cleanup(func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	})

	if err := bus.Subscribe("system", "", "ping", func(e eventbus.Event[*anypb.Any]) {
		payload, err := anypb.New(&protopkg.PingStatus{Status: "pong"})
		if err != nil {
			t.Errorf("marshal pong: %v", err)
			return
		}
		reply := eventbus.NewEvent("system", "", "pong", "test-daemon", payload, false)
		reply.ID = e.ID // correlate reply to the originating request
		if err := bus.Publish(reply); err != nil {
			t.Errorf("publish pong: %v", err)
		}
	}); err != nil {
		t.Fatalf("subscribe ping: %v", err)
	}

	if err := bus.Subscribe("system", "", "agents/", func(e eventbus.Event[*anypb.Any]) {
		payload, err := anypb.New(&protopkg.AgentStatusResponse{
			Agents: []*protopkg.AgentStatus{{Id: "agent-1"}},
		})
		if err != nil {
			t.Errorf("marshal agents reply: %v", err)
			return
		}
		reply := eventbus.NewEvent("system", "", "agents.reply", "test-daemon", payload, false)
		reply.ID = e.ID
		if err := bus.Publish(reply); err != nil {
			t.Errorf("publish agents reply: %v", err)
		}
	}); err != nil {
		t.Fatalf("subscribe agents/: %v", err)
	}

	if err := bus.Subscribe("system", "", "agent/lifecycle.action", func(e eventbus.Event[*anypb.Any]) {
		var req protopkg.LifecycleControl
		if err := e.Payload.UnmarshalTo(&req); err != nil {
			t.Errorf("unmarshal lifecycle action: %v", err)
			return
		}
		payload, err := anypb.New(&protopkg.LifecycleStatus{
			AgentId: req.AgentId,
			State:   "RUNNING", // terminal per statewatch
		})
		if err != nil {
			t.Errorf("marshal lifecycle status: %v", err)
			return
		}
		status := eventbus.NewEvent("system", "", "agent/lifecycle.status", "test-daemon", payload, false)
		if err := bus.Publish(status); err != nil {
			t.Errorf("publish lifecycle status: %v", err)
		}
	}); err != nil {
		t.Fatalf("subscribe lifecycle action: %v", err)
	}

	return bus
}

// TestPing_ConcurrentCallersDoNotStealReplies is the R15 regression: with
// uncorrelated SubscribeOnce semantics, one caller would consume another's
// pong and the loser would time out.
func TestPing_ConcurrentCallersDoNotStealReplies(t *testing.T) {
	bus := newTestDaemon(t)
	c := NewFromBus(bus)

	const callers = 10
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, err := c.Ping(ctx)
			if err != nil {
				errs <- err
				return
			}
			if status != "pong" {
				t.Errorf("Ping status = %q, want pong", status)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Ping failed (stolen reply?): %v", err)
	}
}

func TestAgentStatus_ConcurrentCallersDoNotStealReplies(t *testing.T) {
	bus := newTestDaemon(t)
	c := NewFromBus(bus)

	const callers = 10
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			agents, err := c.AgentStatus(ctx)
			if err != nil {
				errs <- err
				return
			}
			if len(agents) != 1 || agents[0].Id != "agent-1" {
				t.Errorf("AgentStatus = %v, want [agent-1]", agents)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent AgentStatus failed (stolen reply?): %v", err)
	}
}

// TestLifecycle_DistinctAgentsNoCrossTalk verifies the agentID-keyed
// statewatch correlation: concurrent lifecycle actions on different agents
// each observe their own agent's transition. (Concurrent actions on the
// SAME agent share the agent's converging state stream by design; that is
// status observation, not reply theft.)
func TestLifecycle_DistinctAgentsNoCrossTalk(t *testing.T) {
	bus := newTestDaemon(t)
	c := NewFromBus(bus)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := c.Start(ctx, []string{"agent-a", "agent-b"})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("agent %s: %v", r.AgentID, r.Err)
			continue
		}
		if r.Status.GetAgentId() != r.AgentID {
			t.Errorf("cross-talk: result for %s carries status of %s", r.AgentID, r.Status.GetAgentId())
		}
		if r.Status.GetState() != "RUNNING" {
			t.Errorf("agent %s state = %q, want RUNNING", r.AgentID, r.Status.GetState())
		}
	}
}
