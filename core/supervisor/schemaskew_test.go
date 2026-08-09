// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package supervisor

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/product"
	"github.com/goppydae/gapi/core/schemahash"
	"github.com/goppydae/gapi/internal/agentreg"
	"google.golang.org/protobuf/types/known/anypb"
)

// TestSkewReportNamesBothHashes. The value of the report IS the two
// values side by side; a message saying only "schema mismatch" sends the
// reader back to the two binaries to find out what differed, which is
// the work the report exists to remove.
func TestSkewReportNamesBothHashes(t *testing.T) {
	msg, isSkew := skewReport("weather", "run-7", "aaa", "bbb")
	if !isSkew {
		t.Fatal("differing hashes were not reported as skew")
	}
	for _, want := range []string{"weather", "run-7", "aaa", "bbb"} {
		if !strings.Contains(msg, want) {
			t.Errorf("report omits %q: %s", want, msg)
		}
	}
}

// TestSkewReportOmitsAnAbsentRun. Registration has no run id - the agent
// has not started - so the report must not print an empty one. `run ""`
// in a log line reads as a run whose id is the empty string, which is a
// different and wrong claim.
func TestSkewReportOmitsAnAbsentRun(t *testing.T) {
	msg, isSkew := skewReport("weather", "", "aaa", "bbb")
	if !isSkew {
		t.Fatal("differing hashes were not reported as skew")
	}
	if strings.Contains(msg, "run ") {
		t.Errorf("report invents a run id at registration: %s", msg)
	}
	if !strings.Contains(msg, "weather") {
		t.Errorf("report omits the agent: %s", msg)
	}
}

// TestSkewReportIsSilentWhenTheyMatch is the ordinary case, and the one
// that must produce nothing at all.
func TestSkewReportIsSilentWhenTheyMatch(t *testing.T) {
	if _, isSkew := skewReport("weather", "run-7", "same", "same"); isSkew {
		t.Fatal("matching hashes reported as skew")
	}
}

// TestSkewReportIsSilentWhenTheAgentPredatesTheField.
//
// An agent built before schema_hash existed reports "". Calling that a
// mismatch would flag EVERY older agent in a fleet on the first upgrade,
// and a diagnostic that fires on everything is noise - which is how a
// signal gets filtered and then lost.
func TestSkewReportIsSilentWhenTheAgentPredatesTheField(t *testing.T) {
	if _, isSkew := skewReport("weather", "run-7", "", "bbb"); isSkew {
		t.Fatal("an agent with no schema_hash was reported as skewed")
	}
}

// TestSkewReportIsSilentWhenTheDaemonCannotAnswer guards the direction
// nobody thinks to test. If the daemon's own hash were ever empty, every
// agent on the node would be reported as skewed against nothing.
func TestSkewReportIsSilentWhenTheDaemonCannotAnswer(t *testing.T) {
	if _, isSkew := skewReport("weather", "run-7", "aaa", ""); isSkew {
		t.Fatal("skew reported against an empty daemon hash")
	}
}

// TestSkewReportSaysTheAgentIsNotRefused pins operator decision 71 into
// the artifact an operator actually reads.
//
// The decision is that the hash is a diagnostic and never an enforcement
// input. An operator meeting this line at 3am must not have to go and
// find out whether their agent was just refused, and a future change
// that starts refusing will have to edit this text - which is the point.
func TestSkewReportSaysTheAgentIsNotRefused(t *testing.T) {
	msg, _ := skewReport("weather", "run-7", "aaa", "bbb")
	if !strings.Contains(msg, "NOT refused") {
		t.Errorf("report does not say the agent still runs: %s", msg)
	}
}

// captureLogs returns a logger writing structured records into buf.
func captureLogs(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// TestReportSchemaSkewWarnsAndPublishes covers the reporting path as a
// whole: the WARN an operator reads, and the event GOBLIN-DIV-080 will
// consume.
//
// Both halves are asserted because they fail independently. A publish
// that silently no-ops leaves the log intact and the seam dead, which
// is the state that entry was filed against.
func TestReportSchemaSkewWarnsAndPublishes(t *testing.T) {
	// The event source is DERIVED from core/product, which panics on
	// an unset identity by design (GAPI-DIV-061). A test that skipped
	// this would be exercising a process no binary ever starts.
	product.Set("gapi")

	var buf bytes.Buffer
	bus := eventbus.NewInprocBus[*anypb.Any]()

	got := make(chan string, 4)
	if err := bus.Subscribe("system", "", TopicSchemaSkew,
		func(e eventbus.Event[*anypb.Any]) { got <- e.Topic }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	s := &Supervisor{logger: captureLogs(&buf), bus: bus}
	s.reportSchemaSkew(&agentreg.AgentDescription{
		ID:         "weather",
		SchemaHash: "not-the-daemons-hash",
	})

	out := buf.String()
	if !strings.Contains(out, "WARN") {
		t.Errorf("skew was not reported at WARN:\n%s", out)
	}
	if !strings.Contains(out, "not-the-daemons-hash") {
		t.Errorf("the report omits the agent's hash:\n%s", out)
	}
	if !strings.Contains(out, schemahash.Contract()) {
		t.Errorf("the report omits the daemon's hash:\n%s", out)
	}

	select {
	case topic := <-got:
		if topic != TopicSchemaSkew {
			t.Errorf("published on %q, want %q", topic, TopicSchemaSkew)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event was published; the seam GOBLIN-DIV-080 tracks is dead")
	}
}

// TestReportSchemaSkewIsSilentForAMatchingAgent is the other half, and
// the one that keeps the report worth reading. An agent built against
// this daemon's contract must produce no log line and no event.
func TestReportSchemaSkewIsSilentForAMatchingAgent(t *testing.T) {
	// The event source is DERIVED from core/product, which panics on
	// an unset identity by design (GAPI-DIV-061). A test that skipped
	// this would be exercising a process no binary ever starts.
	product.Set("gapi")

	var buf bytes.Buffer
	bus := eventbus.NewInprocBus[*anypb.Any]()

	got := make(chan string, 4)
	if err := bus.Subscribe("system", "", TopicSchemaSkew,
		func(e eventbus.Event[*anypb.Any]) { got <- e.Topic }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	s := &Supervisor{logger: captureLogs(&buf), bus: bus}
	s.reportSchemaSkew(&agentreg.AgentDescription{
		ID:         "weather",
		SchemaHash: schemahash.Contract(),
	})

	if out := buf.String(); strings.Contains(out, "WARN") {
		t.Errorf("a matching agent produced a warning:\n%s", out)
	}
	select {
	case topic := <-got:
		t.Fatalf("a matching agent published %q", topic)
	case <-time.After(200 * time.Millisecond):
	}
}
