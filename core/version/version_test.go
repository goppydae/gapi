// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package version

import (
	"runtime/debug"
	"strings"
	"testing"
)

// stubBuildInfo installs a fake reader and restores the real one. The
// seam exists because it cannot be exercised any other way: a TEST
// binary carries no dependency information at all. Measured in goblin's
// internal/cli, which links 38 gapi packages and still reports
// len(BuildInfo.Deps) == 0, while the shipped binary records
// "dep github.com/goppydae/gapi v0.1.0-proto2f". Without the seam the
// dep path would be permanently unreachable from a test, and a gate
// that cannot run reports success.
func stubBuildInfo(t *testing.T, bi *debug.BuildInfo, ok bool) {
	t.Helper()
	prev := buildInfoReader
	buildInfoReader = func() (*debug.BuildInfo, bool) { return bi, ok }
	t.Cleanup(func() { buildInfoReader = prev })
}

// stubStamps sets the linker-injected globals and restores them.
func stubStamps(t *testing.T, gapi, goADK, pyADK string) {
	t.Helper()
	pg, pgo, ppy := GAPIVersion, GoADKVersion, PythonADKVersion
	GAPIVersion, GoADKVersion, PythonADKVersion = gapi, goADK, pyADK
	t.Cleanup(func() { GAPIVersion, GoADKVersion, PythonADKVersion = pg, pgo, ppy })
}

func depInfo(path, version string) *debug.BuildInfo {
	return &debug.BuildInfo{
		Main: debug.Module{Path: "github.com/goppydae/goblin", Version: "(devel)"},
		Deps: []*debug.Module{{Path: path, Version: version}},
	}
}

// TestKernelVersion_PrefersStampedVersion keeps gapi's own builds
// authoritative: gapi stamps core/version.GAPIVersion, and that must win
// over anything derived, so a deliberate stamp is never second-guessed.
func TestKernelVersion_PrefersStampedVersion(t *testing.T) {
	stubStamps(t, "1.2.3", "dev", "dev")
	stubBuildInfo(t, depInfo("github.com/goppydae/gapi", "v9.9.9"), true)

	if got := KernelVersion(); got != "1.2.3" {
		t.Fatalf("want the stamped 1.2.3, got %q", got)
	}
}

// TestKernelVersion_FromDependency is the defect this entry exists for:
// goblin embeds gapi and stamps only its own version, so the kernel row
// must come from the module graph.
func TestKernelVersion_FromDependency(t *testing.T) {
	stubStamps(t, "dev", "dev", "dev")
	stubBuildInfo(t, depInfo("github.com/goppydae/gapi", "v0.1.0-proto2f"), true)

	if got := KernelVersion(); got != "0.1.0-proto2f" {
		t.Fatalf("want 0.1.0-proto2f from the dependency graph, got %q", got)
	}
}

// TestKernelVersion_FromMainModule covers gapi's own binaries in an
// unstamped build, where gapi is the MAIN module and never a dep.
func TestKernelVersion_FromMainModule(t *testing.T) {
	stubStamps(t, "dev", "dev", "dev")
	stubBuildInfo(t, &debug.BuildInfo{
		Main: debug.Module{Path: "github.com/goppydae/gapi", Version: "v0.1.0-proto2f"},
	}, true)

	if got := KernelVersion(); got != "0.1.0-proto2f" {
		t.Fatalf("want 0.1.0-proto2f from the main module, got %q", got)
	}
}

