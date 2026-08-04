// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk_test

import (
	"os"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// TestMain declares a product identity for this package's tests.
//
// Required, not decorative: the cross-ADK parity tests build a real
// supervisor.New in-process, which reaches cgroups.Setup and the config
// loader, and core/product has no usable default (GAPI-DIV-061). The
// harness that launches a real gapid needs nothing from this - that
// child declares itself - but the in-process half does.
func TestMain(m *testing.M) {
	product.Set("gapi")
	os.Exit(m.Run())
}
