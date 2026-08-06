// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agent

import (
	"context"
	"testing"
)

// AN UNSTAMPED BINARY IS NOT A BINARY WITH AN UNKNOWN HASH.
//
// A build that did not come from `agent build` has no provenance, and
// that is a legitimate state - the shipped ADK source compiled directly
// by a developer, for one. What it must not do is put a non-value on
// stdout where a caller comparing digests expects a value. The absence
// is reported on stderr with a non-zero exit, so a script asking a
// binary what it was built from cannot mistake silence for an answer.
func TestUnstampedProvenanceFailsRatherThanPrintingAPlaceholder(t *testing.T) {
	orig := SourceHash
	t.Cleanup(func() { SourceHash = orig })

	SourceHash = ""
	if code := emitProvenance(); code == 0 {
		t.Error("an unstamped binary reported success for --provenance")
	}

	SourceHash = "d5d5e1bd96ecdef6f6920e0c67bf3361b91fdc8fa0a9de8f28e404c1701a6648"
	if code := emitProvenance(); code != 0 {
		t.Errorf("a stamped binary failed --provenance with code %d", code)
	}
}

// TestProvenanceIsADispatchVerb: the value has to be askable. A stamp no
// caller can request is the same defect as one nothing carries, one step
// later.
func TestProvenanceIsADispatchVerb(t *testing.T) {
	orig := SourceHash
	t.Cleanup(func() { SourceHash = orig })
	SourceHash = "d5d5e1bd96ecdef6f6920e0c67bf3361b91fdc8fa0a9de8f28e404c1701a6648"

	resetForTest()
	Register(Spec{ID: "provenance-verb", Type: "service", Start: func(context.Context) error { return nil }})
	t.Cleanup(resetForTest)

	if code := run([]string{"--provenance"}); code != 0 {
		t.Errorf("run(--provenance) returned %d, want 0", code)
	}
}
