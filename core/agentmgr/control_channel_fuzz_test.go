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
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protodelim"

	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/schemaskew"
	gapiv1 "github.com/goppydae/gapi/pkg/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// readControl is the kernel's most exposed parser: it eats whatever a
// supervised process writes to its control descriptor and decodes each
// length-delimited frame as an AgentControl message.
//
// GAPI-DIV-042 named the stdout JSON decoder, which no longer exists.
// Operator decision 37 moved the protocol off stdout and onto an
// inherited fd, and decision 38 made the frames protobuf - so the
// untrusted input this repo must fuzz moved with it. THIS TARGET
// REPLACES FuzzGoAgentStreamControl AND FuzzPythonAgentStreamControl,
// which were repointed at streamLogs during that move and asserted an
// invariant streamLogs cannot violate: it does no parsing, so it would
// have passed against arbitrary code.
//
// Invariants:
//
//   - totality: arbitrary bytes on an agent's control descriptor never
//     panic the supervisor and always terminate the reader loop;
//   - containment: a frame may only produce the publications the channel
//     is for. Nothing a child process writes may make the supervisor
//     publish a state the frame did not name, and a stream of garbage
//     may not publish at all;
//   - the bad-frame budget holds: an unreadable stream costs a bounded
//     number of log lines, not one per byte.
func FuzzReadControl(f *testing.F) {
	for _, s := range controlSeeds() {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		rec := &recordingPublisher{}
		// io.Discard'd logger: the target must stay deterministic and
		// cheap, and what is being asserted is the publications, not the
		// log text. The budget is asserted separately, by count.
		log := slog.New(slog.DiscardHandler)

		readControl(bytes.NewReader(data), rec, "fuzz-agent", log)

		for _, got := range rec.states {
			if got.state == "" {
				t.Fatalf("published a status with no state from %q", data)
			}
			if got.state != got.noted {
				t.Fatalf("published %q but recorded %q for the exit watcher", got.state, got.noted)
			}
		}
	})
}

// controlSeeds are frames and near-frames. The valid ones are built with
// the real marshaller rather than written as bytes: a seed corpus
// hand-encoding the wire format is a second implementation of the framing
// that can disagree with the first.
func controlSeeds() [][]byte {
	frame := func(msg *gapiv1.AgentControl) []byte {
		var buf bytes.Buffer
		if _, err := protodelim.MarshalTo(&buf, msg); err != nil {
			panic(err)
		}
		return buf.Bytes()
	}
	status := func(v uint32, state string) *gapiv1.AgentControl {
		return &gapiv1.AgentControl{
			SchemaVersion: v,
			Event: &gapiv1.AgentControl_Status{
				Status: &gapiv1.LifecycleStatus{
					AgentId: "seed", State: state, Message: "seed", RunId: "run-1",
				},
			},
		}
	}

	running := frame(status(controlSchemaVersion, "RUNNING"))
	stopped := frame(status(controlSchemaVersion, "STOPPED"))
	heartbeat := frame(&gapiv1.AgentControl{
		SchemaVersion: controlSchemaVersion,
		Event:         &gapiv1.AgentControl_Heartbeat{Heartbeat: &gapiv1.Heartbeat{AgentId: "seed"}},
	})

	seeds := [][]byte{
		running,
		stopped,
		heartbeat,
		append(append([]byte{}, running...), stopped...),                 // ordered pair
		frame(status(controlSchemaVersion, "")),                          // status naming no state
		frame(status(99, "RUNNING")),                                     // unknown schema version
		frame(&gapiv1.AgentControl{SchemaVersion: controlSchemaVersion}), // no arm set
		{},                             // empty stream
		bytes.Repeat([]byte{0x00}, 64), // zero-length frames: the amplification case
		{0xff, 0xff, 0xff, 0xff, 0xff}, // varint that never terminates
		{0x80},                         // truncated varint
		{0x05, 0x01, 0x02},             // length prefix longer than the body
		[]byte("plain text, not a frame at all\n"),
		append([]byte{byte(len(running))}, running...), // length prefix over a framed message
		running[:len(running)-1],                       // truncated frame
		append(append([]byte{}, running...), 0x00),     // valid frame then garbage
		bytes.Repeat(running, 32),                      // burst
	}
	// A frame whose declared length is enormous. protodelim's own limit
	// must refuse it rather than allocate for it.
	seeds = append(seeds, append([]byte{0xff, 0xff, 0xff, 0xff, 0x7f}, []byte(strings.Repeat("A", 16))...))
	return seeds
}

