// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

// Package schemaskew decides what a contract mismatch is, once.
//
// THE DAEMON MUST NOT COMPARE TWO WAYS. Registration sees the hash an
// agent reported at --describe and the status path sees the one it
// puts on the wire, and those live in different packages - the
// supervisor and the agent manager. A copy of this logic in each is the
// drift GAPI-DIV-127 was filed about, one layer up from the value
// itself.
package schemaskew

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/internal/logattr"
	"google.golang.org/protobuf/types/known/anypb"
)

// Contract-skew reporting (GAPI-DIV-127).
//
// THE DAEMON REPORTS AND NEVER REFUSES. Operator decision 71: an exact
// hash answers "were these built from the same contract sources", not
// "can these two safely talk" - protobuf exists so those differ, and
// buf's breaking check is the ecosystem's compatibility oracle at build
// time. The daemon learns "different", not "unsafe", and refusal is not
// supported by that evidence.
//
// Boot targets make the asymmetry concrete rather than theoretical: a
// boot-local agent refused at start can strand network-ready.target and
// abort a node through gate expiry, which a log line cannot.

// TopicSchemaSkew carries a detected contract mismatch.
//
// IT HAS NO CONSUMER TODAY AND THAT IS DELIBERATE, tracked as
// GOBLIN-DIV-080 so the gap sits in the ledger rather than in someone's
// memory. The daemon cannot remediate: `agent build` is a gapictl verb
// that shells out to `go build`, and a daemon on a nix-built or
// image-based host has no toolchain. Remediation belongs to the
// orchestrator, which has a scheduler and can place an agent on a node
// whose daemon matches - so this event is the seam by which that becomes
// a subscriber rather than a change to gapi.
const TopicSchemaSkew = "agent.schema.skew"

// Report renders a mismatch and answers whether there is one.
//
// SILENT IN BOTH UNKNOWN DIRECTIONS. An agent predating the field
// reports "", and so would a daemon that could not compute its own. In
// neither case is "different" known, and reporting on absence would flag
// an entire fleet on the first upgrade - a diagnostic that fires on
// everything gets filtered, and a filtered warning is no warning. Skew
// is a claim that two KNOWN values differ.
//
// runID is omitted when empty rather than printed. Registration has no
// run id because the agent has not started, and `run ""` reads as a run
// whose id is the empty string - a different and wrong claim.
func Report(agentID, runID, agentHash, daemonHash string) (string, bool) {
	if agentHash == "" || daemonHash == "" || agentHash == daemonHash {
		return "", false
	}

	who := "agent " + agentID
	if runID != "" {
		who = fmt.Sprintf("agent %s (run %s)", agentID, runID)
	}

	// The ledger id stays in the COMMENTS and out of this string. An
	// operator reading it at 3am does not have a divergence ledger, and
	// core/product's scan is right to call a "GAPI-" prefix in
	// operator-facing prose a vendor name leaking into goblind's output.
	return fmt.Sprintf(
		"%s was built against protobuf contract %s; this daemon carries %s. "+
			"The agent is NOT refused - this is a diagnostic, not an "+
			"enforcement decision.",
		who, agentHash, daemonHash), true
}

// Seen remembers which incarnations have already been reported.
//
// KEYED ON run_id RATHER THAN AGENT ID. A restarted agent is a new
// process that may carry a different binary, so suppressing its report
// because its predecessor was reported would hide exactly the case a
// redeploy creates - which is the case this path exists for.
//
// It grows with the number of skewed incarnations a daemon sees, which
// is bounded by how often somebody redeploys a mismatched agent without
// running reload. That is small, and an entry is two short strings; a
// daemon that accumulates enough of these has a louder problem than its
// memory.
type Seen struct {
	mu   sync.Mutex
	runs map[string]bool
}

func NewSeen() *Seen {
	return &Seen{runs: map[string]bool{}}
}

// First reports whether this is the first sighting of runID.
func (s *Seen) First(runID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs[runID] {
		return false
	}
	s.runs[runID] = true
	return true
}

// Publisher is the bus a Reporter emits on. An interface rather than
// the concrete bus so this package stays a leaf both the supervisor and
// the agent manager can import without either of them importing the
// other.
type Publisher interface {
	Publish(eventbus.Event[*anypb.Any]) error
}

// Reporter emits a mismatch the one way the daemon emits them.
//
// THE DECISION AND THE EMIT ARE BOTH HERE, and the second is why this
// type exists rather than only Report. The daemon has two detection
// sites in two packages - registration in core/supervisor, status in
// core/agentmgr - and the log line, its level, its attributes and the
// event topic are as much "what a mismatch is" as the comparison. Two
// copies drift, and a mismatch reported at WARN in one place and INFO
// in the other is a mismatch an operator only sometimes sees.
type Reporter struct {
	log    *slog.Logger
	bus    Publisher
	source func() string
	module string
	seen   *Seen
}

// NewReporter builds one.
//
// module names the subsystem in the log attributes. source is the
// event's origin and must be DERIVED from core/product rather than
// spelled, because goblind links this code and its operators have never
// heard of gapid (GAPI-DIV-061).
//
// SOURCE IS A FUNCTION, NOT A STRING, and that is not indirection for
// its own sake. product.Daemon() PANICS on an unset identity, which is
// correct for a daemon and wrong for a frame parser: readControl builds
// a Reporter for every control stream, and resolving the name eagerly
// made the kernel's most exposed parser require a global before it
// could read a byte. Resolved on the one path that publishes, which in
// a real process always has an identity.
func NewReporter(log *slog.Logger, bus Publisher, source func() string, module string) *Reporter {
	return &Reporter{log: log, bus: bus, source: source, module: module, seen: NewSeen()}
}

// Report emits without deduplication, for a site whose triggers are
// already deliberate: daemon start and an explicit `agent reload`.
// Silence when an operator has just asked the daemon to re-read would be
// the defect, not the noise.
func (r *Reporter) Report(agentID, agentHash, daemonHash string) {
	r.emit(agentID, "", agentHash, daemonHash)
}

// ReportOnce emits at most once per incarnation, for a site whose
// messages repeat: status is per-transition, so a skewed agent that
// transitions often would repeat the warning until operators filter the
// topic - and a filtered warning is no warning.
func (r *Reporter) ReportOnce(agentID, runID, agentHash, daemonHash string) {
	msg, isSkew := Report(agentID, runID, agentHash, daemonHash)
	if !isSkew || !r.seen.First(runID) {
		return
	}
	r.write(msg, agentID, agentHash)
}

func (r *Reporter) emit(agentID, runID, agentHash, daemonHash string) {
	msg, isSkew := Report(agentID, runID, agentHash, daemonHash)
	if !isSkew {
		return
	}
	r.write(msg, agentID, agentHash)
}

func (r *Reporter) write(msg, agentID, agentHash string) {
	r.log.LogAttrs(context.Background(), slog.LevelWarn, msg,
		logattr.Module(r.module),
		logattr.AgentID(agentID),
		logattr.Hash(agentHash))

	evt := eventbus.NewEvent[*anypb.Any]("system", "", TopicSchemaSkew, r.source(), nil)
	if err := r.bus.Publish(evt); err != nil {
		r.log.LogAttrs(context.Background(), slog.LevelError,
			"failed to publish schema skew event",
			logattr.Module(r.module), logattr.AgentID(agentID), logattr.Err(err))
	}
}
