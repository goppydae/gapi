// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package schemaskew

import (
	"strings"
	"testing"
)

// TestSkewReportNamesBothHashes. The value of the report IS the two
// values side by side; a message saying only "schema mismatch" sends the
// reader back to the two binaries to find out what differed, which is
// the work the report exists to remove.
func TestSkewReportNamesBothHashes(t *testing.T) {
	msg, isSkew := Report("weather", "run-7", "aaa", "bbb")
	if !isSkew {
		t.Fatal("differing hashes were not reported as skew")
	}
	for _, want := range []string{"weather", "run-7", "aaa", "bbb"} {
		if !strings.Contains(msg, want) {
			t.Errorf("report omits %q: %s", want, msg)
		}
	}
}

// TestSkewReportOmitsAnAbsentRun. Registration has no run id - the agent
// has not started - so the report must not print an empty one. `run ""`
// in a log line reads as a run whose id is the empty string, which is a
// different and wrong claim.
func TestSkewReportOmitsAnAbsentRun(t *testing.T) {
	msg, isSkew := Report("weather", "", "aaa", "bbb")
	if !isSkew {
		t.Fatal("differing hashes were not reported as skew")
	}
	if strings.Contains(msg, "run ") {
		t.Errorf("report invents a run id at registration: %s", msg)
	}
	if !strings.Contains(msg, "weather") {
		t.Errorf("report omits the agent: %s", msg)
	}
}

// TestSkewReportIsSilentWhenTheyMatch is the ordinary case, and the one
// that must produce nothing at all.
func TestSkewReportIsSilentWhenTheyMatch(t *testing.T) {
	if _, isSkew := Report("weather", "run-7", "same", "same"); isSkew {
		t.Fatal("matching hashes reported as skew")
	}
}

// TestSkewReportIsSilentWhenTheAgentPredatesTheField.
//
// An agent built before schema_hash existed reports "". Calling that a
// mismatch would flag EVERY older agent in a fleet on the first upgrade,
// and a diagnostic that fires on everything is noise - which is how a
// signal gets filtered and then lost.
func TestSkewReportIsSilentWhenTheAgentPredatesTheField(t *testing.T) {
	if _, isSkew := Report("weather", "run-7", "", "bbb"); isSkew {
		t.Fatal("an agent with no schema_hash was reported as skewed")
	}
}

// TestSkewReportIsSilentWhenTheDaemonCannotAnswer guards the direction
// nobody thinks to test. If the daemon's own hash were ever empty, every
// agent on the node would be reported as skewed against nothing.
func TestSkewReportIsSilentWhenTheDaemonCannotAnswer(t *testing.T) {
	if _, isSkew := Report("weather", "run-7", "aaa", ""); isSkew {
		t.Fatal("skew reported against an empty daemon hash")
	}
}

// TestSkewReportSaysTheAgentIsNotRefused pins operator decision 71 into
// the artifact an operator actually reads.
//
// The decision is that the hash is a diagnostic and never an enforcement
// input. An operator meeting this line at 3am must not have to go and
// find out whether their agent was just refused, and a future change
// that starts refusing will have to edit this text - which is the point.
func TestSkewReportSaysTheAgentIsNotRefused(t *testing.T) {
	msg, _ := Report("weather", "run-7", "aaa", "bbb")
	if !strings.Contains(msg, "NOT refused") {
		t.Errorf("report does not say the agent still runs: %s", msg)
	}
}