// recordingPublisher captures what readControl asked the agent to do.
type recordingPublisher struct {
	states        []recordedState
	heartbeats    int
	lastHeartbeat *gapiv1.Heartbeat
	pendingRun    string
	pending       string
	// frames counts every frame readControl decoded, including ones it
	// went on to refuse - which is what noteFrameSeen records.
	frames int
}

type recordedState struct {
	state, runID, noted string
}

func (r *recordingPublisher) noteAnnouncedState(state, runID string) {
	r.pending, r.pendingRun = state, runID
}

func (r *recordingPublisher) noteFrameSeen() { r.frames++ }

func (r *recordingPublisher) publishStatusWithRunID(state, _, runID string) {
	r.states = append(r.states, recordedState{state: state, runID: runID, noted: r.pending})
}

func (r *recordingPublisher) publishHeartbeat(hb *gapiv1.Heartbeat) {
	r.heartbeats++
	r.lastHeartbeat = hb
}

var _ statusPublisher = (*recordingPublisher)(nil)

// TestReadControlBudgetsBadFrames pins the amplification ceiling.
//
// MEASURED BEFORE THE BUDGET: 1 MiB of 0x00 produced 131 MB of ERROR log
// in 0.67s, from a stream the function's own comment calls untrusted.
// This asserts the reader gives up instead, which is also what stops the
// CPU cost - a rate limit would keep decoding for as long as the bytes
// keep coming.
func TestReadControlBudgetsBadFrames(t *testing.T) {
	var logged strings.Builder
	log := slog.New(slog.NewTextHandler(&logged, nil))
	rec := &recordingPublisher{}

	// Every zero byte is a zero-length frame that fails the schema check.
	readControl(bytes.NewReader(bytes.Repeat([]byte{0x00}, 1<<20)), rec, "flood", log)

	lines := strings.Count(logged.String(), "\n")
	if lines > maxBadControlFrames+1 {
		t.Fatalf("1 MiB of zeroes logged %d lines, budget is %d", lines, maxBadControlFrames)
	}
	if len(rec.states) != 0 {
		t.Fatalf("garbage published %d statuses", len(rec.states))
	}
}

// TestReadControlPreservesFrameOrder is the ordering claim at the reader.
// The bus half of it is core/eventbus's; this end asserts the reader does
// not reorder what it was handed.
func TestReadControlPreservesFrameOrder(t *testing.T) {
	var buf bytes.Buffer
	for _, state := range []string{"PENDING", "RUNNING", "STOPPED"} {
		msg := &gapiv1.AgentControl{
			SchemaVersion: controlSchemaVersion,
			Event: &gapiv1.AgentControl_Status{
				Status: &gapiv1.LifecycleStatus{AgentId: "ord", State: state, RunId: "run-1"},
			},
		}
		if _, err := protodelim.MarshalTo(&buf, msg); err != nil {
			t.Fatal(err)
		}
	}

	rec := &recordingPublisher{}
	readControl(&buf, rec, "ord", slog.New(slog.DiscardHandler))

	var got []string
	for _, s := range rec.states {
		got = append(got, s.state)
	}
	want := []string{"PENDING", "RUNNING", "STOPPED"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("frames arrived %v, want %v", got, want)
	}
}

// skewBus satisfies statusPublisher. A nil bus is correct here: the
// fuzz corpus drives frame PARSING, and a publish would measure the bus
// rather than the reader.
func (r *recordingPublisher) skewBus() schemaskew.Publisher { return nopPublisher{} }

// nopPublisher accepts and discards.
type nopPublisher struct{}

func (nopPublisher) Publish(eventbus.Event[*anypb.Any]) error { return nil }
