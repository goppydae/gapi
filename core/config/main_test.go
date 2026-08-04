// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"os"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// TestMain declares a product identity for this package's tests.
//
// It is required rather than convenient. core/product has no usable
// default (GAPI-DIV-061), so config.Load, EnvKeyFor and AgentSearchPaths
// panic until a binary says who it is - and a test binary is a binary
// with no root command to say it. Setting "gapi" here is what a gapid
// process does at startup; the tests then exercise the same code path a
// real gapid takes.
func TestMain(m *testing.M) {
	product.Set("gapi")
	os.Exit(m.Run())
}
