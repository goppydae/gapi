// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
	protopkg "github.com/goppydae/gapi/pkg/proto"
)

// silentScript is spawned, holds its control descriptor open, and never
// writes to it. It is what a hung agent and a mis-built ADK look like
// from the supervisor's side, and until GAPI-DIV-104 the two were
// indistinguishable from a slow start.
func silentScript(t *testing.T) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "silent.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return script
}

// speakingScript writes one RUNNING frame and then stays alive. The
// frame is produced by the REAL encoder for the reason self_stop_test.go
// gives: a hand-written framing is a second implementation, and a test
// that agrees only with itself proves nothing about the reader.
func speakingScript(t *testing.T, agentID string) string {
	t.Helper()

	var buf bytes.Buffer
	msg := &protopkg.AgentControl{
		SchemaVersion: controlSchemaVersion,
		Event: &protopkg.AgentControl_Status{
			Status: &protopkg.LifecycleStatus{
				AgentId: agentID,
				State:   "RUNNING",
				Message: "up",
			},
		},
	}
	if _, err := protodelim.MarshalTo(&buf, msg); err != nil {
		t.Fatalf("marshal frame: %v", err)
	}

	var esc strings.Builder
	for _, b := range buf.Bytes() {
		fmt.Fprintf(&esc, "\\%03o", b)
	}

	script := filepath.Join(t.TempDir(), "speaking.sh")
	body := "#!/bin/sh\nprintf '" + esc.String() + "' >&3\nsleep 30\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return script
}

func statusChannel(t *testing.T, bus *eventbus.EventBus[*anypb.Any], id string) <-chan *protopkg.LifecycleStatus {
	t.Helper()
	ch := make(chan *protopkg.LifecycleStatus, 16)
	if err := bus.Subscribe("system", "", eventbus.TopicAgentLifecycleStatus,
		func(e eventbus.Event[*anypb.Any]) {
			if e.Payload == nil {
				return
			}
			var st protopkg.LifecycleStatus
			if err := e.Payload.UnmarshalTo(&st); err != nil {
				return
			}
			if st.GetAgentId() == id {
				select {
				case ch <- &st:
				default:
				}
			}
		}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	return ch
}

// TestStartingIsPublishedAtExecWithRunID is the observation half of
// GAPI-DIV-104: spawning a child is something the supervisor OBSERVES,
// and it says so with the run id that identifies the attempt.
//
// The agent here never speaks, which is the point - the STARTING must
// come from the supervisor watching itself exec, not from anything the
// child announces. Before this, an agent that never spoke produced no
// supervisor-side event at all.
func TestStartingIsPublishedAtExecWithRunID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	const id = "starting-at-exec"
	bus := eventbus.NewInprocBus[*anypb.Any]()
	statuses := statusChannel(t, bus, id)

	agent := NewGoAgent(
		id, "service", silentScript(t),
		nil, nil, nil, nil, "", "", "", nil, bus, NewMockDependencyResolver(),
	)
	agent.SetRunID("run-abc")

	if err := agent.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = agent.Stop(ctx)
	}()

	select {
	case st := <-statuses:
		if st.GetState() != "STARTING" {
			t.Errorf("first status: got %q, want STARTING", st.GetState())
		}
		if st.GetRunId() != "run-abc" {
			t.Errorf("STARTING run_id: got %q, want %q", st.GetRunId(), "run-abc")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no STARTING published for a spawned child")
	}
}

// TestSilenceIsObservable is what the controller's start deadline reads
// to tell a silent agent from a slow one. It asserts the two directions
// separately, because a HasSpoken that is always false would satisfy the
// silent case alone.
func TestSilenceIsObservable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	t.Run("silent child never reports speech", func(t *testing.T) {
		bus := eventbus.NewInprocBus[*anypb.Any]()
		agent := NewGoAgent(
			"silent", "service", silentScript(t),
			nil, nil, nil, nil, "", "", "", nil, bus, NewMockDependencyResolver(),
		)
		if err := agent.Start(context.Background()); err != nil {
			t.Fatalf("start: %v", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = agent.Stop(ctx)
		}()

		// Long enough that a frame in flight would have landed.
		time.Sleep(500 * time.Millisecond)

		if agent.HasSpoken() {
			t.Error("a child that wrote nothing is reported as having spoken")
		}
		if d := agent.FirstFrameLatency(); d != 0 {
			t.Errorf("first-frame latency for a silent child: got %s, want 0", d)
		}
	})

	t.Run("speaking child reports speech and a latency", func(t *testing.T) {
		const id = "speaking"
		bus := eventbus.NewInprocBus[*anypb.Any]()
		agent := NewGoAgent(
			id, "service", speakingScript(t, id),
			nil, nil, nil, nil, "", "", "", nil, bus, NewMockDependencyResolver(),
		)
		if err := agent.Start(context.Background()); err != nil {
			t.Fatalf("start: %v", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = agent.Stop(ctx)
		}()

		deadline := time.Now().Add(5 * time.Second)
		for !agent.HasSpoken() && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}

		if !agent.HasSpoken() {
			t.Fatal("a child that wrote a frame is reported as silent")
		}
		d := agent.FirstFrameLatency()
		if d <= 0 {
			t.Errorf("first-frame latency: got %s, want a positive interval", d)
		}
		// The interval is exec to first frame. A value at or above the
		// script's own sleep would mean it is measuring the wrong span.
		if d > 30*time.Second {
			t.Errorf("first-frame latency %s exceeds the child's whole lifetime", d)
		}
		t.Logf("MEASURED exec-to-first-frame: %s", d)
	})
}
