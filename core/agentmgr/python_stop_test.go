// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
	protopkg "github.com/goppydae/gapi/pkg/proto"
)

// graceStop mirrors core/lifecycle's Controller.GraceStop default. It is
// restated rather than imported because the point of the test is the
// relationship between the supervisor's budget and the agent's exit, and
// a test that moved with the constant would stop asserting it.
const graceStop = 3 * time.Second

// TestPythonAgentStopsWithinGrace is GAPI-DIV-108's gate.
//
// A GRACEFUL STOP WAS UNREACHABLE BY CONSTRUCTION. runner.py's SIGTERM
// handler slept 5 seconds "to give time for events to flush (especially
// over stdout/QUIC)" - a window operator decision 37 deleted when the
// control channel became an inherited descriptor. Five seconds cannot
// fit inside a three-second grace, so EVERY Python agent stop spent the
// whole budget, was SIGKILLed, returned context.DeadlineExceeded, and
// had its own clean STOPPED overwritten by the supervisor's
// "killed after timeout".
//
// MEASURED before the fix: Stop took 3.002683347s and returned context
// deadline exceeded.
//
// The assertions are separate on purpose. Timing alone would pass if
// Stop returned early with an error, and the message alone would pass if
// the agent were killed fast.
func TestPythonAgentStopsWithinGrace(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	const id = "python-stop-grace"
	bus := eventbus.NewInprocBus[*anypb.Any]()

	messages := make(chan string, 8)
	if err := bus.Subscribe("system", "", eventbus.TopicAgentLifecycleStatus,
		func(e eventbus.Event[*anypb.Any]) {
			if e.Payload == nil {
				return
			}
			var st protopkg.LifecycleStatus
			if err := e.Payload.UnmarshalTo(&st); err != nil {
				return
			}
			if st.GetAgentId() == id && st.GetState() == "STOPPED" {
				select {
				case messages <- st.GetMessage():
				default:
				}
			}
		}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	agent := NewPythonAgent(
		id, "service",
		"../../test/adk/fixtures/simple.py.service",
		"../../adk/python/agent/runner.py",
		nil, nil, nil, nil, "", "", "", nil, bus, NewMockDependencyResolver(), false,
	)
	agent.SetRunID("grace-run")

	if err := agent.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Wait for the agent to be genuinely up, so the stop under test is a
	// stop of a running agent rather than a race with its startup.
	deadline := time.Now().Add(30 * time.Second)
	for !agent.HasSpoken() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !agent.HasSpoken() {
		t.Fatal("agent never wrote a control frame; cannot test its stop")
	}

	ctx, cancel := context.WithTimeout(context.Background(), graceStop)
	defer cancel()

	begin := time.Now()
	err := agent.Stop(ctx)
	elapsed := time.Since(begin)

	if err != nil {
		t.Errorf("Stop returned %v; a graceful stop must not consume its deadline", err)
	}
	// Generous against the measured floor: the agent's own stop path
	// sleeps 0.25s and joins its start thread, so a healthy stop is well
	// under a second. Failing at half the grace still catches a
	// regression that reintroduces any multi-second wait.
	if budget := graceStop / 2; elapsed > budget {
		t.Errorf("Stop took %s, over half the %s grace: something is waiting that should not be",
			elapsed, graceStop)
	}

	select {
	case msg := <-messages:
		if msg == "killed after timeout" {
			t.Errorf("the agent was killed rather than stopped: STOPPED message %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Error("no STOPPED status was published for a stopped agent")
	}
}
