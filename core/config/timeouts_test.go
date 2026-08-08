// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"testing"
	"time"

	"github.com/goppydae/gapi/core/budget"
)

// TestAgentStartTimeoutIsSlackerThanTheSupervisorAllows is task 6's
// re-statement as an assertion (GAPI-DIV-107).
//
// The relationship, not the number: a harness that allowed LESS than
// the supervisor's declaration ceiling would fail agents the supervisor
// was willing to admit, and the failure would name the harness rather
// than the subject - which is precisely the shape GAPI-DIV-120 is open
// about.
func TestAgentStartTimeoutIsSlackerThanTheSupervisorAllows(t *testing.T) {
	if TestAgentStartTimeout <= budget.Ceiling {
		t.Errorf("TestAgentStartTimeout %s does not exceed the declaration ceiling %s: the harness can now fail an agent the supervisor would admit",
			TestAgentStartTimeout, budget.Ceiling)
	}

	for _, lang := range []string{"go", "python"} {
		if d := budget.DefaultReadinessBudget(lang); TestAgentStartTimeout <= d {
			t.Errorf("TestAgentStartTimeout %s does not exceed %s's derived default %s", TestAgentStartTimeout, lang, d)
		}
	}
}

// TestAgentStartTimeoutIsNotShrunk is a regression guard, and it is
// deliberately a LOWER bound rather than an equality - an equality
// would be a second declaration of the value in timeouts.go.
//
// GAPI-DIV-120 is open against the suite this bounds. Its failure mode
// is a second mode at 2.3x the slowest passing run, not a thin margin,
// so tightening this converts a diagnosable flake into a faster red and
// throws away the diagnosis. GAPI-DIV-107 explicitly declined to shrink
// it; this is the line that makes a later drive-by shrink visible.
func TestAgentStartTimeoutIsNotShrunk(t *testing.T) {
	const asStatedWhenDiv107Landed = 120 * time.Second
	if TestAgentStartTimeout < asStatedWhenDiv107Landed {
		t.Errorf("TestAgentStartTimeout shrank to %s from %s while GAPI-DIV-120 is open against test/adk",
			TestAgentStartTimeout, asStatedWhenDiv107Landed)
	}
}
