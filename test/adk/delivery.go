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
	"regexp"
	"sort"
	"strings"
)

// THE GATE GAPI-DIV-125 CLOSES ON: A LOST REQUEST IS NAMED, NOT INFERRED.
//
// The defect this exists for is a control request that is published with
// no error and never arrives, on a connection that is demonstrably
// alive. It took five occurrences and three wrong attributions because
// nothing could SEE it: the loss was reconstructed from reply timings
// and, once, from noticing that one event id appeared exactly once in a
// 146K job log. That is archaeology, not a gate.
//
// The join key is the event id, which both sides already print. A client
// publish is `module=eventbus event=publish event_id=... source=client`;
// an arrival is `module=transport event=receive event_id=...`, which
// exists only because the receive path was taught to log success. Before
// that this check could not have been written at all - the system logged
// every transmission and no reception, so no log could locate a loss
// even in principle.
//
// DELIBERATELY A PURE FUNCTION OVER TEXT, not a probe wired into the
// transport. The entry requires this assertion be DEMONSTRATED FAILING
// against a captured lossy run, and the runs that exhibit the loss are
// archived CI logs from a tree that no longer reproduces it. A checker
// that only runs live could never be shown to fail; one that takes text
// can be pointed at testdata/ and proved.

// TWO LOG FORMATS, AND ASSUMING ONE OF THEM PRODUCED A FALSE POSITIVE ON
// THE FIRST LIVE RUN. Decision 11 allows slog Text or JSON, and this
// suite uses BOTH AT ONCE: gapid emits JSON while gapictl emits text. A
// first version of this file matched whole-line text shapes, so every
// daemon arrival was invisible and the only receipts it could see were
// the CLIENT's own heartbeats. It reported a lifecycle action lost that
// the daemon's log plainly recorded arriving, in a test whose own
// assertions had passed - which would have turned the entire ADK suite
// red on a defect in the checker.
//
// So fields are extracted INDIVIDUALLY rather than matched as a line
// shape. That reads both formats, and it does not care what order slog
// happens to emit attributes in.
// Precompiled rather than built on demand into a package-level map: the
// field set is fixed and known, and a lazily-filled global would be
// mutable shared state for no gain.
var fieldRes = map[string]*regexp.Regexp{
	"module":   fieldPattern("module"),
	"event":    fieldPattern("event"),
	"event_id": fieldPattern("event_id"),
	"source":   fieldPattern("source"),
	"topic":    fieldPattern("topic"),
}

func fieldPattern(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?:"` + name + `":"([^"]*)"|\b` + name + `=([^\s,}"]+))`)
}

// field returns the value of a structured log attribute, in either the
// text form `name=value` or the JSON form `"name":"value"`.
func field(line, name string) string {
	m := fieldRes[name].FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

// publishedRequest is one control request a client put on the wire.
type publishedRequest struct {
	ID    string
	Topic string
}

// deliveryAudit is what the logs say about requests and arrivals.
type deliveryAudit struct {
	Published []publishedRequest
	Received  map[string]struct{}
}

// auditDelivery joins publishes to arrivals across any number of log
// texts. The two sides normally come from different processes - the
// client's output and the daemon's - so they are separate arguments
// rather than one concatenated blob, and a caller passing only one gets
// a truthful answer about what that one contains.
//
// ONLY source=client PUBLISHES COUNT. The daemon publishes constantly -
// replies, heartbeats, lifecycle events - and holding it to "every
// publish must be received by somebody" would assert something this
// suite does not know and does not care about. The claim is narrower and
// is the one that broke: a request a CLIENT sent must arrive.
func auditDelivery(logs ...string) deliveryAudit {
	a := deliveryAudit{Received: map[string]struct{}{}}
	seen := map[string]struct{}{}

	for _, text := range logs {
		for _, line := range strings.Split(text, "\n") {
			id := field(line, "event_id")
			if id == "" {
				continue
			}
			switch {
			case field(line, "event") == "publish" &&
				field(line, "module") == "eventbus" &&
				field(line, "source") == "client":
				// Deduplicated by id: a retry republishes the SAME event
				// (GAPI-DIV-122), so counting each attempt would report
				// one request as several and make the arithmetic lie.
				if _, dup := seen[id]; !dup {
					seen[id] = struct{}{}
					a.Published = append(a.Published, publishedRequest{ID: id, Topic: field(line, "topic")})
				}
			case field(line, "event") == "receive" &&
				field(line, "module") == "transport":
				a.Received[id] = struct{}{}
			}
		}
	}
	return a
}

// unmatched returns the published requests with no arrival, sorted so a
// failure message is stable across runs.
func (a deliveryAudit) unmatched() []publishedRequest {
	var out []publishedRequest
	for _, p := range a.Published {
		if _, ok := a.Received[p.ID]; !ok {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// check reports whether every client request arrived.
//
// IT FAILS ON ZERO PUBLISHES, AND THAT CLAUSE IS NOT DEFENSIVE PADDING.
// A walker that silently matches nothing and reports success is the
// exact shape this repository has already shipped more than once - a
// gate that could not fail, green because its subject was absent. If the
// log format changes, or the harness stops capturing one side, this
// check would otherwise go quietly green for the rest of its life.
// Failing loudly on an empty inspection is what makes a pass mean
// something.
func (a deliveryAudit) check() error {
	if len(a.Published) == 0 {
		return fmt.Errorf(
			"delivery audit inspected ZERO client publishes: it cannot have verified anything. " +
				"Either no control request was issued, or the log capture or the publish format changed")
	}

	lost := a.unmatched()
	if len(lost) == 0 {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d of %d client requests were published and never arrived (GAPI-DIV-125):",
		len(lost), len(a.Published))
	for _, p := range lost {
		fmt.Fprintf(&b, "\n  event_id=%s topic=%s", p.ID, p.Topic)
	}
	fmt.Fprintf(&b, "\n%d arrivals were recorded in total.", len(a.Received))
	return fmt.Errorf("%s", b.String())
}
