// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

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
		reply := eventbus.NewEvent("system", "", "pong", "test-daemon", payload)
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
		reply := eventbus.NewEvent("system", "", "agents.reply", "test-daemon", payload)
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
		status := eventbus.NewEvent("system", "", "agent/lifecycle.status", "test-daemon", payload)
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

// TestPing_SurvivesALateSubscriber is GAPI-DIV-120's client-side gate.
//
// The defect: Ping published ONCE, fire-and-forget. A daemon that is
// listening but has not yet subscribed - the whole of agent bring-up -
// silently dropped the probe, and the client then waited out its ENTIRE
// deadline for a pong nobody would send. Measured against the real
// daemon before the fix: 0.21s to first pong with no agents, 30.21s with
// the ADK fixture set, which is exactly gapictl's 30s timeout rather
// than anything about the agents.
//
// The window here is deliberately WIDER than one retry interval, so this
// test cannot pass by luck of the first publish landing late. Reverting
// the retry loop in Ping makes it fail by timing out.
func TestPing_SurvivesALateSubscriber(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	t.Cleanup(func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	})

	// No ping subscriber yet: this is the window.
	const window = 3 * requestRetryInterval

	subscribed := make(chan struct{})
	go func() {
		time.Sleep(window)
		if err := bus.Subscribe("system", "", "ping", func(e eventbus.Event[*anypb.Any]) {
			payload, err := anypb.New(&protopkg.PingStatus{Status: "pong"})
			if err != nil {
				return
			}
			reply := eventbus.NewEvent("system", "", "pong", "test-daemon", payload)
			reply.ID = e.ID
			_ = bus.Publish(reply)
		}); err != nil {
			t.Errorf("late subscribe: %v", err)
		}
		close(subscribed)
	}()

	// Comfortably above the window and far below the 30s production
	// deadline, so a pass means the retry worked rather than that the
	// deadline was generous.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	status, err := NewFromBus(bus).Ping(ctx)
	elapsed := time.Since(start)

	<-subscribed
	if err != nil {
		t.Fatalf("Ping across a late-subscriber window failed: %v (waited %s)", err, elapsed)
	}
	if status != "pong" {
		t.Errorf("Ping status = %q, want pong", status)
	}
	if elapsed < window {
		t.Errorf("Ping returned in %s, before the %s window elapsed; "+
			"the test is not exercising a late subscriber", elapsed, window)
	}
}

// TestAgentStatus_SurvivesALateSubscriber is GAPI-DIV-122's gate.
//
// The same defect as TestPing_SurvivesALateSubscriber, in the method the
// -120 fix did not reach. The daemon subscribes agents/ only after
// setupAgents returns, so a status request issued during bring-up has no
// subscriber and is dropped, and a single publish then waits out the
// whole deadline. Measured in a clean NixOS guest before the fix: the
// request went out at 17:06:05 and the client gave up at 17:06:35 -
// exactly its 30s deadline, one call, no load. That is what had been
// reddening the VM check on main for five consecutive commits.
//
// Retrying is safe HERE and not everywhere: a status read is idempotent.
// The lifecycle verbs are excluded deliberately, because re-publishing a
// mutating action could execute it twice. GAPI-DIV-122 names each
// exclusion.
//
// As with the ping test, the window is wider than one retry interval and
// the elapsed time is asserted, so this cannot pass by luck or without
// exercising a late subscriber.
func TestAgentStatus_SurvivesALateSubscriber(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	t.Cleanup(func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	})

	const window = 3 * requestRetryInterval

	subscribed := make(chan struct{})
	go func() {
		time.Sleep(window)
		if err := bus.Subscribe("system", "", "agents/", func(e eventbus.Event[*anypb.Any]) {
			payload, err := anypb.New(&protopkg.AgentStatusResponse{
				Agents: []*protopkg.AgentStatus{{Id: "agent-1"}},
			})
			if err != nil {
				return
			}
			reply := eventbus.NewEvent("system", "", "agents.reply", "test-daemon", payload)
			reply.ID = e.ID
			_ = bus.Publish(reply)
		}); err != nil {
			t.Errorf("late subscribe: %v", err)
		}
		close(subscribed)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	agents, err := NewFromBus(bus).AgentStatus(ctx)
	elapsed := time.Since(start)

	<-subscribed
	if err != nil {
		t.Fatalf("AgentStatus across a late-subscriber window failed: %v (waited %s)", err, elapsed)
	}
	if len(agents) != 1 || agents[0].Id != "agent-1" {
		t.Errorf("AgentStatus = %v, want one agent-1", agents)
	}
	if elapsed < window {
		t.Errorf("AgentStatus returned in %s, before the %s window elapsed; "+
			"the test is not exercising a late subscriber", elapsed, window)
	}
}
