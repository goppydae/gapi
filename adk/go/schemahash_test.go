// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk_test

import (
	"testing"

	adk "github.com/goppydae/gapi/adk/go"
	"github.com/goppydae/gapi/core/schemahash"
)

// TestSchemaHashIsSetWithoutAnyCaller is the whole point of computing at
// init.
//
// GAPI-DIV-127's second defect: a Go agent reported the empty string
// because nothing called SetSchemaHash, while the Python runner did - and
// a comparison only one language answers reads as coverage while
// providing none. Adding a Go call site would have fixed the symptom and
// kept the mechanism; computing at init removes the opt-in from both
// languages at once, because the Python ADK is this package through gopy.
func TestSchemaHashIsSetWithoutAnyCaller(t *testing.T) {
	got := adk.SchemaHash()
	if got == "" {
		t.Fatal("a Go agent reports no schema hash without an explicit setter call")
	}
	if want := schemahash.Contract(); got != want {
		t.Fatalf("ADK hash %q does not match the linked contract %q", got, want)
	}
}

// TestSetSchemaHashStillOverrides keeps the seam the mismatch fixtures
// need. Forcing a skew is how GAPI-DIV-127 closes - a test that only
// checks matching hashes passes when both sides compute nothing - so the
// setter has to remain able to lie.
func TestSetSchemaHashStillOverrides(t *testing.T) {
	original := adk.SchemaHash()
	t.Cleanup(func() { adk.SetSchemaHash(original) })

	adk.SetSchemaHash("forced-mismatch")
	if got := adk.SchemaHash(); got != "forced-mismatch" {
		t.Fatalf("SetSchemaHash did not override: got %q", got)
	}
}
