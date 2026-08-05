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

const standaloneAgentSource = `package agent

import "context"

const (
	ID          = "standalone"
	Type        = "service"
	Version     = "1.0.0"
	Description = "Built outside any Go module"
)

func Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
`

// TestBuildGoAgentOutsideAnyModule is GAPI-DIV-092's closing assertion:
// an operator who INSTALLED gapi, and therefore has no checkout for the
// build to borrow a module from, can build a Go agent and run it.
//
// THE DIRECTORY MUST BE OUTSIDE THE REPOSITORY TREE. A test that staged
// its source under the gapi checkout would inherit this module's go.mod
// and pass while proving nothing - which is the trap the entry names.
// t.TempDir() is under the system temp directory, so the assertion below
// is not decorative: it fails the test if that ever stops being true.
func TestBuildGoAgentOutsideAnyModule(t *testing.T) {
	product.Set("gapi")

	src := t.TempDir()
	assertNotInAModule(t, src)

	// The ADK is named explicitly because a test binary's working
	// directory is pkg/cli, where neither the install tier nor the
	// checkout tier resolves. On a real installed system the install tier
	// finds it; the code path under test is the same either way.
	t.Setenv(product.EnvKey("GO_ADK"), testADKSource(t).Dir)

	srcPath := filepath.Join(src, "standalone.go.service")
	if err := os.WriteFile(srcPath, []byte(standaloneAgentSource), 0600); err != nil {
		t.Fatalf("write agent source: %v", err)
	}

	outDir := t.TempDir()
	bin, hash, err := buildGoAgent(srcPath, outDir)
	if err != nil {
		t.Fatalf("build outside a module: %v", err)
	}
	if hash == "" {
		t.Fatal("build produced no source hash")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("built binary is missing: %v", err)
	}

	// Building is half of it. The entry closes on a binary that DESCRIBES,
	// because an agent that compiles and cannot answer discovery is not a
	// runnable agent.
	out, err := exec.Command(bin, "--describe").CombinedOutput()
	if err != nil {
		t.Fatalf("--describe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "standalone") {
		t.Fatalf("--describe did not name the agent:\n%s", out)
	}

	// The staging directory used to be created beside the author's file.
	// Nothing but the source belongs in that directory now.
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read source dir: %v", err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("build left scratch files beside the source: %v", names)
	}
}

// TestResolveGoADKFindsTheCheckoutFromASubdirectory is the regression
// gate for a real CI failure: the checkout tier was written as a bare
// "adk/go" relative to the working directory, which silently assumes the
// caller stands in the repository root. The ADK harness runs gapictl from
// test/adk, so every Go case in the cross-language parity suite failed
// with "cannot locate the Go ADK source".
//
// This test is worth more than it looks. Its own package directory,
// pkg/cli, is TWO LEVELS DOWN, so a resolver that only checks the
// working directory fails it - which is exactly what the unit tests could
// not catch before, because they all named the ADK explicitly.
func TestResolveGoADKFindsTheCheckoutFromASubdirectory(t *testing.T) {
	product.Set("gapi")

	// No override: the checkout tier is the one under test.
	t.Setenv(product.EnvKey("GO_ADK"), "")

	adk, err := resolveGoADK()
	if err != nil {
		t.Fatalf("resolveGoADK from %s: %v", mustGetwd(t), err)
	}
	if _, err := os.Stat(filepath.Join(adk.Dir, "agent", "run.go")); err != nil {
		t.Fatalf("resolved ADK at %s does not hold agent/run.go: %v", adk.Dir, err)
	}
	if adk.GoDirective == "" {
		t.Fatal("resolved ADK carries no go directive")
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}

// TestStagedADKIsCoveredByTheProvenanceHash guards the constraint
// GAPI-DIV-092 said the fix must not walk past: the stamp claims to cover
// what was compiled, and the ADK is compiled in. If the ADK source
// changes, the hash must change - otherwise the stamp certifies an input
// that is not the whole input.
func TestStagedADKIsCoveredByTheProvenanceHash(t *testing.T) {
	product.Set("gapi")

	real := testADKSource(t)

	src := t.TempDir()
	srcPath := filepath.Join(src, "hashed.go.service")
	if err := os.WriteFile(srcPath, []byte(standaloneAgentSource), 0600); err != nil {
		t.Fatalf("write agent source: %v", err)
	}

	t.Setenv(product.EnvKey("GO_ADK"), real.Dir)
	_, first, err := buildGoAgent(srcPath, t.TempDir())
	if err != nil {
		t.Fatalf("first build: %v", err)
	}

	// A byte-for-byte copy of the ADK with one comment added. It compiles
	// identically and produces a different input.
	altered := copyADKWithMarker(t, real)
	t.Setenv(product.EnvKey("GO_ADK"), altered.Dir)
	_, second, err := buildGoAgent(srcPath, t.TempDir())
	if err != nil {
		t.Fatalf("second build: %v", err)
	}

	if first == second {
		t.Fatalf("the provenance hash did not change when the ADK source did (%s)", first)
	}
}

// copyADKWithMarker duplicates the ADK source tree and appends a comment
// to one file.
func copyADKWithMarker(t *testing.T, from goADK) goADK {
	t.Helper()

	root := t.TempDir()
	dst := filepath.Join(root, "agent")
	if err := os.MkdirAll(dst, 0750); err != nil {
		t.Fatalf("create ADK copy: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(from.Dir, "agent"))
	if err != nil {
		t.Fatalf("read ADK source: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(from.Dir, "agent", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if name == "run.go" {
			data = append(data, []byte("\n// provenance marker\n")...)
		}
		if err := os.WriteFile(filepath.Join(dst, name), data, 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	mod := "module " + adkModulePath + "\n\ngo " + from.GoDirective + "\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatalf("write ADK go.mod: %v", err)
	}

	adk, err := loadGoADK(root, "test copy")
	if err != nil {
		t.Fatalf("load ADK copy: %v", err)
	}
	return adk
}

// assertNotInAModule fails if dir has a go.mod at or above it. The
// standalone test means nothing without this.
func assertNotInAModule(t *testing.T, dir string) {
	t.Helper()
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			t.Fatalf("%s is inside a module (go.mod at %s); the standalone case is not being tested", dir, d)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return
		}
		d = parent
	}
}
