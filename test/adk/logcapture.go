// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk

import (
	"fmt"
	"strings"
	"sync"
)

// logRecorder keeps a copy of everything written through it.
//
// BOTH SIDES OF THE JOIN WERE ALREADY BEING THROWN AWAY, which is the
// whole reason GAPI-DIV-125 was diagnosed by hand-grepping CI job logs.
// The daemon's output went straight to os.Stdout and was never readable
// in-process; every gapictl invocation was captured with CombinedOutput
// and discarded on success. The information existed at both ends and
// nothing retained it, so no test could assert a request had arrived
// even though both sides print the event id.
//
// Concurrency is not theoretical here: gapid's stdout and stderr are two
// writers on the same recorder, and its logging comes from whatever
// goroutine produced the event.
type logRecorder struct {
	mu sync.Mutex
	b  strings.Builder
}

func (r *logRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.b.Write(p)
}

// record appends text that was captured rather than streamed - the
// gapictl invocations, which are collected with CombinedOutput.
func (r *logRecorder) record(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.b.WriteString(s)
	r.b.WriteString("\n")
}

func (r *logRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.b.String()
}

// checkDelivery is the teardown half of GAPI-DIV-125's gate: every
// control request a client published during this harness's life must
// have arrived at the daemon.
//
// IT RUNS FROM Stop, DELIBERATELY, AND NOT AS A METHOD TESTS MUST
// REMEMBER TO CALL. Every existing caller already writes
// `defer func() { if err := h.Stop(); err != nil { t.Errorf(...) } }()`,
// so returning the audit's error from Stop wires the gate into all five
// call sites with no opt-in and nothing to forget. A checkDelivery that
// tests had to invoke themselves would be a mechanism built and never
// wired, which is the defect shape this repository keeps producing.
//
// TWO CASES ARE DELIBERATELY NOT FAILURES, and both would otherwise
// produce red tests that say nothing true:
//
//   - The harness never became READY. Stop is called from Start's own
//     failure path, and a daemon that never answered a ping has no
//     delivery record to audit. Reporting "zero publishes" there would
//     bury the real startup error under a derived one.
//   - The harness issued NO client calls. A test that starts a daemon
//     and stops it has published nothing, and that is a fact this
//     harness knows rather than a suspicious silence.
//
// The audit's own zero-publish failure still guards what matters: calls
// WERE made and no publish line was parsed, which means the capture or
// the log format changed and the gate has stopped seeing its subject.
// WHAT IT INSPECTED IS PRINTED, for the reason readyAfter is: a gate
// that skipped and a gate that passed are indistinguishable from a green
// test, and this harness has already shipped a readiness stall that
// passed for months because the number was recorded and never shown. The
// line makes "the audit ran and matched 3 of 3" a fact in the CI log
// rather than something a reader has to take on faith.
func (h *TestHarness) checkDelivery() error {
	if !h.ready || h.clientCalls == 0 {
		fmt.Printf("harness: delivery audit skipped (ready=%t, client calls=%d)\n", h.ready, h.clientCalls)
		return nil
	}

	audit := auditDelivery(h.clientLog.String(), h.daemonLog.String())
	if err := audit.check(); err != nil {
		return err
	}
	fmt.Printf("harness: delivery audit matched %d of %d client requests across %d arrivals\n",
		len(audit.Published), len(audit.Published), len(audit.Received))
	return nil
}
