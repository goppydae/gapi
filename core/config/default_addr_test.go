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
	"path/filepath"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// The zero-config transport address must be the PRODUCT'S, not gapi's.
//
// This is GAPI-DIV-071's closing assertion. Load() was product-aware in
// every other respect - the config file, the env prefix, the search
// path - and then set one hardcoded default, so a goblind resolving its
// address with no file and no environment would have bound gapi's port
// while every other surface said goblin. Nothing printed the value, so
// the only symptom was a client dialling a dead address.
//
// The assertion is on goblin because gapi's own default is what the bug
// produced: a test that only checked gapi would pass against the
// hardcoded literal.
func TestLoad_DefaultAddressFollowsTheProduct(t *testing.T) {
	product.Set("goblin")
	defer product.Set("gapi")

	// Isolate from any config.yaml on the search path by naming an EMPTY
	// one. An empty file parses to no keys, so every value comes from a
	// default - which is what this test must measure, rather than
	// whatever the host happens to have in /etc/goblin.
	//
	// Not a nonexistent path: SetConfigFile on a missing file returns a
	// PathError, and only a search that finds nothing yields viper's
	// ConfigFileNotFoundError that Load tolerates.
	empty := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("write empty config: %v", err)
	}
	t.Setenv(product.EnvKey("CONFIG"), empty)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with no config file: %v", err)
	}

	if got, want := cfg.Transport.Address, "127.0.0.1:29000"; got != want {
		t.Errorf("goblind's zero-config control address = %q, want %q "+
			"(a goblin daemon must not default to gapi's port)", got, want)
	}
}
