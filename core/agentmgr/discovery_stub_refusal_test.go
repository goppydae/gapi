// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// DISCOVERY AND SUPERVISION MUST ANSWER THE SAME QUESTION THE SAME WAY.
//
// GAPI-DIV-086: the --describe invocation set no environment, so a runner
// with no native binding fell back to the stub and described the agent
// successfully, while the run path refused that same runner at start. The
// node enumerated an agent it could not run, and reported both
// truthfully. The gap was invisible because each half was correct alone.
//
// The binding is forced absent by pointing the manager at a runner whose
// import of gapi.native.adk cannot succeed - which is the real condition
// on a packaged host that never built the extension (GAPI-DIV-085), not a
// simulation of one.

// stubOnlyRunner writes a runner that behaves as the real one does with
// no extension present: it honours ADK_REJECT_DUMMY and otherwise
// describes the agent from the stub.
func stubOnlyRunner(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "runner.py")
	body := `import os, sys, json
if os.getenv("ADK_REJECT_DUMMY"):
    print("[FATAL] the native ADK extension is not importable and "
          "ADK_REJECT_DUMMY is set, so the stub is refused", file=sys.stderr)
    sys.exit(1)
print(json.dumps({"describe": {"schema_version": "1.0.0", "id": "stub_agent",
    "name": "stub_agent", "type": "service", "language": "python",
    "enabled": True, "capabilities": ["start"]}}))
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	return path
}

func stubOnlyModule(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub_agent.py.service")
	if err := os.WriteFile(path, []byte("ID = \"stub_agent\"\n"), 0o600); err != nil {
		t.Fatalf("write module: %v", err)
	}
	return path
}

// TestProductionDiscoveryRefusesTheStub is the entry's gate: refused
// LOUDLY, with a message naming the missing extension, not skipped
// quietly - which is GAPI-DIV-079's class.
func TestProductionDiscoveryRefusesTheStub(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	am := NewAgentManager(nil, nil, stubOnlyRunner(t), true, nil)

	_, err := am.pythonDescribe(stubOnlyModule(t))
	if err == nil {
		t.Fatal("production discovery described an agent that cannot start")
	}
	if !strings.Contains(err.Error(), "extension") {
		t.Fatalf("refusal does not name the missing extension: %v", err)
	}
}

// TestNonProductionDiscoveryPermitsTheStub pins the other side, so the
// refusal cannot quietly become unconditional.
//
// A developer without a built extension still discovers their agent. That
// is the same condition the run path applies, and matching it is the
// whole point: an answer that differs by code path is the defect.
func TestNonProductionDiscoveryPermitsTheStub(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	am := NewAgentManager(nil, nil, stubOnlyRunner(t), false, nil)

	d, err := am.pythonDescribe(stubOnlyModule(t))
	if err != nil {
		t.Fatalf("non-production discovery refused the stub: %v", err)
	}
	if d == nil {
		t.Fatal("describe returned no descriptor and no error")
	}
}
