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
	"log/slog"

	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/product"
	"github.com/goppydae/gapi/core/schemahash"
	"github.com/goppydae/gapi/core/schemaskew"
	"github.com/goppydae/gapi/internal/agentreg"
	"github.com/goppydae/gapi/internal/logattr"
	"google.golang.org/protobuf/types/known/anypb"
)

// The daemon's two reporting sites for a contract mismatch
// (GAPI-DIV-127). WHAT a mismatch is lives in core/schemaskew, once,
// because the other site is in core/agentmgr and two copies of that
// decision is the drift this entry is about.

// reportStatusSkew covers the case registration cannot: a binary
// replaced after discovery when nobody ran `gapictl agent reload`.
//
// DEDUPED, unlike the registration path, and the difference is not
// taste. Status is per-transition, so a skewed agent that transitions
// often would repeat the warning until operators filter the topic - and
// a filtered warning is no warning. Registration fires only at daemon
// start and on an explicit reload, where silence would be the defect.
func (s *Supervisor) reportStatusSkew(agentID, runID, agentHash string) {
	msg, isSkew := schemaskew.Report(agentID, runID, agentHash, schemahash.Contract())
	if !isSkew || !s.skew.First(runID) {
		return
	}

	s.logger.LogAttrs(context.Background(), slog.LevelWarn, msg,
		logattr.Module("lifecycle"),
		logattr.AgentID(agentID),
		logattr.Hash(agentHash))

	evt := eventbus.NewEvent[*anypb.Any]("system", "", schemaskew.TopicSchemaSkew, product.Daemon(), nil)
	if err := s.bus.Publish(evt); err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError,
			"failed to publish schema skew event",
			logattr.Module("lifecycle"), logattr.AgentID(agentID), logattr.Err(err))
	}
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
	msg, isSkew := schemaskew.Report(ad.ID, "", ad.SchemaHash, schemahash.Contract())
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
	evt := eventbus.NewEvent[*anypb.Any]("system", "", schemaskew.TopicSchemaSkew, product.Daemon(), nil)
	if err := s.bus.Publish(evt); err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError,
			"failed to publish schema skew event",
			logattr.Module("discovery"), logattr.AgentID(ad.ID), logattr.Err(err))
	}
}
