// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }

// events decodes the control channel into its "event" values, in order.
func events(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()

	var out []string
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("control line is not JSON: %q (%v)", line, err)
		}
		ev, _ := m["event"].(string)
		out = append(out, ev)
	}
	return out
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
	go func() { code <- supervised(mustSpec(t), newControlTo(buf)) }()

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
	go func() { code <- supervised(mustSpec(t), newControlTo(buf)) }()

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

	want := []string{"starting", "ready", "stopping", "stopped"}
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
	if got := supervised(mustSpec(t), newControlTo(buf)); got != 1 {
		t.Errorf("exit code %d, want 1", got)
	}

	got := events(t, buf)
	for _, ev := range got {
		if ev == "ready" {
			t.Fatalf("a Start that failed immediately still reported ready: %v", got)
		}
	}
	if len(got) == 0 || got[len(got)-1] != "error" {
		t.Errorf("event sequence %v, want it to end in error", got)
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
