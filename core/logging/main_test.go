// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package logging_test

import (
	"os"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// TestMain declares a product identity for this package's tests: the
// kmsg tag is composed from it, and core/product has no usable default
// (GAPI-DIV-061).
func TestMain(m *testing.M) {
	product.Set("gapi")
	os.Exit(m.Run())
}
