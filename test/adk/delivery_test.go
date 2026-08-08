// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk

import (
	"os"
	"strings"
	"testing"
)

// THE CHECK IS DEMONSTRATED FAILING AGAINST A REAL LOSSY RUN, which is
// what GAPI-DIV-125's exit requires and is the only reason a green from
// it means anything.
//
// The fixture is an excerpt of gapi CI job 93011319891 - the run the
// entry was filed on - holding every line this parser considers and
// nothing else. The entry's finding, established independently of this
// code, is that the published id 019fde51-95de-7332-9ca7-b5b11196bc72
// appears EXACTLY ONCE in the whole 146K job log, on its publish line,
// while thirteen sibling requests were answered normally.
//
// A CHECK FIRST SEEN GREEN PROVES NOTHING, and this suite has shipped
// that mistake before. The lossy capture is what stops this one from
// being another.
func TestDeliveryAuditNamesALostRequest(t *testing.T) {
	lossy := readFixture(t, "testdata/lossy-job93011319891.txt")

	audit := auditDelivery(lossy)

	if len(audit.Published) != 1 {
		t.Fatalf("parsed %d client publishes from the capture, want 1; the parser and the log format have diverged", len(audit.Published))
	}

	err := audit.check()
	if err == nil {
		t.Fatal("delivery audit passed on a capture with a known lost request; the check cannot fail and is not a gate")
	}

	const lost = "019fde51-95de-7332-9ca7-b5b11196bc72"
	if !strings.Contains(err.Error(), lost) {
		t.Errorf("failure does not NAME the lost request %s; got: %v", lost, err)
	}
}

// A CLEAN RUN MUST PASS, or the check above could be satisfied by one
// that always fails - which would be a gate in the same useless
// direction, just noisier.
func TestDeliveryAuditPassesWhenEveryRequestArrives(t *testing.T) {
	clean := `2026/08/08 00:00:00 INFO structured event module=eventbus event=publish event_id=aaa-1 source=client topic=agents/
2026/08/08 00:00:00 INFO structured event module=transport event=receive event_id=aaa-1 source=client topic=agents/
2026/08/08 00:00:01 INFO structured event module=eventbus event=publish event_id=bbb-2 source=client topic=ping
2026/08/08 00:00:01 INFO structured event module=transport event=receive event_id=bbb-2 source=client topic=ping`

	audit := auditDelivery(clean)
	if len(audit.Published) != 2 {
		t.Fatalf("parsed %d publishes, want 2", len(audit.Published))
	}
	if err := audit.check(); err != nil {
		t.Fatalf("clean run reported a loss: %v", err)
	}
}

// ZERO PUBLISHES IS A FAILURE, NOT A PASS.
//
// This is the clause that keeps the gate honest when something upstream
// breaks. If the log format changes or the harness stops capturing a
// side, the audit would otherwise inspect nothing, find nothing missing,
// and report success for the rest of its life - which is the
// gate-that-cannot-fail this repository has now produced enough times to
// test for on purpose.
func TestDeliveryAuditFailsWhenItInspectedNothing(t *testing.T) {
	for _, tt := range []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"daemon chatter only, no client publishes", `2026/08/08 00:00:00 INFO structured event module=transport event=receive event_id=zzz-9
2026/08/08 00:00:00 INFO structured event module=eventbus event=publish event_id=yyy-8 source=supervisor topic=agent/heartbeat`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := auditDelivery(tt.text).check()
			if err == nil {
				t.Fatal("audit passed having inspected zero client publishes")
			}
			if !strings.Contains(err.Error(), "ZERO") {
				t.Errorf("failure does not say it verified nothing; got: %v", err)
			}
		})
	}
}

