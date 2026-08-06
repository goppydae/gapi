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

// A CLEAN SELF-STOP IS NOT A CRASH.
//
// An agent that returns cleanly from Start announces STOPPED on its
// control descriptor and exits. The exit watcher saw only that the
// process was gone and published FAILED over the top of it - MEASURED at
// 20 of 20 runs, message "process exited unexpectedly: <nil>", while both
// a code comment in adk/go/agent/run.go and the PR body claimed the frame
// prevented exactly that.
//
// The defect was in the SEAM. The watcher was correct about what it
// observed and the ADK was correct about what it announced; nothing owned
// their meeting, and the frame was still in the pipe when the watcher
// decided. The fix joins the control reader before classifying, so the
// test does not depend on winning a race - which is why it asserts over
// repeated runs rather than once.

// selfStopScript writes a real STOPPED frame to the control descriptor
// and exits 0.
//
// The frame is marshalled by the REAL encoder and emitted as octal
// escapes: a script hand-writing the wire format would be a second
// implementation of the framing, and a test that passes against a framing
// only it agrees with proves nothing about the supervisor's reader.
func selfStopScript(t *testing.T, agentID string) string {
	t.Helper()

	var buf bytes.Buffer
	msg := &protopkg.AgentControl{
		SchemaVersion: controlSchemaVersion,
		Event: &protopkg.AgentControl_Status{
			Status: &protopkg.LifecycleStatus{
				AgentId: agentID,
				State:   "STOPPED",
				Message: "agent stopped",
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

	script := filepath.Join(t.TempDir(), "selfstop.sh")
	body := "#!/bin/sh\nprintf '" + esc.String() + "' >&3\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return script
}

func TestCleanSelfStopIsNotReportedFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	// Repeated because the defect was a race the frame lost most but not
	// all of the time. One green run was what let it reach review.
	const runs = 10
	for i := 0; i < runs; i++ {
		bus := eventbus.NewInprocBus[*anypb.Any]()
		id := fmt.Sprintf("self-stop-%d", i)

		states := make(chan string, 8)
		if err := bus.Subscribe("system", "", eventbus.TopicAgentLifecycleStatus,
			func(e eventbus.Event[*anypb.Any]) {
				var st protopkg.LifecycleStatus
				if err := e.Payload.UnmarshalTo(&st); err != nil {
					return
				}
				if st.AgentId == id {
					states <- st.State
				}
			}); err != nil {
			t.Fatalf("subscribe: %v", err)
		}

		agent := NewGoAgent(
			id, "service", selfStopScript(t, id),
			nil, nil, nil, nil, "", "", "", nil, bus, NewMockDependencyResolver(),
		)
		if err := agent.Start(context.Background()); err != nil {
			t.Fatalf("run %d: start: %v", i, err)
		}

		// The watcher joins the control reader, so a decision has been
		// taken by the time the process is reaped. Wait for the reap.
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, running := agent.Pid(); !running {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("run %d: process not reaped", i)
			}
			time.Sleep(10 * time.Millisecond)
		}
		// Give a late publish somewhere to land, so the assertion is not
		// simply outrunning the defect.
		time.Sleep(150 * time.Millisecond)

		var seen []string
		for {
			select {
			case s := <-states:
				seen = append(seen, s)
				continue
			default:
			}
			break
		}

		var sawStopped bool
		for _, s := range seen {
			if s == "FAILED" {
				t.Fatalf("run %d: clean self-stop published FAILED (states: %v)", i, seen)
			}
			if s == "STOPPED" {
				sawStopped = true
			}
		}
		if !sawStopped {
			t.Fatalf("run %d: the agent's STOPPED frame never reached the bus (states: %v)", i, seen)
		}
		_ = bus.Close()
	}
}

// TestUnannouncedExitIsStillReportedFailed is the other half, and the one
// that stops the fix from being "never report FAILED".
//
// An agent that dies without announcing anything is exactly the case the
// watcher exists for, and honouring the frame must not have silenced it.
func TestUnannouncedExitIsStillReportedFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	bus := eventbus.NewInprocBus[*anypb.Any]()
	script := filepath.Join(t.TempDir(), "silent.sh")
	// Exits nonzero having said nothing on the control descriptor.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	failed := make(chan string, 4)
	if err := bus.Subscribe("system", "", eventbus.TopicAgentLifecycleStatus,
		func(e eventbus.Event[*anypb.Any]) {
			var st protopkg.LifecycleStatus
			if err := e.Payload.UnmarshalTo(&st); err != nil {
				return
			}
			if st.AgentId == "silent-death" && st.State == "FAILED" {
				failed <- st.Message
			}
		}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	agent := NewGoAgent(
		"silent-death", "service", script,
		nil, nil, nil, nil, "", "", "", nil, bus, NewMockDependencyResolver(),
	)
	if err := agent.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	select {
	case msg := <-failed:
		if msg == "" {
			t.Fatal("FAILED carried no message")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a silent death was not reported FAILED")
	}
}
