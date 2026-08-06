// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protodelim"

	gapiv1 "github.com/goppydae/gapi/pkg/proto"
)

func boolPtr(b bool) *bool { return &b }

// events decodes the control channel into its "event" values, in order.
func events(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()

	// The control channel carries protodelim-framed AgentControl, not
	// JSON lines (operator decisions 37 and 38). The sequence these
	// tests assert is now a sequence of STATES, because the ADK sets the
	// state rather than emitting an event name the supervisor has to
	// translate - which is the substance of GAPI-DIV-087.
	var out []string
	r := bufio.NewReader(bytes.NewReader(buf.Bytes()))
	for {
		var frame gapiv1.AgentControl
		if err := protodelim.UnmarshalFrom(r, &frame); err != nil {
			if errors.Is(err, io.EOF) {
				return out
			}
			t.Fatalf("control stream is not framed AgentControl: %v", err)
		}
		if frame.GetSchemaVersion() != controlSchemaVersion {
			t.Fatalf("frame carries schema version %d, want %d",
				frame.GetSchemaVersion(), controlSchemaVersion)
		}
		switch ev := frame.GetEvent().(type) {
		case *gapiv1.AgentControl_Status:
			if ev.Status.GetState() == "" {
				t.Fatal("a status frame carries no state")
			}
			out = append(out, ev.Status.GetState())
		case *gapiv1.AgentControl_Heartbeat:
			out = append(out, "HEARTBEAT")
		default:
			t.Fatalf("frame carries no known event arm")
		}
	}
}

// TestRun_StartFlagIsAccepted is the regression for GAPI-DIV-052.
//
// GoAgent.Start execs the binary as '<path> --start'. Every Go template
// and fixture declared only -describe and then called flag.Parse, so the
// flag package rejected --start and the process exited 2 before any agent
// code ran - the compiled-Go supervised path had never executed. The
// assertion is specifically that the exit code is NOT 2, because 2 is
// what flag.Parse's failure produces and 0/1 are outcomes of the agent
// actually running.
func TestRun_StartFlagIsAccepted(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()

	started := make(chan struct{})
	Register(Spec{
		ID:   "probe",
		Type: "service",
		Start: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return nil
		},
	})

	// Run the supervised path in the background and stop it, rather than
	// letting it block the test: the property here is that dispatch
	// reaches Start at all.
	buf := &bytes.Buffer{}
	code := make(chan int, 1)
	go func() { code <- supervised(mustSpec(t), newControlTo(buf, "test-agent")) }()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("Start was never called: dispatch did not reach the agent")
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal self: %v", err)
	}

	select {
	case got := <-code:
		if got == 2 {
			t.Errorf("exit code 2: flag parsing rejected the supervisor's verb")
		}
		if got != 0 {
			t.Errorf("exit code %d, want 0", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("supervised path did not return after SIGTERM")
	}
}

// TestRun_ParseRejectsUnknownFlagWithTwo is the CONTROL for the test
// above. Without it, "not 2" proves nothing: if run returned 0 for
// everything, the --start assertion would pass against a dispatcher that
// had been deleted entirely.
func TestRun_ParseRejectsUnknownFlagWithTwo(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()
	Register(Spec{ID: "probe", Type: "service", Start: func(context.Context) error { return nil }})

	if got := run([]string{"--nonesuch"}); got != 2 {
		t.Errorf("unknown flag exited %d, want 2 - the control for "+
			"TestRun_StartFlagIsAccepted no longer distinguishes anything", got)
	}
}

// TestSupervised_ReachesRunningThenStops pins the event sequence the
// supervisor turns into lifecycle state. streamControl maps "ready" to
// RUNNING and nothing else does, so an agent that never emits it sits in
// PENDING forever - which is what every Go agent did (GAPI-DIV-051).
func TestSupervised_ReachesRunningThenStops(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()

	stopped := false
	Register(Spec{
		ID:         "probe",
		Type:       "service",
		Initialize: func() error { return nil },
		Start: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
		Stop: func() error { stopped = true; return nil },
	})

	buf := &bytes.Buffer{}
	code := make(chan int, 1)
	go func() { code <- supervised(mustSpec(t), newControlTo(buf, "test-agent")) }()

	// Wait past the ready grace, then stop.
	time.Sleep(readyGrace + 250*time.Millisecond)
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal self: %v", err)
	}

	select {
	case got := <-code:
		if got != 0 {
			t.Errorf("exit code %d, want 0", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("supervised path did not return")
	}

	want := []string{statePending, stateRunning, statePending, stateStopped}
	got := events(t, buf)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("event sequence %v, want %v", got, want)
	}
	if !stopped {
		t.Error("Stop was never invoked")
	}
}

// TestSupervised_FailingStartNeverReportsReady is the discriminating half
// of the readiness grace. Emitting "ready" as soon as Start's goroutine
// is spawned would satisfy the sequence test above while announcing a
// dead agent as RUNNING, so the property is that a fast failure produces
// NO ready at all.
func TestSupervised_FailingStartNeverReportsReady(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()
	Register(Spec{
		ID:    "probe",
		Type:  "service",
		Start: func(context.Context) error { return errors.New("boom") },
	})

	buf := &bytes.Buffer{}
	if got := supervised(mustSpec(t), newControlTo(buf, "test-agent")); got != 1 {
		t.Errorf("exit code %d, want 1", got)
	}

	got := events(t, buf)
	for _, ev := range got {
		if ev == stateRunning {
			t.Fatalf("a Start that failed immediately still reported running: %v", got)
		}
	}
	if len(got) == 0 || got[len(got)-1] != stateFailed {
		t.Errorf("state sequence %v, want it to end in %s", got, stateFailed)
	}
}

// TestFire_BareInvocationRunsOnceAndReturns covers the timer path.
// TimerAgent.fireCommand runs a non-Python agent with NO arguments and
// waits for the process to exit before scheduling the next firing, so a
// bare invocation must return rather than enter a supervision loop.
func TestFire_BareInvocationRunsOnceAndReturns(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()

	fires := 0
	Register(Spec{
		ID:    "probe",
		Type:  "timer",
		Start: func(context.Context) error { fires++; return nil },
	})

	done := make(chan int, 1)
	go func() { done <- run(nil) }()

	select {
	case got := <-done:
		if got != 0 {
			t.Errorf("exit code %d, want 0", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a bare invocation did not return: the timer would never fire again")
	}
	if fires != 1 {
		t.Errorf("Start ran %d times, want 1", fires)
	}
}

// mustSpec returns the registered spec, failing the test if Register was
// not called - the tests above all register first, so a nil here is a
// bug in the test rather than in the code.
func mustSpec(t *testing.T) *Spec {
	t.Helper()
	s, err := registeredSpec()
	if err != nil {
		t.Fatalf("registeredSpec: %v", err)
	}
	return s
}

func TestRegister_RejectsASecondAgent(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()
	Register(Spec{ID: "first", Start: func(context.Context) error { return nil }})

	defer func() {
		if recover() == nil {
			t.Error("registering a second agent did not panic")
		}
	}()
	Register(Spec{ID: "second", Start: func(context.Context) error { return nil }})
}

func TestRegister_RejectsAnAgentWithoutStart(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()

	defer func() {
		if recover() == nil {
			t.Error("registering an agent with no Start did not panic")
		}
	}()
	Register(Spec{ID: "noStart"})
}

func TestRun_WithoutRegisterFailsWithoutPanicking(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()

	if got := run([]string{"--describe"}); got != 1 {
		t.Errorf("exit code %d, want 1", got)
	}
}
