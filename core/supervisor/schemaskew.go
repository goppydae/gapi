// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package supervisor

import (
	"github.com/goppydae/gapi/core/schemahash"
	"github.com/goppydae/gapi/internal/agentreg"
)

// The daemon's two reporting sites for a contract mismatch
// (GAPI-DIV-127). WHAT a mismatch is lives in core/schemaskew, once,
// because the other site is in core/agentmgr and two copies of that
// decision is the drift this entry is about.

// reportSchemaSkew warns and publishes for one registered agent.
//
// NOT DEDUPED: setupAgents has exactly two triggers, daemon start and
// the system/agent.reload subscriber, and both are deliberate operator
// moments. The status path in core/agentmgr dedupes instead, because
// transitions repeat within one incarnation.
//
// It must never alter registration, start or agent state. A change that
// makes this able to stop an agent is wrong even if its tests pass -
// operator decision 71.
func (s *Supervisor) reportSchemaSkew(ad *agentreg.AgentDescription) {
	s.skew.Report(ad.ID, ad.SchemaHash, schemahash.Contract())
}
