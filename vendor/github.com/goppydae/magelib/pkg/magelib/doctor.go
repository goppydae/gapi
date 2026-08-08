// Copyright (c) 2026 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package magelib

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// DoctorConfig carries the per-repo facts the doctor checks against. The
// checks themselves are ecosystem policy and live here; the differences
// between repos are data, not forks.
type DoctorConfig struct {
	// ReplaceTargets are the relative paths every replace directive points
	// at; each must exist on disk as a sibling checkout (workspace shape).
	ReplaceTargets []string
	// ProtoPlugins are the codegen plugins that must be reachable on PATH
	// from the pinned store (codegen reachability).
	ProtoPlugins []string
	// GopyVersion is the pinned FFI codegen tool version (e.g. "v0.4.10").
	// Empty means the repo has no FFI seam and the check passes with a note.
	GopyVersion string
	// RequiredEnv are environment variables the targets assume are set.
	RequiredEnv []string
	// SharedTools extends the hermetic-resolution tool list beyond the base
	// compiler set (linters, buf, docs toolchain, ...).
	SharedTools []string
	// DeclaredTools is the WHOLE hermetic-resolution tool list, inheriting
	// nothing. It is the DoctorConfig counterpart to CheckHermeticTools.
	//
	// WITHOUT IT THE DOCTOR STILL FORCED A COMPILER SET ON A REPO THAT
	// COMPILES NOTHING (MAGELIB-DIV-015). CheckHermeticTools gave the LINT
	// path a way to declare its own set, and this check kept prepending
	// baseTools regardless - so goppydae-docs dropped gcc and protobuf
	// from its flake, watched `mage lint` pass, and got `FAIL
	// hermetic-resolution  protoc not found. Run 'nix develop'` from
	// `mage doctor` in the same shell. Half a mechanism, which is the
	// shape this repository's ledger keeps catching.
	//
	// Mutually exclusive with SharedTools, and setting both is a config
	// ERROR rather than a precedence rule: SharedTools EXTENDS the base
	// set and DeclaredTools REPLACES it, so a caller setting both has
	// stated a contradiction about which it meant. Same call the file-
	// length gate makes for a path claimed as both a waiver and a skip.
	DeclaredTools []string
	// Out receives the per-check report lines. Nil means os.Stdout, which
	// is what every Magefile caller wants; tests set it to capture the
	// output contract.
	Out io.Writer
}

// Doctor runs the six field-guide checks in order. Output contract: one line
// per check -- name, PASS or FAIL, and a one-line remedy on FAIL; non-nil
// error (nonzero exit) if anything failed. It builds nothing, installs
// nothing, mutates nothing (operator field guide section 2).
func Doctor(cfg DoctorConfig) error {
	out := cfg.Out
	if out == nil {
		out = os.Stdout
	}
	failed := 0
	// The report lines are the terminus: a write failure here has nowhere
	// left to be reported, so the count is discarded deliberately.
	report := func(name string, err error, passNote string) {
		if err != nil {
			failed++
			_, _ = fmt.Fprintf(out, "FAIL %-22s %v\n", name, err)
			return
		}
		if passNote != "" {
			_, _ = fmt.Fprintf(out, "PASS %-22s %s\n", name, passNote)
			return
		}
		_, _ = fmt.Fprintf(out, "PASS %s\n", name)
	}

	// 1. Hermetic resolution: every ecosystem tool from the pinned store.
	report("hermetic-resolution", checkHermeticResolution(cfg), "")

	// 2. Pin agreement: shell go version matches the toolchain directive.
	report("pin-agreement", CheckToolchainPin("go.mod"), "")

	// 3. Workspace shape: every replace target exists as a sibling on disk.
	report("workspace-shape", checkWorkspaceShape(cfg.ReplaceTargets), "")

	// 4. Codegen reachability: proto plugins on PATH at pinned store paths.
	report("codegen-reachability", checkCodegen(cfg.ProtoPlugins), "")

	// 5. Binding toolchain: gopy present at its pinned version, not @latest.
	if cfg.GopyVersion == "" {
		report("binding-toolchain", nil, "no FFI codegen tool required")
	} else {
		report("binding-toolchain", checkGopy(cfg.GopyVersion), "")
	}

	// 6. Required environment: variables the targets assume are set.
	report("required-environment", checkEnv(cfg.RequiredEnv), "")

	if failed > 0 {
		return fmt.Errorf("doctor: %d check(s) failed", failed)
	}
	return nil
}

func checkHermeticResolution(cfg DoctorConfig) error {
	tools, err := hermeticResolutionTools(cfg)
	if err != nil {
		return err
	}
	for _, tool := range tools {
		if err := checkStorePath(tool); err != nil {
			return err
		}
	}
	return nil
}

// hermeticResolutionTools decides which list the check runs over.
//
// The empty-DeclaredTools case falls through to the additive form rather
// than checking nothing, which matters because a check over an empty
// tool set passes without resolving anything - the failure
// CheckHermeticTools rejects outright. Here the fall-through is the
// safer default: a repo that sets neither field gets the base compiler
// set, which is what every caller before MAGELIB-DIV-015 got.
func hermeticResolutionTools(cfg DoctorConfig) ([]string, error) {
	if len(cfg.DeclaredTools) == 0 {
		return append(append([]string{}, baseTools...), cfg.SharedTools...), nil
	}
	if len(cfg.SharedTools) > 0 {
		return nil, fmt.Errorf(
			"doctor config: SharedTools and DeclaredTools are both set; "+
				"SharedTools extends the base compiler set %v and DeclaredTools "+
				"replaces it, so setting both states a contradiction about which "+
				"was meant - declare one list", baseTools)
	}
	return cfg.DeclaredTools, nil
}

func checkWorkspaceShape(targets []string) error {
	for _, dir := range targets {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("replace target %s missing -- clone the sibling repo next to this one", dir)
		}
	}
	return nil
}

func checkCodegen(plugins []string) error {
	for _, plugin := range plugins {
		if err := checkStorePath(plugin); err != nil {
			return fmt.Errorf("%w -- run 'nix develop'", err)
		}
	}
	return nil
}

// checkGopy verifies the FFI codegen tool is present and was built from the
// pinned module version (read from the binary's embedded module info, so a
// floating @latest install is caught even when the binary exists).
func checkGopy(pinned string) error {
	path, err := exec.LookPath("gopy")
	if err != nil {
		return fmt.Errorf("gopy not found -- re-enter 'nix develop' (the shell hook installs the pinned version)")
	}
	out, err := exec.Command("go", "version", "-m", path).Output()
	if err != nil {
		return fmt.Errorf("could not read module info from %s: %w", path, err)
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "mod" && strings.Contains(fields[1], "gopy") {
			if fields[2] != pinned {
				return fmt.Errorf("gopy is %s, pinned is %s -- remove %s and re-enter 'nix develop'", fields[2], pinned, path)
			}
			return nil
		}
	}
	return fmt.Errorf("no gopy module info in %s -- remove it and re-enter 'nix develop'", path)
}

func checkEnv(vars []string) error {
	for _, v := range vars {
		if os.Getenv(v) == "" {
			return fmt.Errorf("%s is not set -- enter the dev shell ('nix develop'), which exports it", v)
		}
	}
	return nil
}
