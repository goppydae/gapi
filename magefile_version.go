// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// GAPI-DIV-126's gate: the value reached the ARTIFACT.
//
// THE FIX IS NOT THE EXIT AND THAT IS THE WHOLE POINT. GAPI-DIV-056's
// closure recorded the residual verbatim - "nothing asserts the -ldflags
// injection happened, so a block reading 'dev' and 'unknown' throughout
// passes" - and it was true for six days, across many green runs, with
// no gate able to see it. A unit test cannot close this: the stamp
// variables are placeholders BY DEFINITION in a test binary, so a test
// of the package proves nothing about what a build produces.
//
// IT MUST RUN FOR BOTH BUILD PATHS. They differ in exactly the way that
// hides this defect - the nix derivation has no VCS data and must stamp,
// mage has it and derives - so a gate covering one certifies the other
// by accident. That is verify-in-the-mode-that-ships applied to a
// version string rather than to a module resolution.

// versionRowSources are the rows cli-contract.md gives a source, and
// therefore the rows that must resolve in every build.
//
// LISTED BY NAME RATHER THAN COUNTED. A gate reporting "5 placeholder
// rows" passes the day someone adds a sixth, and the failure message
// would not say which row to go and look at.
// `Build Tag` carries an `enum` because `dev` is its ANSWER, not its
// absence. The contract's table gives `dev` as the derived value - the
// build channel of a ref that is not a tag - so checking it against the
// placeholder set makes a correct non-release build unpassable. This
// gate found that in its own row list on the first run, which is the
// distinction the enum now encodes: every other row here has no legal
// value that coincides with a placeholder.
var versionRowSources = []versionRow{
	{name: "Commit"},
	{name: "Build Tag", enum: []string{"dev", "release"}},
	{name: "Source Date"},
	{name: "Built By"},
	{name: "Built With Go"},
	{name: "Platform"},
	{name: "Go ADK"},
	{name: "Python ADK"},
}

// versionRow is one contract-sourced row and how it is judged.
type versionRow struct {
	name string

	// enum, when set, replaces the placeholder check with membership.
	enum []string
}

// versionRowExempt is the one row the contract deliberately leaves
// unresolved, and it is spelled out rather than skipped silently.
//
// AN EXEMPTION IS A CLAIM ABOUT SCOPE. This one is bounded by a ledger
// entry that is open: the schema hash reads `unknown` by design until
// GAPI-DIV-127 settles what the value should CONTAIN. When that entry
// closes, this map should empty and the row joins the list above -
// which is a thing to check when closing it, not a thing to discover.
var versionRowExempt = map[string]string{
	"Protobuf Schema Hash": "GAPI-DIV-127 (what the value contains is unsettled)",
}

// placeholders are the two spellings the stamp points use. Both, because
// the block mixes them and a check knowing one would pass on the other.
var placeholders = map[string]bool{"dev": true, "unknown": true, "": true}

// CheckVersionStamps runs the version surface of every shipped artifact
// and fails naming any row that did not resolve.
func CheckVersionStamps() error {
	binaries, err := versionArtifacts()
	if err != nil {
		return err
	}

	var problems []string
	for _, bin := range binaries {
		block, err := runVersion(bin.path)
		if err != nil {
			return fmt.Errorf("version stamps: running %s: %w", bin.label, err)
		}
		problems = append(problems, unresolvedRows(bin.label, block)...)
		fmt.Printf("Version stamps: %s, %d row(s) checked\n",
			bin.label, len(versionRowSources))
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf(
			"%d version row(s) did not resolve in a shipped artifact:\n  %s",
			len(problems), strings.Join(problems, "\n  "))
	}
	return nil
}

type versionArtifact struct {
	label string
	path  string
}

// versionArtifacts builds both paths and returns what each produced.
//
// The mage binaries are built first because they are cheap; the nix
// derivations follow. `nix build` is invoked with the same installable
// CI uses, so the artifact under test is the artifact that ships.
func versionArtifacts() ([]versionArtifact, error) {
	if err := Build(); err != nil {
		return nil, fmt.Errorf("version stamps: mage build: %w", err)
	}
	out := []versionArtifact{
		{"mage bin/gapid", "bin/gapid"},
		{"mage bin/gapictl", "bin/gapictl"},
	}

	// THE NIX HALF IS THE HALF THAT WOULD OTHERWISE BE CERTIFIED BY
	// ACCIDENT, so its absence must be an error rather than a skip. An
	// absent check and a passing check are the same line in a summary.
	for _, spec := range []struct{ installable, bin string }{
		{".#gapictl", "gapictl"},
		{".#default", "gapid"},
	} {
		store, err := nixOutPath(spec.installable)
		if err != nil {
			return nil, err
		}
		out = append(out, versionArtifact{
			label: "nix " + spec.installable + " " + spec.bin,
			path:  filepath.Join(store, "bin", spec.bin),
		})
	}
	return out, nil
}

// nixOutPath builds an installable and returns its store path.
func nixOutPath(installable string) (string, error) {
	cmd := exec.Command("nix", "build", installable, "--no-link", "--print-out-paths")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("version stamps: nix build %s: %w", installable, err)
	}
	lines := strings.Fields(strings.TrimSpace(string(out)))
	if len(lines) == 0 {
		return "", fmt.Errorf("version stamps: nix build %s printed no store path", installable)
	}
	return lines[len(lines)-1], nil
}

// runVersion executes the artifact's version subcommand.
//
// The BINARY, not the package. Reading the wrapper instead of the
// wrapped binary is a trap this repo has already hit: bin/.gapid-wrapped
// is the ELF and bin/gapid is a makeWrapper shell script, so a tool
// reading the latter reports "unrecognized file format" and looks like a
// stripped binary. Executing sidesteps the distinction entirely - a
// wrapper runs.
func runVersion(path string) (string, error) {
	out, err := exec.Command(path, "version").Output() //nolint:gosec // build-declared artifact path
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// unresolvedRows returns one message per contract-sourced row that came
// back a placeholder.
//
// It also fails on a row that is MISSING, because a rename that dropped
// a row would otherwise pass: nothing renders, nothing is a placeholder,
// and the gate reports success about a row that no longer exists.
func unresolvedRows(label, block string) []string {
	values := map[string]string{}
	for _, line := range strings.Split(block, "\n") {
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		values[strings.TrimSpace(name)] = strings.TrimSpace(value)
	}

	var problems []string
	for _, row := range versionRowSources {
		value, present := values[row.name]
		if !present {
			problems = append(problems, fmt.Sprintf(
				"%s: row %q is absent from the block", label, row.name))
			continue
		}
		if len(row.enum) > 0 {
			if !contains(row.enum, value) {
				problems = append(problems, fmt.Sprintf(
					"%s: row %q reads %q, not one of %s",
					label, row.name, value, strings.Join(row.enum, ", ")))
			}
			continue
		}
		if placeholders[value] {
			problems = append(problems, fmt.Sprintf(
				"%s: row %q reads %q", label, row.name, value))
		}
	}
	return problems
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
