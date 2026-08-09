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
	"fmt"
	"sync"
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
