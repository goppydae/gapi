// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package supervisor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/product"
	"github.com/goppydae/gapi/core/schemahash"
	"github.com/goppydae/gapi/internal/agentreg"
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

// skewReport renders a mismatch and answers whether there is one.
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
func skewReport(agentID, runID, agentHash, daemonHash string) (string, bool) {
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

// reportSchemaSkew warns and publishes for one registered agent.
//
// NOT DEDUPED, deliberately. setupAgents has exactly two triggers -
// daemon start, and the system/agent.reload subscriber - and both are
// deliberate operator moments. Someone who has just run
// `gapictl agent reload` is asking the daemon to re-read what is on
// disk; silence there would be the defect, not the noise. The status
// path is deduped instead, because transitions repeat within one
// incarnation.
//
// It returns nothing and must never alter registration, start or agent
// state. A change that makes this function able to stop an agent is
// wrong even if its tests pass.
func (s *Supervisor) reportSchemaSkew(ad *agentreg.AgentDescription) {
	msg, isSkew := skewReport(ad.ID, "", ad.SchemaHash, schemahash.Contract())
	if !isSkew {
		return
	}

	s.logger.LogAttrs(context.Background(), slog.LevelWarn, msg,
		logattr.Module("discovery"),
		logattr.AgentID(ad.ID),
		logattr.Hash(ad.SchemaHash))

	// DERIVED, not spelled. supervisor.go carries a waiver for the
	// literal "gapid"; this needs none, because product.Daemon() is the
	// same value under whichever product links this code - and goblind
	// links it (GAPI-DIV-061).
	evt := eventbus.NewEvent[*anypb.Any]("system", "", TopicSchemaSkew, product.Daemon(), nil)
	if err := s.bus.Publish(evt); err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError,
			"failed to publish schema skew event",
			logattr.Module("discovery"), logattr.AgentID(ad.ID), logattr.Err(err))
	}
}
