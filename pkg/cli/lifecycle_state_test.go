// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"testing"

	protopkg "github.com/goppydae/gapi/pkg/proto"
)

// BOTH SPELLINGS OF FAILURE MUST BE RECOGNISED.
//
// The runtime state surface is a string, not the enum it declares
// (GAPI-DIV-083), and the daemon does not spell it one way: one path
// writes the bare literal "FAILED" and others carry AgentState.String(),
// which renders "AGENT_STATE_FAILED". A check matching only one is blind
// to half the daemon's outputs while looking correct.
func TestIsFailedStateMatchesBothSpellings(t *testing.T) {
	for _, s := range []string{"FAILED", protopkg.AgentState_AGENT_STATE_FAILED.String()} {
		if !isFailedState(s) {
			t.Errorf("isFailedState(%q) = false; the daemon emits this spelling for a failed action", s)
		}
	}

	// A running or stopped agent is not a failed action. Guarding the
	// negative matters as much as the positive: a predicate that returns
	// true for everything turns every lifecycle call into an error and
	// gets reverted rather than fixed.
	for _, s := range []string{
		"STOPPED",
		"RUNNING",
		protopkg.AgentState_AGENT_STATE_STOPPED.String(),
		protopkg.AgentState_AGENT_STATE_RUNNING.String(),
		"",
	} {
		if isFailedState(s) {
			t.Errorf("isFailedState(%q) = true; only a failed action may fail the command", s)
		}
	}
}
