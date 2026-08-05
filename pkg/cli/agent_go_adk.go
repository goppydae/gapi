// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goppydae/gapi/core/product"
	"github.com/goppydae/gapi/internal/safeio"
)

// adkModulePath is the module path the shipped ADK source declares, and
// the path the generated main imports the runtime FROM is
// adkModulePath + "/agent".
//
// It is adk/go/AGENT, never adk/go: gopy binds every exported symbol in
// adk/go into the Python ADK's native module, so importing the runtime
// from there would leak it into Python's C bindings.
const adkModulePath = "github.com/goppydae/gapi/adk/go"

// adkImportPath is the Go ADK runtime's import path.
const adkImportPath = adkModulePath + "/agent"

// goADK is a located copy of the Go ADK runtime's source.
//
// Dir holds the .go files of package agent. GoDirective is the `go` line
// to write into the generated module files, taken from wherever the
// source was found rather than from a constant here: a toolchain version
// hard-coded in the CLI is one that drifts from the source it compiles.
type goADK struct {
	Dir         string
	GoDirective string
	Origin      string
}

// resolveGoADK locates the Go ADK runtime source.
//
// THREE TIERS, IN THE SAME ORDER AND FOR THE SAME REASONS AS
// resolvePyRunner (core/supervisor/lifecycle_handlers.go): an explicit
// override, then the install layout, then the checkout. It differs from
// resolvePyRunner in two respects, both deliberate: the last tier is
// CHECKED rather than returned on faith, so a path that does not exist
// produces a clear error here instead of a `go build` failure whose
// message is about modules and says nothing about the ADK; and that tier
// WALKS UP rather than trusting the working directory to be the
// repository root.
//
// GAPI-DIV-092: before this existed, a Go agent could only be built from
// inside a checkout, while the CLI told every operator otherwise.
func resolveGoADK() (goADK, error) {
	var tried []string

	if v := os.Getenv(product.EnvKey("GO_ADK")); v != "" {
		adk, err := loadGoADK(v, product.EnvKey("GO_ADK"))
		if err != nil {
			// An explicit override that does not resolve is an operator
			// error, not a reason to fall through to a different ADK than
			// the one they named.
			return goADK{}, err
		}
		return adk, nil
	}

	// The install layout: <prefix>/bin/<cli> beside
	// <prefix>/share/<product>/go. The product segment is DERIVED, not
	// spelled: goblind links this code, and a path with the kernel
	// vendor's name in it is exactly what core/product's scan exists to
	// keep out of the kernel (GAPI-DIV-061).
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "..", "share", product.Name(), "go")
		if adk, err := loadGoADK(cand, "install tree"); err == nil {
			return adk, nil
		}
		tried = append(tried, cand)
	}

	// The checkout: adk/go at or ABOVE the working directory.
	//
	// Not a bare "adk/go" relative to the cwd, which assumes the caller
	// stands in the repository root - and nothing makes that true. The
	// ADK harness runs gapictl from test/adk, and a developer building an
	// agent has no reason to be at the top of the tree either.
	//
	// This is where resolvePyRunner's third tier is copied in SHAPE but
	// not in weakness. That tier returns a cwd-relative path on faith; it
	// happens to hit in a dev tree and resolved against / on a booted
	// system, which is how GAPI-DIV-077 stayed invisible in a checkout and
	// was fatal in an image. Walking up removes the assumption instead of
	// relying on where the caller happened to stand.
	if wd, err := os.Getwd(); err == nil {
		for d := wd; ; {
			if adk, err := loadGoADK(filepath.Join(d, "adk", "go"), "checkout"); err == nil {
				return adk, nil
			}
			parent := filepath.Dir(d)
			if parent == d {
				break
			}
			d = parent
		}
		tried = append(tried, filepath.Join(wd, "adk", "go")+" (and every parent)")
	}

	return goADK{}, fmt.Errorf(
		"cannot locate the Go ADK source (looked in: %s); set %s to the directory holding agent/*.go",
		strings.Join(tried, ", "), product.EnvKey("GO_ADK"))
}

// loadGoADK validates that dir looks like the ADK source tree and reads
// the go directive out of it.
//
// The validation is deliberately a FILE the package must contain rather
// than the directory merely existing: an empty share/gapi/go, which is
// what a half-finished package install leaves behind, would otherwise be
// accepted and fail later as a compile error in generated code.
func loadGoADK(dir, origin string) (goADK, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return goADK{}, fmt.Errorf("resolve ADK path %s: %w", dir, err)
	}
	if _, err := os.Stat(filepath.Join(abs, "agent", "run.go")); err != nil {
		return goADK{}, fmt.Errorf("%s is not a Go ADK source tree (%s): %w", abs, origin, err)
	}

	directive, err := goDirectiveFrom(abs)
	if err != nil {
		return goADK{}, err
	}
	return goADK{Dir: abs, GoDirective: directive, Origin: origin}, nil
}

