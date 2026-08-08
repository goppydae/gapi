// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package lifecycle

import (
	"fmt"
	"strings"
	"time"
)

// The waits a start and a reload are made of, and the error they
// produce. Split out of controller.go when GAPI-DIV-107 gave the start
// two deadlines instead of one: the file was at 476 lines and the
// 500-line rule is not something to spend on a wait loop.

func (c *Controller) awaitTarget(d time.Duration, want string) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	want = strings.ToLower(strings.TrimSpace(want))
	for {
		select {
		case got := <-c.stateCh:
			if strings.EqualFold(got.state, want) {
				return nil
			}
		case <-timer.C:
			return fmt.Errorf("timeout waiting for agent state=%s", want)
		}
	}
}

// awaitRunningWithRunIDSince waits for this run to reach RUNNING under
// TWO deadlines, which is the whole of GAPI-DIV-107 at this seam.
//
// readiness bounds the interval that contains the agent's own start() -
// unbounded user code, so the budget is generous and per-agent.
//
// silence bounds only what was MEASURED: exec to first control frame,
// Go p95 1.248ms and Python p95 37.237ms. A child that has written
// nothing by then is not slow, it is hung before its first report or
// built against an ADK that never opened the descriptor - and waiting
// out the readiness budget to say so throws away evidence that arrived
// in under 40ms. That was GAPI-DIV-104's complaint: the discriminator
// existed and was acted on at 10s.
//
// THE SILENCE DEADLINE IS NOT A SECOND CHANCE TO FAIL. It fires once;
// if the child HAS spoken by then the deadline is retired and the
// readiness budget owns the rest of the wait. A runner that cannot
// answer the question at all - an in-process one, with no control
// channel - gets no silence deadline, which is distinct from getting
// one and passing it.
func (c *Controller) awaitRunningWithRunIDSince(readiness, silence time.Duration, wantRunID string, since time.Time) error {
	timer := time.NewTimer(readiness)
	defer timer.Stop()

	var silenceC <-chan time.Time
	if _, ok := c.runner.(SpeechReporter); ok && silence > 0 && silence < readiness {
		st := time.NewTimer(silence)
		defer st.Stop()
		silenceC = st.C
	}

	for {
		select {
		case ev := <-c.stateCh:
			if ev.when.Before(since) {
				continue
			}
			if ev.state == "running" && ev.runID == wantRunID {
				return nil
			}
		case <-silenceC:
			if sr, ok := c.runner.(SpeechReporter); ok && !sr.HasSpoken() {
				return c.startTimeout(silence, wantRunID)
			}
			// It spoke. The question this deadline asks has been
			// answered and will not become false again for this run.
			silenceC = nil
		case <-timer.C:
			return c.startTimeout(readiness, wantRunID)
		}
	}
}

// StartTimeout is the start deadline expiring, as data rather than as a
// sentence (GAPI-DIV-104).
//
// Silent is the discriminator the old bare timeout could not express: a
// child that has written nothing is hung before its first report or was
// built against an ADK that never opened the control descriptor, while
// one that has spoken and not reached RUNNING is merely slow. Those want
// different operator responses, and a caller must be able to branch on
// the difference without matching on a message.
//
// SilenceKnown is separate from Silent because "the runner cannot answer
// this question" is a third state, not a quiet false. An in-process
// runner has no control channel at all.
//
// FirstFrame is how long exec-to-first-speech actually took, when the
// runner can say (GAPI-DIV-107). It is the difference between an agent
// that spoke at 1ms and then hung inside its own start(), and one whose
// ADK took most of the budget to come up at all - two failures with the
// same Silent=false and completely different answers.
type StartTimeout struct {
	AgentID      string
	RunID        string
	Waited       time.Duration
	Silent       bool
	SilenceKnown bool
	FirstFrame   time.Duration
}

func (e *StartTimeout) Error() string {
	switch {
	case e.SilenceKnown && e.Silent:
		return fmt.Sprintf(
			"agent %s was spawned and wrote no control frame within %s (run_id=%s): hung before its first report, or its ADK never opened the control descriptor",
			e.AgentID, e.Waited, e.RunID)
	case e.SilenceKnown && e.FirstFrame > 0:
		return fmt.Sprintf(
			"agent %s spoke after %s but did not reach running within %s (run_id=%s)",
			e.AgentID, e.FirstFrame, e.Waited, e.RunID)
	case e.SilenceKnown:
		return fmt.Sprintf(
			"agent %s spoke but did not reach running within %s (run_id=%s)",
			e.AgentID, e.Waited, e.RunID)
	default:
		return fmt.Sprintf(
			"timeout waiting for agent %s state=running after %s (run_id=%s)",
			e.AgentID, e.Waited, e.RunID)
	}
}

func (c *Controller) startTimeout(d time.Duration, runID string) error {
	e := &StartTimeout{AgentID: c.id, RunID: runID, Waited: d}
	if sr, ok := c.runner.(SpeechReporter); ok {
		e.SilenceKnown = true
		e.Silent = !sr.HasSpoken()
	}
	// THE INSTRUMENT GETS A CONSUMER. FirstFrameLatency was built for
	// GAPI-DIV-104's measurement and read by nothing outside a test for
	// the whole life of GAPI-DIV-107; a value produced and never read
	// can be wrong indefinitely.
	if fr, ok := c.runner.(FirstFrameReporter); ok {
		e.FirstFrame = fr.FirstFrameLatency()
	}
	return e
}
