// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk_test

import (
	"bufio"
	"os"
	"strconv"
	"testing"

	"google.golang.org/protobuf/encoding/protodelim"

	adk "github.com/goppydae/gapi/adk/go"
	gapiv1 "github.com/goppydae/gapi/pkg/proto"
)

// TestSendEventWritesATypedFrame is what GAPI-DIV-100's gate became.
//
// That gate asserted a subscriber RECEIVED what SendEvent published,
// over a real QUIC transport into a real EventBus, because SendEvent
// published to the bus itself and its events were being dropped at the
// key lookup. Operator decision 37 moved the channel to an inherited
// descriptor with the SUPERVISOR publishing, so there is no bus here to
// assert against any more - and the routing half of -100 is asserted
// where routing now happens, in core/transport.
//
// What survives, and is asserted here, is -087's half: the call is TYPED
// and what it writes is a frame, not a JSON string that something has to
// parse back.
func TestSendEventWritesATypedFrame(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	// Point the ADK at the write end's ACTUAL descriptor. Dup'ing onto
	// a fixed number here would clobber whatever the test binary already
	// has open there - which it did, and go test lost its own log to a
	// broken pipe.
	t.Setenv("ADK_CONTROL_FD", strconv.Itoa(int(w.Fd())))
	t.Setenv("ADK_RUN_ID", "run-under-test")

	adk.SendEvent("probe", "running", "agent reported ready")
	_ = w.Close()

	var frame gapiv1.AgentControl
	if err := protodelim.UnmarshalFrom(bufio.NewReader(r), &frame); err != nil {
		t.Fatalf("control channel carries no readable frame: %v", err)
	}

	st := frame.GetStatus()
	if st == nil {
		t.Fatal("frame carries no status arm")
	}
	if st.GetState() != "RUNNING" {
		t.Errorf("state = %q, want RUNNING", st.GetState())
	}
	if st.GetAgentId() != "probe" {
		t.Errorf("agent id = %q, want probe", st.GetAgentId())
	}
	if st.GetRunId() != "run-under-test" {
		t.Errorf("run id = %q, want run-under-test", st.GetRunId())
	}
}

// TestQuotedAgentIDProducesAWellFormedFrame is GAPI-DIV-087's other
// gate, and it fails on the code this replaced.
//
// StartHeartbeat used to build its payload with fmt.Sprintf and NO
// escaping, so an agent id containing a quote emitted malformed JSON
// every five seconds - forever, unnoticed, because nothing downstream
// asserted the frame was readable. A typed message cannot be malformed
// by its own contents: the defect class disappears rather than being
// fixed case by case.
func TestQuotedAgentIDProducesAWellFormedFrame(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	t.Setenv("ADK_CONTROL_FD", strconv.Itoa(int(w.Fd())))

	const hostile = `weird"id\with'quotes`
	adk.SendEvent(hostile, "running", `message with "quotes" too`)
	_ = w.Close()

	var frame gapiv1.AgentControl
	if err := protodelim.UnmarshalFrom(bufio.NewReader(r), &frame); err != nil {
		t.Fatalf("a quoted agent id produced an unreadable frame: %v", err)
	}
	if got := frame.GetStatus().GetAgentId(); got != hostile {
		t.Errorf("agent id = %q, want %q", got, hostile)
	}
}
