// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// builtBinDir holds the gapid and gapictl this suite compiled for
// itself. Empty until BuildBinaries runs.
var builtBinDir string

// BuildBinaries compiles gapid and gapictl into a directory the suite
// owns, and returns a function that removes it.
//
// GAPI-DIV-097: this suite shells out to both binaries, and before this
// existed it did not produce either of them. The absence check in
// gapictlPath fails loudly when bin/gapictl is missing, which made the
// harness look guarded while nothing at all detected a STALE binary -
// the common case for a developer who edits pkg/cli and runs the suite.
// A bin/gapictl a day older than the change under test produced
// `ok github.com/goppydae/gapi/test/adk 191.263s`, an entirely green run
// against the previous binary, over four subtests that failed
// immediately after `mage build`. CI was immune only because its
// workflow builds first; the local run typed the same command in the
// wrong sequence.
//
// Building rather than asserting freshness removes the failure mode
// instead of reporting it, and makes `go test ./test/adk/` correct on
// its own - which is what a developer will type.
//
// The binaries are NOT written to the checkout's bin/. A suite that
// overwrote the operator's build would make its own correctness a
// side effect on someone else's artifact, and `mage build` and this
// suite would race whenever both ran.
//
// No version ldflags are passed. mage's Build stamps them; nothing here
// reads a version, and a test that needed one would be asserting on the
// build system rather than on the code.
func BuildBinaries() (func(), error) {
	root, err := findProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to find project root: %w", err)
	}

	dir, err := os.MkdirTemp("", "gapi-adk-bin-")
	if err != nil {
		return nil, fmt.Errorf("failed to create bin dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	for _, name := range []string{"gapid", "gapictl"} {
		cmd := exec.Command("go", "build",
			"-o", filepath.Join(dir, name), "./cmd/"+name)
		cmd.Dir = root
		if out, berr := cmd.CombinedOutput(); berr != nil {
			cleanup()
			return nil, fmt.Errorf("failed to build %s: %w\n%s", name, berr, out)
		}
	}

	builtBinDir = dir
	return cleanup, nil
}

// suiteBinDir returns the directory BuildBinaries populated.
//
// It reports the omission rather than falling back to the checkout's
// bin/: a fallback would restore the exact hole GAPI-DIV-097 records,
// silently, on any path that forgot to build.
func suiteBinDir() (string, error) {
	if builtBinDir == "" {
		return "", fmt.Errorf("suite binaries were not built; TestMain must call BuildBinaries")
	}
	return builtBinDir, nil
}