// TestKernelVersion_RejectsPlaceholders is the discriminating case.
//
// A workspace build resolves gapi from the sibling checkout and the
// toolchain records "(devel)" - which is HONEST, and is not a version.
// An implementation that returns whatever the dep says would report
// "(devel)" as the embedded kernel, which is a different lie from "dev"
// rather than a fix. Empty Deps is the measured condition inside every
// test binary, so accepting it would make the resolution look like it
// works while reporting nothing.
func TestKernelVersion_RejectsPlaceholders(t *testing.T) {
	cases := []struct {
		name string
		bi   *debug.BuildInfo
		ok   bool
	}{
		{"devel dependency", depInfo("github.com/goppydae/gapi", "(devel)"), true},
		{"empty dependency version", depInfo("github.com/goppydae/gapi", ""), true},
		{"gapi absent from deps", depInfo("github.com/goppydae/goblin", "v1.0.0"), true},
		{"no deps at all", &debug.BuildInfo{Deps: nil}, true},
		{"build info unavailable", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubStamps(t, "dev", "dev", "dev")
			stubBuildInfo(t, tc.bi, tc.ok)

			if got := KernelVersion(); got != "dev" {
				t.Fatalf("want dev when no concrete version is available, got %q", got)
			}
		})
	}
}

// TestKernelVersion_StripsTheModuleVersionPrefix is the case the
// fixtures originally hid.
//
// Go module versions are canonically "v"-prefixed and the toolchain
// records them that way in build info, while the VERSION file and every
// stamped path spell the version WITHOUT it. cli-contract.md fixes the
// user-facing spelling at "Runtime Core: 0.1.0-proto2b", no prefix - so
// a derived value that keeps the "v" makes goblind and gapictl print the
// same fact two different ways, and puts goblind in violation of the
// contract.
//
// This went unnoticed because the fixtures here were written with bare
// versions like "0.1.0-proto2f", which build info never produces. A test
// whose fixture disagrees with reality passes against both the correct
// implementation and the broken one; only running a real binary exposed
// it. All fixtures in this file now carry the prefix.
func TestKernelVersion_StripsTheModuleVersionPrefix(t *testing.T) {
	cases := []struct {
		name string
		bi   *debug.BuildInfo
		want string
	}{
		{
			"dependency",
			depInfo("github.com/goppydae/gapi", "v0.1.0-proto2g"),
			"0.1.0-proto2g",
		},
		{
			"main module",
			&debug.BuildInfo{Main: debug.Module{
				Path:    "github.com/goppydae/gapi",
				Version: "v0.1.0-proto2g",
			}},
			"0.1.0-proto2g",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubStamps(t, "dev", "dev", "dev")
			stubBuildInfo(t, tc.bi, true)

			if got := KernelVersion(); got != tc.want {
				t.Fatalf("KernelVersion() = %q, want %q - the module version prefix must not reach the version block", got, tc.want)
			}
		})
	}
}

// TestKernelVersion_MatchesOnSegmentBoundary guards the module lookup,
// which identifies the kernel as "the module containing this package"
// rather than by name - the kernel does not spell its own name in
// literals (GAPI-DIV-061), and a derived match also survives a rename.
//
// A raw string prefix is not a module match: a module whose path merely
// starts with the same characters would otherwise be read as the one
// that was linked, and would supply a wrong version rather than none.
func TestKernelVersion_MatchesOnSegmentBoundary(t *testing.T) {
	stubStamps(t, "dev", "dev", "dev")
	// Shares a character prefix with this package's import path but is a
	// different module.
	stubBuildInfo(t, depInfo("github.com/goppydae/gap", "v7.7.7"), true)

	if got := KernelVersion(); got != "dev" {
		t.Fatalf("a character-prefix module was accepted as the kernel: got %q", got)
	}
}

// TestKernelVersion_LongestContainingModuleWins covers the case where
// more than one module path contains this package. Only the innermost is
// the module actually linked; a parent path would report some other
// module's version as the kernel's.
func TestKernelVersion_LongestContainingModuleWins(t *testing.T) {
	stubStamps(t, "dev", "dev", "dev")
	self := selfPackagePath()
	outer := self[:strings.LastIndex(self, "/")]   // .../core
	inner := outer[:strings.LastIndex(outer, "/")] // the real module
	stubBuildInfo(t, &debug.BuildInfo{
		Main: debug.Module{Path: "github.com/goppydae/goblin", Version: "(devel)"},
		Deps: []*debug.Module{
			{Path: inner, Version: "v1.1.1"},
			{Path: outer, Version: "v2.2.2"},
		},
	}, true)

	if got := KernelVersion(); got != "2.2.2" {
		t.Fatalf("want the innermost containing module's version 2.2.2, got %q", got)
	}
}

