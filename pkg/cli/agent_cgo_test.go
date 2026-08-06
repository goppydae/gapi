// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// THE SUITE WAS BLIND TO THE ONE CONFIGURATION THAT MATTERED.
//
// GAPI-DIV-105: CGO_ENABLED defaults to 1 and adk/go/agent imports `net`,
// so the staged build pulled in runtime/cgo and died with
// `cgo: C compiler "gcc" not found` on a host with a Go toolchain and no
// gcc. Every existing test that builds an agent runs inside `nix
// develop`, where a C compiler is always present - so the entire suite
// agreed with the defect.
//
// These tests SCRUB THE COMPILER FROM PATH rather than reasoning about
// what would happen without one, because reasoning is exactly what let
// this sit: the entry itself was inferred from the environment before it
// was measured against one.

// pathWithoutCC replaces PATH with a directory holding a symlink to `go`
// and nothing else, so no C compiler is reachable.
//
// CONSTRUCTED, NOT SUBTRACTED. Removing the directories that contain a
// compiler was the obvious approach and it did not work: exec.LookPath
// returns only the FIRST match, this dev shell carries more than one
// directory with a cc in it, and the scrub left gcc resolving - which the
// guard caught, so the tests SKIPPED rather than passing. A skip on the
// only assertion that matters is a silent pass, and subtracting an
// unknown set is how you get one. Building the set is exact.
func pathWithoutCC(t *testing.T) {
	t.Helper()

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("no go toolchain on PATH: %v", err)
	}

	dir := t.TempDir()
	if err := os.Symlink(goBin, filepath.Join(dir, "go")); err != nil {
		t.Fatalf("link go into the minimal PATH: %v", err)
	}
	t.Setenv("PATH", dir)

	// Assert the constructed environment IS the one being claimed. The
	// whole point is a host with a Go toolchain and no C compiler; a test
	// that assumed it had built that would be the same mistake one layer
	// up.
	for _, cc := range []string{"gcc", "cc", "clang"} {
		if p, lerr := exec.LookPath(cc); lerr == nil {
			t.Fatalf("minimal PATH still resolves %s at %s", cc, p)
		}
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("minimal PATH lost the go toolchain: %v", err)
	}
}

// TestAgentBuildsWithNoCCompiler is the entry's gate, in its first form:
// the build SUCCEEDS on a host with no C compiler.
func TestAgentBuildsWithNoCCompiler(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping build test in short mode")
	}
	product.Set("gapi")

	adk := testADKSource(t)
	t.Setenv(product.EnvKey("GO_ADK"), adk.Dir)

	src := filepath.Join(t.TempDir(), "nocc.go.service")
	if err := os.WriteFile(src, []byte(standaloneAgentSource), 0600); err != nil {
		t.Fatalf("write agent source: %v", err)
	}

	pathWithoutCC(t)
	cgoFlag = nil
	t.Cleanup(func() { cgoFlag = nil })

	bin, _, err := buildGoAgent(src, t.TempDir())
	if err != nil {
		t.Fatalf("agent build failed with no C compiler on PATH: %v", err)
	}

	// It must also RUN. A binary that links but cannot describe itself
	// would satisfy the letter of the gate and none of its intent.
	out, err := exec.Command(bin, "--describe").Output()
	if err != nil {
		t.Fatalf("--describe on the cgo-free binary: %v", err)
	}
	if !strings.Contains(string(out), "\"describe\"") {
		t.Fatalf("--describe produced no descriptor: %s", out)
	}
}

// TestCGOFlagPreflightsTheCompiler is the gate's second form: if the
// operator asks for cgo back, the missing compiler is named HERE, before
// the toolchain names it obscurely.
func TestCGOFlagPreflightsTheCompiler(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping build test in short mode")
	}
	product.Set("gapi")

	adk := testADKSource(t)
	t.Setenv(product.EnvKey("GO_ADK"), adk.Dir)

	src := filepath.Join(t.TempDir(), "wantcgo.go.service")
	if err := os.WriteFile(src, []byte(standaloneAgentSource), 0600); err != nil {
		t.Fatalf("write agent source: %v", err)
	}

	pathWithoutCC(t)
	on := true
	cgoFlag = &on
	t.Cleanup(func() { cgoFlag = nil })

	_, _, err := buildGoAgent(src, t.TempDir())
	if err == nil {
		t.Fatal("--cgo with no C compiler on PATH did not fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "C compiler") {
		t.Fatalf("failure does not name the C compiler: %v", err)
	}
	if !strings.Contains(msg, "--cgo") {
		t.Fatalf("failure does not say how to get out of it: %v", err)
	}
}

// TestStagedCGOPrecedence pins the three levels. It is a unit test on the
// resolver rather than three more builds: what is being asserted is the
// ORDER, and a build would take a minute to say the same thing.
func TestStagedCGOPrecedence(t *testing.T) {
	// Snapshot and restore rather than leaving CGO_ENABLED unset behind
	// us: t.Setenv restores itself, os.Unsetenv does not, and the rest of
	// this package builds agents.
	prev, had := os.LookupEnv("CGO_ENABLED")
	t.Cleanup(func() {
		cgoFlag = nil
		if had {
			if err := os.Setenv("CGO_ENABLED", prev); err != nil {
				t.Errorf("restore CGO_ENABLED: %v", err)
			}
			return
		}
		if err := os.Unsetenv("CGO_ENABLED"); err != nil {
			t.Errorf("unset CGO_ENABLED: %v", err)
		}
	})

	t.Run("default is off", func(t *testing.T) {
		cgoFlag = nil
		if err := os.Unsetenv("CGO_ENABLED"); err != nil {
			t.Fatalf("unset CGO_ENABLED: %v", err)
		}
		if got := stagedCGOEnv(); got != "CGO_ENABLED=0" {
			t.Fatalf("default = %q, want CGO_ENABLED=0", got)
		}
	})

	// An empty value is what an unset shell variable expands to, not a
	// choice. Passing it through would hand the toolchain "CGO_ENABLED="
	// and turn a defaulted build into a failed one.
	t.Run("empty is treated as unset", func(t *testing.T) {
		cgoFlag = nil
		t.Setenv("CGO_ENABLED", "")
		if got := stagedCGOEnv(); got != "CGO_ENABLED=0" {
			t.Fatalf("empty CGO_ENABLED = %q, want CGO_ENABLED=0", got)
		}
	})

	t.Run("environment beats the default", func(t *testing.T) {
		cgoFlag = nil
		t.Setenv("CGO_ENABLED", "1")
		if got := stagedCGOEnv(); got != "CGO_ENABLED=1" {
			t.Fatalf("with CGO_ENABLED=1 in the environment = %q", got)
		}
	})

	t.Run("flag beats the environment", func(t *testing.T) {
		t.Setenv("CGO_ENABLED", "1")
		off := false
		cgoFlag = &off
		if got := stagedCGOEnv(); got != "CGO_ENABLED=0" {
			t.Fatalf("--cgo=false against CGO_ENABLED=1 = %q, want CGO_ENABLED=0", got)
		}

		on := true
		cgoFlag = &on
		t.Setenv("CGO_ENABLED", "0")
		if got := stagedCGOEnv(); got != "CGO_ENABLED=1" {
			t.Fatalf("--cgo against CGO_ENABLED=0 = %q, want CGO_ENABLED=1", got)
		}
	})
}
