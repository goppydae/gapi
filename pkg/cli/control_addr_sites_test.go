// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/goppydae/gapi/core/product"
	"github.com/goppydae/gapi/pkg/cli"
)

// GAPI-DIV-112. The control default was declared FOUR times: the table in
// core/product that GAPI-DIV-071 built to be its single source, the NixOS
// module's option default, the --listen-addr help string, and the shipped
// example config. Nothing kept them in step, and the first one to drift
// presents as a client dialling a dead port - GAPI-DIV-070's exact
// symptom, whose error message named neither address.
//
// Two of those literals are gone: the help string now derives from
// core/product, and the example was a THIRD spelling of the host besides.
// The two that remain cannot derive, because Nix and YAML cannot call Go
// without making evaluation depend on a build - which would put the flake
// check that caught PR #110's defect behind a compile. So they are
// ASSERTED equal here instead. That is a weaker guarantee than derivation
// and it is the reason this test exists rather than a comment saying they
// match.
//
// This is deliberately a test that reads FILES. It is the only form that
// can see a literal in a language the compiler never touches.

// repoRoot is two levels up from pkg/cli. Resolved rather than assumed:
// a wrong path would make every check below silently vacuous, which is
// the failure mode this file exists to prevent elsewhere.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	// Prove the anchor before trusting anything read through it.
	for _, marker := range []string{"go.mod", "nix/module.nix", "config/config.yaml"} {
		if _, err := os.Stat(filepath.Join(root, marker)); err != nil {
			t.Fatalf("repo root %q does not look like the gapi tree (%s missing): %v", root, marker, err)
		}
	}
	return root
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

// mustFind applies re to src and returns the first capture group, failing
// the test when the pattern does not match. A pattern that stops matching
// means the file was restructured, and that must fail loudly rather than
// skip: a regex quietly matching nothing is exactly how a gate stops
// gating.
func mustFind(t *testing.T, re *regexp.Regexp, src, what string) string {
	t.Helper()
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("could not locate %s; the file was restructured and this "+
			"assertion is no longer reading what it names", what)
	}
	return m[1]
}

// TestControlAddrSitesAgree is the assertion four independent literals
// could not pass.
func TestControlAddrSitesAgree(t *testing.T) {
	root := repoRoot(t)

	// NewGapidRoot calls product.Set("gapi") through NewDaemonRoot, so
	// build the root FIRST and read the identity from it afterwards.
	// Reading the want value before constructing the tree would assert
	// against whatever product a previously-run test left behind.
	gapidRoot, _, _ := cli.NewGapidRoot(func(*cobra.Command, []string) error { return nil })
	want := product.DefaultControlAddr()

	t.Run("flag help", func(t *testing.T) {
		startCmd, _, err := gapidRoot.Find([]string{"start"})
		if err != nil {
			t.Fatalf("gapid root has no start subcommand: %v", err)
		}
		f := startCmd.Flags().Lookup("listen-addr")
		if f == nil {
			t.Fatal("gapid start has no --listen-addr flag")
		}
		if !strings.Contains(f.Usage, want) {
			t.Errorf("--listen-addr help = %q, which does not name the default %q", f.Usage, want)
		}
	})

	t.Run("nixos module default", func(t *testing.T) {
		src := readFile(t, filepath.Join(root, "nix", "module.nix"))
		re := regexp.MustCompile(`(?s)listenAddress\s*=\s*mkOption\s*\{.*?default\s*=\s*"([^"]+)"`)
		got := mustFind(t, re, src, "the listenAddress option default in nix/module.nix")
		if got != want {
			t.Errorf("nix/module.nix listenAddress default = %q, want %q "+
				"(core/product.DefaultControlAddr). A NixOS consumer's "+
				"default would come from the module rather than the kernel.", got, want)
		}
	})

	t.Run("shipped example", func(t *testing.T) {
		src := readFile(t, filepath.Join(root, "config", "config.yaml"))
		re := regexp.MustCompile(`(?m)^\s+address:\s*"([^"]+)"`)
		got := mustFind(t, re, src, "transport.address in config/config.yaml")
		if got != want {
			t.Errorf("config/config.yaml transport.address = %q, want %q. "+
				"An example that contradicts the default teaches the wrong "+
				"thing to whoever copies it.", got, want)
		}
	})
}