// A RETRIED REQUEST IS ONE REQUEST. GAPI-DIV-122 republishes the SAME
// event, so counting attempts would report one request as several and
// make "3 of 5 never arrived" arithmetic that cannot be reconciled with
// the log a reader is holding.
func TestDeliveryAuditCountsARetriedRequestOnce(t *testing.T) {
	retried := `2026/08/08 00:00:00 INFO structured event module=eventbus event=publish event_id=ccc-3 source=client topic=agents/
2026/08/08 00:00:00 INFO structured event module=eventbus event=publish event_id=ccc-3 source=client topic=agents/
2026/08/08 00:00:01 INFO structured event module=eventbus event=publish event_id=ccc-3 source=client topic=agents/`

	audit := auditDelivery(retried)
	if len(audit.Published) != 1 {
		t.Fatalf("three republishes of one event counted as %d requests, want 1", len(audit.Published))
	}
	if err := audit.check(); err == nil {
		t.Fatal("a request republished three times and never received must still be reported lost")
	}
}

// The two sides come from different processes, so the join has to work
// across separate texts rather than one concatenated blob.
func TestDeliveryAuditJoinsAcrossTwoLogs(t *testing.T) {
	clientLog := `2026/08/08 00:00:00 INFO structured event module=eventbus event=publish event_id=ddd-4 source=client topic=agents/`
	daemonLog := `2026/08/08 00:00:00 INFO structured event module=transport event=receive event_id=ddd-4 source=client topic=agents/`

	if err := auditDelivery(clientLog, daemonLog).check(); err != nil {
		t.Fatalf("join across two logs reported a loss: %v", err)
	}
	if err := auditDelivery(clientLog).check(); err == nil {
		t.Fatal("the client log alone contains no arrival, so the request must read as lost")
	}
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(b)
}

// BOTH LOG FORMATS, WITH REAL LINES FROM THE RUN THAT CAUGHT IT.
//
// This suite emits both at once - gapid logs JSON, gapictl logs text -
// and the first version of the audit only read text. Every daemon
// arrival was invisible, so the only receipts it saw were the client's
// own heartbeats, and it reported a lifecycle action lost that the
// daemon's log plainly recorded arriving. The test whose assertions had
// all passed failed at teardown on a defect in the checker.
//
// The lines below are copied verbatim from that run, which is the point:
// a hand-written approximation of the format is what let the gap through
// the first time.
func TestDeliveryAuditReadsTextAndJSON(t *testing.T) {
	clientText := `2026/08/08 13:22:13 INFO structured event module=eventbus event=publish event_id=019fe265-8c6d-79cf-9eb1-fc1f4856dddc source=client topic=agent/lifecycle.action payload_type=*anypb.Any`
	daemonJSON := `{"time":"2026-08-08T13:22:13.22819209-04:00","level":"INFO","msg":"structured event","module":"transport","event":"receive","event_id":"019fe265-8c6d-79cf-9eb1-fc1f4856dddc","source":"client","topic":"agent/lifecycle.action"}`

	audit := auditDelivery(clientText, daemonJSON)
	if len(audit.Published) != 1 {
		t.Fatalf("parsed %d publishes from the text log, want 1", len(audit.Published))
	}
	if len(audit.Received) != 1 {
		t.Fatalf("parsed %d arrivals from the JSON log, want 1; a JSON-blind audit reports live requests lost", len(audit.Received))
	}
	if err := audit.check(); err != nil {
		t.Fatalf("request published in text and received in JSON reported lost: %v", err)
	}
}

// The reverse pairing too, so neither format is only ever exercised on
// one side of the join.
func TestDeliveryAuditReadsJSONPublishAndTextReceive(t *testing.T) {
	publishJSON := `{"time":"2026-08-08T13:22:13Z","level":"INFO","msg":"structured event","module":"eventbus","event":"publish","event_id":"eee-5","source":"client","topic":"agents/"}`
	receiveText := `2026/08/08 13:22:13 INFO structured event module=transport event=receive event_id=eee-5 source=client topic=agents/`

	if err := auditDelivery(publishJSON, receiveText).check(); err != nil {
		t.Fatalf("JSON publish joined to text receive reported lost: %v", err)
	}
}
