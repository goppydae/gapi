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

	"github.com/goppydae/gapi/core/product"
	gapiv1 "github.com/goppydae/gapi/pkg/proto"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// statusFrames encodes one control stream carrying the given hashes.
func statusFrames(t *testing.T, runID string, hashes ...string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	for _, h := range hashes {
		frame := &gapiv1.AgentControl{
			SchemaVersion: controlSchemaVersion,
			Event: &gapiv1.AgentControl_Status{
				Status: &gapiv1.LifecycleStatus{
					AgentId:    "probe",
					State:      "RUNNING",
					Time:       timestamppb.Now(),
					RunId:      runID,
					SchemaHash: h,
				},
			},
		}
		if _, err := protodelim.MarshalTo(&buf, frame); err != nil {
			t.Fatalf("encode frame: %v", err)
		}
	}
	return &buf
}

// TestControlReaderReportsAStatusHashMismatch is the daemon half of the
// mid-flight path.
//
// The frame is decoded HERE, not in the supervisor: readControl is what
// receives an agent's status, so this is where the reported contract can
// be compared at all. Registration cannot cover this case - it is a
// binary replaced after discovery, when nobody ran `gapictl agent
// reload`.
func TestControlReaderReportsAStatusHashMismatch(t *testing.T) {
	// The event source is derived from core/product, which panics on an
	// unset identity by design (GAPI-DIV-061).
	product.Set("gapi")

	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	rp := &recordingPublisher{}
	readControl(statusFrames(t, "run-1", "not-the-daemons-hash"), rp, "probe", log)

	out := logs.String()
	if !strings.Contains(out, "not-the-daemons-hash") {
		t.Fatalf("a mismatched status hash was dropped on arrival:\n%s", out)
	}
	if !strings.Contains(out, "NOT refused") {
		t.Errorf("the report does not say the agent still runs:\n%s", out)
	}
}

// TestControlReaderReportsOncePerRun. Status is per-transition, so a
// skewed agent that transitions repeatedly must warn once - otherwise
// operators filter the topic and the signal is gone.
func TestControlReaderReportsOncePerRun(t *testing.T) {
	product.Set("gapi")

	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	rp := &recordingPublisher{}
	readControl(statusFrames(t, "run-1", "bad", "bad", "bad", "bad"), rp, "probe", log)

	if n := strings.Count(logs.String(), "was built against protobuf contract"); n != 1 {
		t.Fatalf("four transitions in one run produced %d reports, want 1", n)
	}
}

// TestControlReaderIsSilentForAnAgentWithNoHash keeps an older fleet
// quiet: an agent predating the field sends "" and must not be flagged.
func TestControlReaderIsSilentForAnAgentWithNoHash(t *testing.T) {
	product.Set("gapi")

	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	rp := &recordingPublisher{}
	readControl(statusFrames(t, "run-1", ""), rp, "probe", log)

	if strings.Contains(logs.String(), "was built against protobuf contract") {
		t.Errorf("an agent that sent no hash was reported:\n%s", logs.String())
	}
}
