// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// stagedAgent writes the standard test agent source and returns its path,
// with the ADK resolved from the checkout.
func stagedAgent(t *testing.T, name string) string {
	t.Helper()
	product.Set("gapi")

	real := testADKSource(t)
	t.Setenv(product.EnvKey("GO_ADK"), real.Dir)

	dir := t.TempDir()
	srcPath := filepath.Join(dir, name)
	if err := os.WriteFile(srcPath, []byte(standaloneAgentSource), 0600); err != nil {
		t.Fatalf("write agent source: %v", err)
	}
	return srcPath
}

// TestTheProvenanceHashReachesTheBinary is GAPI-DIV-103's remaining exit:
// the computed source hash must appear in an artifact an operator can
// read.
//
// THIS IS THE ASSERTION A SILENTLY-IGNORED `-X` CANNOT PASS, and that is
// the whole point. The build already passed `-X main.SourceHash=<hash>`
// while the generated main declared no such variable; the Go linker drops
// `-X` for a missing symbol without a word, so the value was computed,
// formatted into a flag, and lost - and the code read as though the
// binary were stamped. Asking the binary is the only question whose
// answer distinguishes the two.
func TestTheProvenanceHashReachesTheBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping toolchain build in short mode")
	}

	srcPath := stagedAgent(t, "stamped.go.service")

	binary, sourceHash, err := buildGoAgent(srcPath, t.TempDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if sourceHash == "" {
		t.Fatal("buildGoAgent returned an empty source hash")
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(binary, "--provenance")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("--provenance failed: %v (stderr: %s)", err, stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got == "" {
		t.Fatal("--provenance printed nothing: the stamp did not reach the binary")
	}
	if got != sourceHash {
		t.Errorf("stamped hash %q does not match the hash of what was compiled %q", got, sourceHash)
	}
}

// TestAgentBuildIsByteReproducible.
//
// MEASURED BEFORE THE FIX: three builds of one tree produced three
// different binaries - and NOT for the reason the ledger recorded. The
// entry blamed an ldflags BuildTime stamp, but nothing declared that
// variable either, so the flag was equally inert. The real cause was the
// stage's own path: os.MkdirTemp gives a fresh directory per build and
// its name was embedded 107 times per binary, so two builds differed in
// SIZE by 24 bytes and in 3,272,435 bytes of content.
//
// A source hash certifying bytes nobody can recompile to the same binary
// certifies less than it appears to, which is why this lands with the
// stamp rather than after it.
func TestAgentBuildIsByteReproducible(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping toolchain build in short mode")
	}

	srcPath := stagedAgent(t, "reproducible.go.service")

	firstBin, firstHash, err := buildGoAgent(srcPath, t.TempDir())
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	secondBin, secondHash, err := buildGoAgent(srcPath, t.TempDir())
	if err != nil {
		t.Fatalf("second build: %v", err)
	}

	if firstHash != secondHash {
		t.Errorf("source hash is not deterministic: %q then %q", firstHash, secondHash)
	}

	a, err := os.ReadFile(firstBin) // #nosec G304 -- path produced by buildGoAgent
	if err != nil {
		t.Fatalf("read first binary: %v", err)
	}
	b, err := os.ReadFile(secondBin) // #nosec G304 -- path produced by buildGoAgent
	if err != nil {
		t.Fatalf("read second binary: %v", err)
	}

	if len(a) != len(b) {
		t.Fatalf("two builds of one tree differ in size: %d vs %d bytes", len(a), len(b))
	}
	if !bytes.Equal(a, b) {
		differing := 0
		for i := range a {
			if a[i] != b[i] {
				differing++
			}
		}
		t.Errorf("two builds of one tree are not byte-identical: %d differing bytes", differing)
	}
}
