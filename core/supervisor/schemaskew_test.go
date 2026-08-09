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
	"github.com/goppydae/gapi/core/schemaskew"
	"github.com/goppydae/gapi/internal/agentreg"
	"google.golang.org/protobuf/types/known/anypb"
)

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
	if err := bus.Subscribe("system", "", schemaskew.TopicSchemaSkew,
		func(e eventbus.Event[*anypb.Any]) { got <- e.Topic }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	log := captureLogs(&buf)
	s := &Supervisor{
		logger: log,
		bus:    bus,
		skew:   schemaskew.NewReporter(log, bus, product.Daemon, "discovery"),
	}
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
		if topic != schemaskew.TopicSchemaSkew {
			t.Errorf("published on %q, want %q", topic, schemaskew.TopicSchemaSkew)
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
	if err := bus.Subscribe("system", "", schemaskew.TopicSchemaSkew,
		func(e eventbus.Event[*anypb.Any]) { got <- e.Topic }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	log := captureLogs(&buf)
	s := &Supervisor{
		logger: log,
		bus:    bus,
		skew:   schemaskew.NewReporter(log, bus, product.Daemon, "discovery"),
	}
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

// The status path's cases live in core/agentmgr, where the frame is
// decoded, and the pure decision's live in core/schemaskew. This file
// keeps only what the SUPERVISOR does: registration-time reporting.