// TestSummary_RuntimeCoreReportsResolvedKernel drives the row itself,
// not just the resolver. The existing goblin guards assert only that the
// LABEL appears, so they pass against any value; this asserts the value.
func TestSummary_RuntimeCoreReportsResolvedKernel(t *testing.T) {
	stubStamps(t, "dev", "dev", "dev")
	stubBuildInfo(t, depInfo("github.com/goppydae/gapi", "v0.1.0-proto2f"), true)
	SetBinaryNameAndVersion("goblind", "0.1.0-proto2f")
	t.Cleanup(func() { SetBinaryNameAndVersion("", "") })

	got := Summary()

	if !strings.Contains(got, "Runtime Core:") {
		t.Fatalf("version block omits the kernel row:\n%s", got)
	}
	if strings.Contains(got, "Runtime Core:         dev") {
		t.Fatalf("kernel row still reports dev:\n%s", got)
	}
	if !strings.Contains(got, "Runtime Core:         0.1.0-proto2f") {
		t.Fatalf("kernel row does not carry the resolved version:\n%s", got)
	}
}

// TestSummary_ADKRowsFallBackToKernel encodes current release policy:
// the ADKs ship inside gapi and are in lockstep, so an unstamped ADK row
// is not unknown - it is the kernel's version, and printing dev throws
// away a value we hold.
func TestSummary_ADKRowsFallBackToKernel(t *testing.T) {
	stubStamps(t, "dev", "dev", "dev")
	stubBuildInfo(t, depInfo("github.com/goppydae/gapi", "v0.1.0-proto2f"), true)

	got := Summary()

	// Assert the ADK rows specifically rather than scanning the whole
	// block for "dev". Build Tag is still an unstamped placeholder and is
	// a recorded residual of GAPI-DIV-066, so a blanket search fails on
	// something this test is not about - and would keep failing after the
	// behaviour under test was correct.
	for _, label := range []string{"Go ADK", "Python ADK"} {
		row := rowValue(t, got, label)
		if row == devVersion {
			t.Errorf("%s still reports %q while the kernel is known:\n%s", label, devVersion, got)
		}
		if row != "0.1.0-proto2f" {
			t.Errorf("%s = %q, want the kernel version 0.1.0-proto2f", label, row)
		}
	}
}

// rowValue pulls one row's value out of a rendered block, so assertions
// name a field rather than pattern-matching the whole page.
func rowValue(t *testing.T, block, label string) string {
	t.Helper()
	for _, line := range strings.Split(block, "\n") {
		name, value, found := strings.Cut(line, ":")
		if found && strings.TrimSpace(name) == label {
			return strings.TrimSpace(value)
		}
	}
	t.Fatalf("version block has no %q row:\n%s", label, block)
	return ""
}

// TestSummary_ADKRowsKeepExplicitStamp protects the operator's design
// that an ADK MAY version independently for a hotfix. The fallback must
// be a fallback, not a rewrite: an explicit stamp has to survive it, or
// independent versioning becomes impossible without changing this code
// again.
func TestSummary_ADKRowsKeepExplicitStamp(t *testing.T) {
	stubStamps(t, "dev", "0.1.0-proto2f.1", "dev")
	stubBuildInfo(t, depInfo("github.com/goppydae/gapi", "v0.1.0-proto2f"), true)

	got := Summary()

	if !strings.Contains(got, "0.1.0-proto2f.1") {
		t.Fatalf("an explicitly stamped ADK version was overwritten by the fallback:\n%s", got)
	}
}
