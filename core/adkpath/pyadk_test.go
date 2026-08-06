// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adkpath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// pyTree writes a minimal Python ADK tree at dir and returns dir.
func pyTree(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "agent"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent", "runner.py"), []byte("#\n"), 0o600); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	return dir
}

// TestInstallTierFindsTheLayoutThatShips is the tier GAPI-DIV-109 was
// about, and the reason `resolve` takes the executable path.
//
// THE OLD TIER LOOKED UNDER <exedir>/adk/python. Nothing produces that
// layout: the package installs to share/<product>/python, verified
// against the store path. So the tier could never hit, and the daemon's
// Python support rested entirely on an env default in the wrapper -
// which gapictl did not have at all.
func TestInstallTierFindsTheLayoutThatShips(t *testing.T) {
	product.Set("gapi")

	prefix := t.TempDir()
	exe := filepath.Join(prefix, "bin", "gapid")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	pyTree(t, filepath.Join(prefix, "share", product.Name(), "python"))

	// No override, and a working directory with no checkout above it, so
	// only the install tier can answer.
	adk, err := resolve("", exe, t.TempDir())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if adk.Source != "install tree" {
		t.Errorf("source: got %q, want %q", adk.Source, "install tree")
	}
	if _, err := os.Stat(adk.Runner); err != nil {
		t.Errorf("resolved runner does not exist: %v", err)
	}
}

// TestOverrideThatDoesNotResolveIsAnError: an operator who names an ADK
// gets that ADK or an error, never a different one. Falling through
// would answer a question they did not ask.
func TestOverrideThatDoesNotResolveIsAnError(t *testing.T) {
	product.Set("gapi")

	// A valid tree IS reachable by the checkout tier, so a fallthrough
	// would succeed here and the test would catch it.
	wd := t.TempDir()
	pyTree(t, filepath.Join(wd, "adk", "python"))

	_, err := resolve(filepath.Join(t.TempDir(), "nowhere"), "", wd)
	if err == nil {
		t.Fatal("an override naming no ADK resolved anyway; it fell through to another tree")
	}
	if !strings.Contains(err.Error(), product.EnvKey("PY_ADK")) {
		t.Errorf("error does not name the variable the operator set: %v", err)
	}
}

// TestCheckoutTierWalksUp: nothing makes the caller stand in the
// repository root. The ADK harness runs from test/adk and a developer
// building an agent has no reason to be at the top either.
func TestCheckoutTierWalksUp(t *testing.T) {
	product.Set("gapi")

	root := t.TempDir()
	pyTree(t, filepath.Join(root, "adk", "python"))
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}

	adk, err := resolve("", "", deep)
	if err != nil {
		t.Fatalf("resolve from a subdirectory: %v", err)
	}
	if adk.Source != "checkout" {
		t.Errorf("source: got %q, want %q", adk.Source, "checkout")
	}
}

// TestNothingResolvesIsAnErrorNamingTheVariable.
//
// THE TIER THIS REPLACES RETURNED A RELATIVE PATH ON FAITH. "adk/python/
// agent/runner.py" resolves against / under a systemd unit, so the
// failure arrived later as a per-agent describe error naming a path that
// had never existed on that system (GAPI-DIV-077). An unresolvable
// lookup is an error here, where it can say what it looked for.
func TestNothingResolvesIsAnErrorNamingTheVariable(t *testing.T) {
	product.Set("gapi")

	_, err := resolve("", filepath.Join(t.TempDir(), "bin", "gapid"), t.TempDir())
	if err == nil {
		t.Fatal("resolution succeeded with no ADK anywhere")
	}
	if !strings.Contains(err.Error(), product.EnvKey("PY_ADK")) {
		t.Errorf("error does not tell the operator which variable to set: %v", err)
	}
}

// TestRootIsTheSelection: the runner is derived from the tree, not
// configured alongside it. runner.py appends its own parent to sys.path
// and imports gapi.native from there, so a caller that could set the two
// independently could select a runner from one ADK and a binding from
// another.
func TestRootIsTheSelection(t *testing.T) {
	product.Set("gapi")

	dir := pyTree(t, t.TempDir())
	adk, err := resolve(dir, "", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if want := filepath.Join(adk.Root, "agent", "runner.py"); adk.Runner != want {
		t.Errorf("runner is not derived from the root: got %q, want %q", adk.Runner, want)
	}
}
