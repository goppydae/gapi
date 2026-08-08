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
	"strings"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/budget"
)

// latentRunner speaks, and can say how long it took to do so. It is
// speakingRunner plus FirstFrameReporter, kept separate so a test can
// still exercise a runner that answers one question and not the other.
type latentRunner struct {
	speakingRunner
	latency time.Duration
}

func (l *latentRunner) FirstFrameLatency() time.Duration { return l.latency }

// TestDeclaredBudgetIsHonouredInPlaceOfTheDefault is task 4's gate. An
// agent that declares LESS than the 10s WaitStart used to give everyone
// must fail at what it declared, which is the whole point of the field:
// a value in the descriptor beats a constant in the supervisor.
//
// The assertion is on the ELAPSED time, not on the error text. A test
// that only checked the error would pass against a controller that
// ignored the declaration entirely and timed out on the default.
func TestDeclaredBudgetIsHonouredInPlaceOfTheDefault(t *testing.T) {
	const declared = 150 * time.Millisecond

	r := &speakingRunner{spoke: true}
	c, _ := newTestController(t, "declares-short", r)
	r.c = c
	// What discovery does with a descriptor that declared a budget.
	c.ReadinessBudget = declared
	c.SilenceBudget = budget.SilenceBudget("go")

	start := time.Now()
	err := c.ApplyWithContext(context.Background(), ActionStart)
	elapsed := time.Since(start)

	var st *StartTimeout
	if !errors.As(err, &st) {
		t.Fatalf("expected a *StartTimeout, got %T: %v", err, err)
	}
	if st.Waited != declared {
		t.Errorf("timed out after a budget of %s, want the declared %s", st.Waited, declared)
	}
	if elapsed >= 10*time.Second {
		t.Fatalf("start took %s: the declared budget was ignored and the old 10s default was used", elapsed)
	}
	// Generous upper bound - this asserts which deadline fired, not the
	// scheduler's precision.
	if elapsed > declared+2*time.Second {
		t.Errorf("start took %s for a declared budget of %s", elapsed, declared)
	}
	if elapsed < declared {
		t.Errorf("start took %s, less than the declared budget of %s", elapsed, declared)
	}
}

// TestSilentAgentFailsAtTheSilenceBudgetNotTheReadinessOne is task 5's
// gate and the exit's own closing condition.
//
// BOTH ASSERTIONS OR NEITHER. The elapsed time alone passes against a
// controller that simply has a short readiness budget; the Silent flag
// alone passes against the old code, which set it correctly after
// waiting out the full readiness budget. Together they say the silence
// deadline is what fired.
func TestSilentAgentFailsAtTheSilenceBudgetNotTheReadinessOne(t *testing.T) {
	const silence = 80 * time.Millisecond
	const readiness = 5 * time.Second

	r := &speakingRunner{spoke: false}
	c, _ := newTestController(t, "silent-agent", r)
	r.c = c
	c.SilenceBudget = silence
	c.ReadinessBudget = readiness

	start := time.Now()
	err := c.ApplyWithContext(context.Background(), ActionStart)
	elapsed := time.Since(start)

	var st *StartTimeout
	if !errors.As(err, &st) {
		t.Fatalf("expected a *StartTimeout, got %T: %v", err, err)
	}
	if !st.Silent {
		t.Error("the agent wrote no control frame, so Silent must be true")
	}
	if st.Waited != silence {
		t.Errorf("the timeout reports waiting %s, want the silence budget %s", st.Waited, silence)
	}
	// The decisive one: the readiness budget is 62x the silence budget,
	// so a run that waited it out cannot be mistaken for one that did
	// not, even on a loaded machine.
	if elapsed >= readiness {
		t.Fatalf("a silent agent took %s to fail, waiting out the readiness budget %s: the silence budget is not wired", elapsed, readiness)
	}
	if elapsed < silence {
		t.Errorf("a silent agent failed after %s, sooner than the silence budget %s", elapsed, silence)
	}
}

// TestSpeakingAgentSurvivesTheSilenceDeadline is what stops task 5
// degrading into "fail everything early". A child that spoke has
// answered the silence question, and the readiness budget - which
// covers its own start() - must own the rest of the wait.
func TestSpeakingAgentSurvivesTheSilenceDeadline(t *testing.T) {
	const silence = 40 * time.Millisecond
	const readiness = 400 * time.Millisecond

	r := &speakingRunner{spoke: true}
	c, _ := newTestController(t, "slow-but-speaking", r)
	r.c = c
	c.SilenceBudget = silence
	c.ReadinessBudget = readiness

	start := time.Now()
	err := c.ApplyWithContext(context.Background(), ActionStart)
	elapsed := time.Since(start)

	var st *StartTimeout
	if !errors.As(err, &st) {
		t.Fatalf("expected a *StartTimeout, got %T: %v", err, err)
	}
	if st.Silent {
		t.Error("the agent spoke, so Silent must be false")
	}
	if st.Waited != readiness {
		t.Errorf("the timeout reports waiting %s, want the readiness budget %s", st.Waited, readiness)
	}
	if elapsed < readiness {
		t.Errorf("a speaking agent failed after %s, before its readiness budget %s: the silence deadline was not retired", elapsed, readiness)
	}
}