// goDirectiveFrom reads the `go` line from the module that owns dir.
//
// The shipped tree carries its own go.mod. A checkout does not - adk/go
// is part of the kernel module - so the search walks up to the go.mod
// that does own it. Either way the directive comes from the module the
// source was written against.
func goDirectiveFrom(dir string) (string, error) {
	for d := dir; ; {
		data, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if v, ok := strings.CutPrefix(strings.TrimSpace(line), "go "); ok {
					return strings.TrimSpace(v), nil
				}
			}
			return "", fmt.Errorf("%s declares no go directive", filepath.Join(d, "go.mod"))
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", fmt.Errorf("no go.mod found at or above %s", dir)
		}
		d = parent
	}
}

// assembleGoAgent writes the author's file, the generated main, a copy of
// the ADK source and the two module files into dir, ready to build.
//
// THE STAGE IS SELF-CONTAINED, and that is the whole design (GAPI-DIV-092).
// It carries its own module, and the ADK arrives as a local `replace`
// target rather than as a version to resolve, so `go build` in it touches
// no module proxy and needs no go.sum. The layout:
//
//	dir/go.mod        module agentbuild.local/agent, replace ADK => ./adk
//	dir/agent.go      the author's file
//	dir/main.go       the generated main
//	dir/adk/go.mod    module github.com/goppydae/gapi/adk/go
//	dir/adk/agent/    the ADK runtime's sources
//
// This replaced staging the package beside the author's source, which
// worked only inside a checkout that could already resolve the kernel.
// There is now ONE path: a developer in the checkout and an operator who
// installed the package build through exactly the same code, differing
// only in where resolveGoADK found the ADK.
func assembleGoAgent(srcPath, dir string, adk goADK) error {
	d, err := scanGoAgent(srcPath)
	if err != nil {
		return err
	}

	// The author's file becomes agent.go. Its name no longer carries the
	// type because the type has already been read out of it.
	if err := os.WriteFile(filepath.Join(dir, "agent.go"), d.sourceAsMain(), 0600); err != nil {
		return fmt.Errorf("write agent source: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), d.generateMain(adkImportPath), 0600); err != nil {
		return fmt.Errorf("write generated main: %w", err)
	}
	return stageADK(dir, adk)
}

// stageADK copies the ADK runtime into dir/adk and writes both go.mod
// files.
//
// The copy is what makes the provenance stamp cover the whole input.
// HashDirectory walks recursively and mixes each file's relative path in,
// so hashing the stage after this hashes the EXACT ADK bytes that get
// compiled - not a version string that names them, which is all a
// generated `require` would have given. That was the constraint
// GAPI-DIV-092 said routes 1 and 2 must not walk past, and copying
// discharges it without any change to the hashing call.
//
// Only *.go is copied. Tests are excluded because they are not compiled
// into the agent, and including them would make the provenance hash
// change for edits that cannot affect the binary.
func stageADK(dir string, adk goADK) error {
	dst := filepath.Join(dir, "adk", "agent")
	if err := os.MkdirAll(dst, 0750); err != nil {
		return fmt.Errorf("create ADK stage: %w", err)
	}

	entries, err := os.ReadDir(filepath.Join(adk.Dir, "agent"))
	if err != nil {
		return fmt.Errorf("read ADK source: %w", err)
	}
	var copied int
	for _, e := range entries {
		// The ADK directory is resolved from an environment variable, so
		// its listing is an untrusted input even though a directory entry
		// cannot contain a separator today. Reducing each name to its base
		// and refusing anything that changes under that leaves no path for
		// an entry to write outside dst.
		name := filepath.Base(e.Name())
		if name != e.Name() {
			return fmt.Errorf("ADK source at %s contains a suspicious entry: %q", adk.Dir, e.Name())
		}
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		data, err := safeio.ReadFileUnder(filepath.Join(adk.Dir, "agent"), filepath.Join(adk.Dir, "agent", name))
		if err != nil {
			return fmt.Errorf("read ADK source %s: %w", name, err)
		}
		out, err := safeio.ResolveUnder(dst, filepath.Join(dst, name))
		if err != nil {
			return fmt.Errorf("stage ADK source %s: %w", name, err)
		}
		if err := os.WriteFile(out, data, 0600); err != nil {
			return fmt.Errorf("stage ADK source %s: %w", name, err)
		}
		copied++
	}
	if copied == 0 {
		return fmt.Errorf("ADK source at %s contains no .go files", adk.Dir)
	}

	adkMod := fmt.Sprintf("module %s\n\ngo %s\n", adkModulePath, adk.GoDirective)
	if err := os.WriteFile(filepath.Join(dir, "adk", "go.mod"), []byte(adkMod), 0600); err != nil {
		return fmt.Errorf("write ADK go.mod: %w", err)
	}

	// v0.0.0 is the conventional placeholder for a requirement that a
	// local replace always satisfies. Nothing resolves it, so no version
	// here can be wrong - and none can be silently right either, which is
	// why the ADK's identity is carried by the hash and not by this line.
	//
	// The stage's own module path names no product. It is a scratch
	// module that exists for one `go build` and is deleted after, so
	// borrowing the vendor's name for it would put that name in the
	// kernel's literals to no reader's benefit.
	stageMod := fmt.Sprintf(
		"module agentbuild.local/agent\n\ngo %s\n\nrequire %s v0.0.0\n\nreplace %s => ./adk\n",
		adk.GoDirective, adkModulePath, adkModulePath)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(stageMod), 0600); err != nil {
		return fmt.Errorf("write stage go.mod: %w", err)
	}
	return nil
}
