// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk_test

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPythonRunner_NativeImportFailureKeepsCWD pins the boundary contract
// behind GAPI-DIV-025.
//
// gopy's generated gapi/native/adk.py chdirs into its own directory so
// dlopen can find _adk.so, and restores the caller's directory only on
// the success path:
//
//	cwd = os.getcwd()
//	os.chdir(currentdir)
//	from . import _adk      # raises when the extension was never built
//	os.chdir(cwd)           # never reached
//
// runner.py catches that ImportError and falls back to DummyAdk, so the
// failure looks survivable - but the process is now parked in
// adk/python/gapi/native, and every relative path the runner is given
// afterwards resolves against the wrong root. On CI (where *.so is
// gitignored and the extension is not built) that turned every python
// fixture invocation into "FileNotFoundError ... /adk/python/gapi/native/
// fixtures/python/simple_service.py", exit 2.
//
// The runner must own its own working directory across that import.
func TestPythonRunner_NativeImportFailureKeepsCWD(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	// A copy of the ADK tree with every compiled extension left behind
	// reproduces a runner that was never built - the CI condition -
	// without touching the developer's working tree.
	stripped := filepath.Join(t.TempDir(), "python")
	if err := copyTreeWithoutSharedObjects(filepath.Join(repoRoot, "adk", "python"), stripped); err != nil {
		t.Fatalf("stage stripped ADK tree: %v", err)
	}

	cmd := exec.Command("python3", filepath.Join(stripped, "agent", "runner.py"),
		"--module", "fixtures/python/simple_service.py", "--describe")
	// The relative --module path is the whole point: it is only
	// resolvable from the directory the caller chose.
	cmd.Dir = filepath.Join(repoRoot, "test", "adk")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("runner --describe failed with the native extension absent: %v\nOutput: %s", err, output)
	}

	// The DummyAdk fallback warns on stderr; the describe document is the
	// last line of the combined stream.
	var metadata map[string]any
	if uerr := json.Unmarshal([]byte(lastJSONLine(string(output))), &metadata); uerr != nil {
		t.Fatalf("describe output did not parse: %v\nOutput: %s", uerr, output)
	}
	if _, ok := metadata["describe"]; !ok {
		t.Errorf("missing 'describe' key; got %v", metadata)
	}
}

// lastJSONLine returns the final line that looks like a JSON document,
// skipping the DummyAdk import diagnostics that share the stream.
func lastJSONLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); strings.HasPrefix(line, "{") {
			return line
		}
	}
	return ""
}

// copyTreeWithoutSharedObjects mirrors src into dst, dropping compiled
// extensions and caches so the staged tree imports exactly as a fresh
// clone does.
func copyTreeWithoutSharedObjects(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		if d.IsDir() {
			if d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o750)
		}
		if strings.HasSuffix(d.Name(), ".so") {
			return nil
		}
		return copyFile(path, filepath.Join(dst, rel))
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
