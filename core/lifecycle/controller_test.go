// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
	protopkg "github.com/goppydae/gapi/pkg/proto"
)

// fakeRunner is a Runner that spawns nothing. It records the controller's
// state AT THE MOMENT Start is called, which is what makes the ordering
// clause of operator decision 42 assertable: a STARTING observed before
// the exec is a claim about a child that does not exist yet.
type fakeRunner struct {
	mu sync.Mutex

	startErr    error
	starts      atomic.Int32
	stateAtExec string
	c           *Controller

	// blockStart holds Start open, so a second caller arrives while the
	// first is still in flight.
	blockStart chan struct{}
}

func (f *fakeRunner) Start(context.Context) error {
	f.starts.Add(1)
	f.mu.Lock()
	if f.c != nil {
		f.stateAtExec = f.c.State()
	}
	block := f.blockStart
	f.mu.Unlock()
	if block != nil {
		<-block
	}
	return f.startErr
}

func (f *fakeRunner) Stop(context.Context) error   { return nil }
func (f *fakeRunner) Reload(context.Context) error { return nil }
func (f *fakeRunner) Reset()                       {}

func (f *fakeRunner) exec() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stateAtExec
}

// speakingRunner answers the silence question; muteRunner declines to
// implement SpeechReporter at all. The two are distinct on purpose - see
// StartTimeout.SilenceKnown.
type speakingRunner struct {
	fakeRunner
	spoke bool
}

func (s *speakingRunner) HasSpoken() bool { return s.spoke }

// newTestController wires a controller onto an in-process bus with a
// deadline short enough to assert against.
func newTestController(t *testing.T, id string, r Runner) (*Controller, <-chan *protopkg.LifecycleStatus) {
	t.Helper()

	bus := eventbus.NewInprocBus[*anypb.Any]()
	statuses := make(chan *protopkg.LifecycleStatus, 32)
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
				case statuses <- &st:
				default:
				}
			}
		}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	c := NewController(id, "testhost", r, bus, nil)
	c.WaitStart = 200 * time.Millisecond
	return c, statuses
}

// waitForState drains statuses until one carries want, or the budget
// expires. Returns the matching status.
func waitForState(t *testing.T, ch <-chan *protopkg.LifecycleStatus, want string, budget time.Duration) *protopkg.LifecycleStatus {
	t.Helper()
	deadline := time.After(budget)
	for {
		select {
		case st := <-ch:
			if st.GetState() == want {
				return st
			}
		case <-deadline:
			t.Fatalf("no %s status within %s", want, budget)
			return nil
		}
	}
}

// TestSilentAgentIsReportedFailedNamingTheSilence is GAPI-DIV-104's
// remaining exit: a child that is spawned and says nothing must be
// distinguishable from one that is merely slow, and the supervisor must
// say so on the bus rather than only to its caller.
//
// Before this, both produced the identical error - "timeout waiting for
// agent state=running" - and the identical StateError, and nothing at
// all was published, so an operator watching the status topic saw
// STARTING and then silence about the silence.
func TestSilentAgentIsReportedFailedNamingTheSilence(t *testing.T) {
	r := &speakingRunner{spoke: false}
	c, statuses := newTestController(t, "silent-agent", r)
	r.c = c

	err := c.ApplyWithContext(context.Background(), ActionStart)
	if err == nil {
		t.Fatal("expected the start deadline to expire, got nil")
	}

	var st *StartTimeout
	if !errors.As(err, &st) {
		t.Fatalf("expected a *StartTimeout, got %T: %v", err, err)
	}
	if !st.SilenceKnown {
		t.Error("runner implements SpeechReporter, so silence must be known")
	}
	if !st.Silent {
		t.Error("the agent wrote no control frame, so Silent must be true")
	}
	if st.RunID == "" {
		t.Error("the timeout must carry the run id it was waiting on")
	}

	failed := waitForState(t, statuses, "FAILED", 2*time.Second)
	if failed.GetRunId() != st.RunID {
		t.Errorf("FAILED run_id %q does not match the attempt %q", failed.GetRunId(), st.RunID)
	}
	if msg := failed.GetMessage(); msg == "" {
		t.Error("the published FAILED names no cause")
	}
	if c.State() != StateError {
		t.Errorf("state after a failed start: got %q, want %q", c.State(), StateError)
	}
}

// TestSlowAgentIsNotReportedSilent is the other half, and it is what
// stops the fix degrading into "always blame silence". An agent that
// spoke and did not reach RUNNING is a different finding.
func TestSlowAgentIsNotReportedSilent(t *testing.T) {
	r := &speakingRunner{spoke: true}
	c, _ := newTestController(t, "slow-agent", r)
	r.c = c

	err := c.ApplyWithContext(context.Background(), ActionStart)

	var st *StartTimeout
	if !errors.As(err, &st) {
		t.Fatalf("expected a *StartTimeout, got %T: %v", err, err)
	}
	if !st.SilenceKnown {
		t.Fatal("runner implements SpeechReporter, so silence must be known")
	}
	if st.Silent {
		t.Error("the agent spoke, so Silent must be false")
	}
}

// TestUnknowableSilenceIsNotReportedAsQuietFalse: a runner that cannot
// answer the question is a third state. Reporting SilenceKnown=false as
// Silent=false would let an in-process runner - which has no control
// channel at all - read as an agent that spoke.
func TestUnknowableSilenceIsNotReportedAsQuietFalse(t *testing.T) {
	r := &fakeRunner{}
	c, _ := newTestController(t, "opaque-agent", r)
	r.c = c

	err := c.ApplyWithContext(context.Background(), ActionStart)

	var st *StartTimeout
	if !errors.As(err, &st) {
		t.Fatalf("expected a *StartTimeout, got %T: %v", err, err)
	}
	if st.SilenceKnown {
		t.Error("runner does not implement SpeechReporter, so silence is not knowable")
	}
}

// TestStartingIsNotClaimedBeforeTheExec is operator decision 42 stated as
// a test. STARTING used to be set before the dependency walk, so the
// supervisor announced a state about a child that had not been spawned.
func TestStartingIsNotClaimedBeforeTheExec(t *testing.T) {
	r := &speakingRunner{spoke: true}
	c, _ := newTestController(t, "ordering-agent", r)
	r.c = c

	_ = c.ApplyWithContext(context.Background(), ActionStart)

	if got := r.exec(); got == StateStarting {
		t.Errorf("state at exec was %q: STARTING was claimed before the child existed", got)
	}
}

// TestConcurrentStartSpawnsOnce guards what moving the transition took
// away. STARTING being set early was doubling as the in-flight marker,
// and the switch at the top of ActionStart read it to make a second
// caller a no-op. With the transition after the exec, nothing but an
// explicit marker prevents two callers from both walking the dependency
// tree and both spawning.
func TestConcurrentStartSpawnsOnce(t *testing.T) {
	r := &speakingRunner{spoke: true}
	r.blockStart = make(chan struct{})
	c, _ := newTestController(t, "concurrent-agent", r)
	r.c = c

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.ApplyWithContext(context.Background(), ActionStart)
		}()
	}

	// Let every caller reach the guard while the first is held inside
	// Start. Without the marker they arrive at a startable state and
	// proceed.
	time.Sleep(300 * time.Millisecond)
	close(r.blockStart)
	wg.Wait()

	if got := r.starts.Load(); got != 1 {
		t.Errorf("Start called %d times for concurrent starts, want 1", got)
	}
}
