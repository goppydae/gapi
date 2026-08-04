// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package product_test

import (
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// These are GAPI-DIV-061's first two gates: an unset identity must FAIL
// at the first read, and a declared one must compose every
// host-namespaced surface from that one value.
//
// The unset case is order-dependent and there is no way around that:
// product identity is process-global, and any test that declares one
// makes "unset" unreachable for every test after it. Go runs a file's
// tests in source order, so the unset assertion is FIRST here and
// refuses to skip - a skip would read as a pass, and this package must
// have no TestMain for the same reason. Unsetting in a cleanup would be
// a setter with no production counterpart, exercising a path no binary
// has.

// A read before any Set must panic, not fall back to a default.
//
// This is the gate that would have caught the shape it was written
// against: a settable prefix with a usable default silently gives an
// embedder the kernel's namespace, so a goblind that never declared
// itself would read GAPI_* and search /etc/gapi while reporting no
// error at all.
func TestName_PanicsBeforeAnyIdentityIsSet(t *testing.T) {
	if product.IsSet() {
		t.Fatal("an identity was already declared before this test ran, so the " +
			"no-default guarantee is untested in this binary. Something added a " +
			"TestMain or an init() to package product_test; this assertion must " +
			"run first or not at all.")
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("product.Name() returned instead of panicking with no identity set - " +
				"an undeclared binary would silently adopt a default namespace")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "read before it was set") {
			t.Fatalf("panicked, but not with the diagnostic an operator needs: %v", r)
		}
	}()
	_ = product.Name()
}

// Every host-namespaced surface derives from the one identity. The
// assertion is on goblin rather than gapi deliberately: gapi's values
// are unchanged by this entry, so a test that only checked gapi would
// pass against hardcoded literals.
func TestSurfacesDeriveFromTheIdentity(t *testing.T) {
	product.Set("goblin")

	for _, c := range []struct {
		surface string
		got     string
		want    string
	}{
		{"env prefix", product.EnvPrefix(), "GOBLIN"},
		{"config env name", product.EnvKey("CONFIG"), "GOBLIN_CONFIG"},
		{"agent path env name", product.EnvKey("AGENT_PATH"), "GOBLIN_AGENT_PATH"},
		{"daemon name", product.Daemon(), "goblind"},
		{"config dir", product.ConfigDir(), "/etc/goblin"},
		{"default log path", product.DefaultLogPath(), "/var/log/goblin/goblin.log"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.surface, c.got, c.want)
		}
	}

	// And back, so a later test in this binary is not left on goblin.
	product.Set("gapi")
	if got := product.Daemon(); got != "gapid" {
		t.Errorf("after re-Set, Daemon() = %q, want %q", got, "gapid")
	}
}

// A malformed name fails at the setter rather than propagating into
// paths and variable names. The constructors take four adjacent strings
// (product, binary, version, short); this is what makes a transposition
// loud instead of producing /etc/GAPI Supervisor Daemon.
func TestSet_RejectsAnythingThatIsNotAProductName(t *testing.T) {
	for _, bad := range []string{"", "GAPI", "gapi d", "0.1.0-proto2d", "Supervision kernel daemon", "gapi/agents"} {
		t.Run(bad, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("product.Set(%q) was accepted; a transposed argument would "+
						"reach the environment prefix and the config path", bad)
				}
			}()
			product.Set(bad)
		})
	}
}

// EnvKey refuses a suffix core/product does not declare, so a new direct
// read cannot appear without entering the registry the documented-names
// gate enumerates.
func TestEnvKey_RefusesAnUndeclaredName(t *testing.T) {
	product.Set("gapi")
	defer func() {
		if recover() == nil {
			t.Fatal("product.EnvKey accepted an undeclared suffix - a reader could then " +
				"compose a name that core/config's documented-names gate cannot see")
		}
	}()
	_ = product.EnvKey("NOT_A_DECLARED_NAME")
}
