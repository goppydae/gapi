// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/goppydae/gapi/core/product"
	"github.com/goppydae/gapi/test/adk"
)

// TestMain declares a product identity for this package's tests, and
// builds the binaries the suite shells out to.
//
// The identity is required, not decorative: the cross-ADK parity tests
// build a real supervisor.New in-process, which reaches cgroups.Setup
// and the config loader, and core/product has no usable default
// (GAPI-DIV-061). The harness that launches a real gapid needs nothing
// from this - that child declares itself - but the in-process half does.
//
// The build is required for the reason GAPI-DIV-097 records: this suite
// runs gapid and gapictl as programs, and a suite whose subject is an
// artifact it does not produce can report ok about a binary that
// predates the change under test. Note that one TestMain governs the
// whole test binary, so this covers package adk as well as adk_test.
func TestMain(m *testing.M) {
	product.Set("gapi")

	cleanup, err := adk.BuildBinaries()
	if err != nil {
		fmt.Fprintf(os.Stderr, "test/adk: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}