// TestUnknowableSilenceGetsNoSilenceDeadline: a runner with no control
// channel cannot be judged silent, and must not be failed early for
// being unable to answer. SilenceKnown=false is a third state, not a
// quiet true.
func TestUnknowableSilenceGetsNoSilenceDeadline(t *testing.T) {
	const silence = 40 * time.Millisecond
	const readiness = 300 * time.Millisecond

	r := &fakeRunner{}
	c, _ := newTestController(t, "opaque-agent", r)
	r.c = c
	c.SilenceBudget = silence
	c.ReadinessBudget = readiness

	start := time.Now()
	err := c.ApplyWithContext(context.Background(), ActionStart)
	elapsed := time.Since(start)

	var st *StartTimeout
	if !errors.As(err, &st) {
		t.Fatalf("expected a *StartTimeout, got %T: %v", err, err)
	}
	if st.SilenceKnown {
		t.Error("runner does not implement SpeechReporter, so silence is not knowable")
	}
	if elapsed < readiness {
		t.Errorf("an opaque runner failed after %s, before its readiness budget %s", elapsed, readiness)
	}
}

// TestFirstFrameLatencyReachesTheTimeout is the instrument getting a
// production consumer, which was false for the entire life of
// GAPI-DIV-107. Two agents that both spoke and both missed the deadline
// are the same finding until this number separates them.
func TestFirstFrameLatencyReachesTheTimeout(t *testing.T) {
	const latency = 7 * time.Millisecond

	r := &latentRunner{latency: latency}
	r.spoke = true
	c, _ := newTestController(t, "latent-agent", r)
	r.c = c
	c.ReadinessBudget = 150 * time.Millisecond

	err := c.ApplyWithContext(context.Background(), ActionStart)

	var st *StartTimeout
	if !errors.As(err, &st) {
		t.Fatalf("expected a *StartTimeout, got %T: %v", err, err)
	}
	if st.FirstFrame != latency {
		t.Errorf("StartTimeout.FirstFrame = %s, want the measured %s: FirstFrameLatency still has no consumer", st.FirstFrame, latency)
	}
	// And an operator reading the line rather than the struct sees it.
	if msg := st.Error(); !strings.Contains(msg, latency.String()) {
		t.Errorf("error %q does not report when the agent spoke", msg)
	}
}

// TestSilentTimeoutReportsNoFirstFrame keeps the two questions apart. A
// child that said nothing has no latency, and reporting 0 as a
// measurement would be a number that looks measured and is not.
func TestSilentTimeoutReportsNoFirstFrame(t *testing.T) {
	r := &latentRunner{latency: 0}
	r.spoke = false
	c, _ := newTestController(t, "mute-agent", r)
	r.c = c

	err := c.ApplyWithContext(context.Background(), ActionStart)

	var st *StartTimeout
	if !errors.As(err, &st) {
		t.Fatalf("expected a *StartTimeout, got %T: %v", err, err)
	}
	if !st.Silent {
		t.Fatal("the agent wrote no control frame, so Silent must be true")
	}
	if st.FirstFrame != 0 {
		t.Errorf("a silent agent reported a first-frame latency of %s", st.FirstFrame)
	}
}

// TestSpawnBudgetIsNotTheDeclaredBudget is the second job found at
// controller.go's first WaitStart call site, held as a property. A
// descriptor declaring a short readiness budget must not shorten the
// context that bounds fork/exec: the runner would fail with a raw
// context deadline instead of a StartTimeout, which is a different
// error shape produced by a field move nobody asked for.
func TestSpawnBudgetIsNotTheDeclaredBudget(t *testing.T) {
	r := &speakingRunner{spoke: true}
	c, _ := newTestController(t, "declares-short", r)
	r.c = c
	c.ReadinessBudget = 50 * time.Millisecond

	if c.SpawnBudget == c.ReadinessBudget {
		t.Fatal("the spawn budget followed the declared readiness budget; they bound different phenomena")
	}

	err := c.ApplyWithContext(context.Background(), ActionStart)
	var st *StartTimeout
	if !errors.As(err, &st) {
		t.Fatalf("expected a *StartTimeout, got %T: %v", err, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Error("the start failed with a raw context deadline: the declared budget reached the spawn context")
	}
}
