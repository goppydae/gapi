// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

// package adk, not adk_test: this suite drives the harness in
// framework.go, which is unexported. Its sibling python_lifecycle_test.go
// is already internal for the same reason.
package adk

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/goppydae/gapi/core/config"
)

// CrossADKTestSuite defines a test scenario that should behave identically
// across both Python and Go ADKs.
type CrossADKTestSuite struct {
	Name        string
	Description string
	GoAgent     string // Path to Go agent source
	PyAgent     string // Path to Python agent module
	TestFunc    func(t *testing.T, lang string, agentPath string)
}

// TestCrossADKParity runs identical tests against both Go and Python agents
// to verify they produce the same behavior.
func TestCrossADKParity(t *testing.T) {
	suites := []CrossADKTestSuite{
		{
			Name:        "DescribeMetadata",
			Description: "Verify --describe output is consistent",
			GoAgent:     "fixtures/go/simple.go.service",
			PyAgent:     "fixtures/python/simple_service.py",
			TestFunc:    testDescribeMetadata,
		},
		{
			Name:        "LifecycleTransitions",
			Description: "Verify Initialize -> Start -> Stop lifecycle",
			GoAgent:     "fixtures/go/lifecycle.go.service",
			PyAgent:     "fixtures/python/lifecycle_agent.py",
			TestFunc:    testLifecycleTransitions,
		},
		{
			Name:        "CapabilityDetection",
			Description: "Verify capability introspection",
			GoAgent:     "fixtures/go/capabilities.go.service",
			PyAgent:     "fixtures/python/capabilities_agent.py",
			TestFunc:    testCapabilityDetection,
		},
		// "SchemaHashing" was here and is now schema_skew_test.go. It
		// hashed an arbitrary temp file with BLAKE3 in Go and SHA-256 in
		// Python - the fixture's own comment said SHA-256 was chosen
		// because it "also produces 64-char hex" - ran each language
		// against itself, compared neither to the other, and asserted
		// only 64 hex characters and stability across two runs. Every
		// one of those holds for two implementations that share nothing,
		// which is why it passed unchanged through all of GAPI-DIV-127's
		// implementation while both ADKs reported no contract at all.
	}

	for _, suite := range suites {
		t.Run(suite.Name, func(t *testing.T) {
			t.Run("Go", func(t *testing.T) {
				suite.TestFunc(t, "go", suite.GoAgent)
			})
			t.Run("Python", func(t *testing.T) {
				suite.TestFunc(t, "python", suite.PyAgent)
			})
		})
	}
}

// testDescribeMetadata verifies that --describe produces consistent output
func testDescribeMetadata(t *testing.T, lang string, agentPath string) {
	var cmd *exec.Cmd

	switch lang {
	case "go":
		cmd = exec.Command(buildGoFixture(t, agentPath), "--describe")
	case "python":
		runner := findPythonRunner(t)
		cmd = exec.Command("python3", runner, "--module", agentPath, "--describe")
	default:
		t.Fatalf("Unknown language: %s", lang)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run --describe: %v\nOutput: %s", err, output)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(output, &metadata); err != nil {
		t.Fatalf("Failed to parse describe output: %v\nOutput: %s", err, output)
	}

	// Verify required fields
	desc, ok := metadata["describe"].(map[string]interface{})
	if !ok {
		t.Fatalf("Missing 'describe' key in metadata")
	}

	requiredFields := []string{"id", "version", "type", "language", "capabilities"}
	for _, field := range requiredFields {
		if _, exists := desc[field]; !exists {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Verify language is correct. (A previous shadowed `lang :=` compared
	// the value to itself, so this assertion could never fail - SA4000.)
	if got := desc["language"].(string); got != lang {
		t.Errorf("Expected language=%s, got %s", lang, got)
	}
}

// testLifecycleTransitions drives an agent THROUGH THE SUPERVISOR and
// asserts the observed state sequence.
//
// The previous version exec'd the agent BESIDE a supervisor and asserted
// nothing that could fail (GAPI-DIV-053). It ran the Go fixture with
// --start, which exited 2 in milliseconds because no template defined
// that flag; slept two seconds; signalled a process that was already
// gone, swallowing the error to t.Logf; then selected on a Wait that had
// already returned against a 30-second timeout. Every step after the
// build passed on a corpse, and the Python half being real is what made
// the pair read as evidence of parity.
//
// What holds it closed now is that the states come from the supervisor's
// own view via gapictl, so an agent that never starts cannot reach
// "running" no matter how the test is written.
func testLifecycleTransitions(t *testing.T, lang string, agentPath string) {
	stage := t.TempDir()
	agentID := stageAgent(t, stage, lang, agentPath)

	h, err := NewHarnessAt(stage)
	if err != nil {
		t.Fatalf("harness: %v", err)
	}
	if err := h.Start(); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	defer func() {
		if err := h.Stop(); err != nil {
			t.Errorf("stop supervisor: %v", err)
		}
	}()

	// PENDING -> RUNNING. For a Go agent this is only reachable because
	// the ADK emits the "ready" control event; nothing else in the tree
	// produces the transition.
	if err := h.WaitForState(agentID, "running", config.TestAgentStartTimeout); err != nil {
		t.Fatalf("%s agent did not reach running: %v", lang, err)
	}

	if err := h.SendLifecycleAction(agentID, "stop"); err != nil {
		t.Fatalf("stop %s agent: %v", lang, err)
	}

	// RUNNING -> STOPPED.
	if err := h.WaitForState(agentID, "stopped", config.TestAgentStopTimeout); err != nil {
		t.Errorf("%s agent did not reach stopped: %v", lang, err)
	}
}

// stageAgent puts one agent into dir and returns the ID discovery will
// give it. Go agents are built through the same path 'gapictl agent
// build' uses; Python agents are copied, because a Python agent's source
// IS its artifact.
func stageAgent(t *testing.T, dir, lang, agentPath string) string {
	t.Helper()

	switch lang {
	case "go":
		out, err := exec.Command(gapictlPath(t), "agent", "build", agentPath, "-o", dir).CombinedOutput()
		if err != nil {
			t.Fatalf("build %s: %v\n%s", agentPath, err, out)
		}
		return declaredID(t, agentPath, `(?m)^\s*ID\s+=\s+"([a-z0-9_]+)"`)

	case "python":
		// The name carries the type, and discovery routes on it: a
		// fixture copied as "lifecycle_agent.py" lands in the Python
		// SERVICE branch by suffix rather than by declaration.
		dst := filepath.Join(dir, "parity_lifecycle_py.py.service")
		body, err := os.ReadFile(agentPath)
		if err != nil {
			t.Fatalf("read %s: %v", agentPath, err)
		}
		if err := os.WriteFile(dst, body, 0600); err != nil {
			t.Fatalf("stage %s: %v", dst, err)
		}
		return declaredID(t, agentPath, `(?m)^ID\s*=\s*"([a-z0-9_]+)"`)

	default:
		t.Fatalf("unknown language: %s", lang)
		return ""
	}
}

// buildGoFixture builds a target-form Go agent through 'gapictl agent
// build' and returns the binary.
//
// It goes through the real command rather than 'go build' on the source:
// a Go agent file is not a program - it has no main - so 'go build' on it
// cannot work, and building it any other way would test a path no
// operator uses.
func buildGoFixture(t *testing.T, agentPath string) string {
	t.Helper()

	dir := t.TempDir()
	out, err := exec.Command(gapictlPath(t), "agent", "build", agentPath, "-o", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", agentPath, err, out)
	}
	return filepath.Join(dir, filepath.Base(agentPath))
}

func declaredID(t *testing.T, path, pattern string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	m := regexp.MustCompile(pattern).FindSubmatch(body)
	if m == nil {
		t.Fatalf("no ID declaration in %s", path)
	}
	return string(m[1])
}

func gapictlPath(t *testing.T) string {
	t.Helper()

	dir, err := suiteBinDir()
	if err != nil {
		t.Fatalf("gapictl: %v", err)
	}
	p := filepath.Join(dir, "gapictl")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("gapictl is not built (%v)", err)
	}
	return p
}

// testCapabilityDetection verifies capability introspection
func testCapabilityDetection(t *testing.T, lang string, agentPath string) {
	var cmd *exec.Cmd

	switch lang {
	case "go":
		cmd = exec.Command(buildGoFixture(t, agentPath), "--describe")
	case "python":
		runner := findPythonRunner(t)
		cmd = exec.Command("python3", runner, "--module", agentPath, "--describe")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run --describe: %v", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(output, &metadata); err != nil {
		t.Fatalf("Failed to parse describe output: %v", err)
	}

	desc := metadata["describe"].(map[string]interface{})
	caps, ok := desc["capabilities"].([]interface{})
	if !ok {
		t.Fatalf("capabilities field is not an array")
	}

	// Expected capabilities for this test agent
	expectedCaps := []string{"initialize", "start", "stop", "reload", "custom_action"}

	var actualCaps []string
	for _, c := range caps {
		actualCaps = append(actualCaps, c.(string))
	}

	if diff := cmp.Diff(expectedCaps, actualCaps); diff != "" {
		t.Errorf("Capabilities mismatch (-want +got):\n%s", diff)
	}
}

// findPythonRunner locates the Python runner script
func findPythonRunner(t *testing.T) string {
	candidates := []string{
		"../../adk/python/agent/runner.py",
		"adk/python/agent/runner.py",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			abs, _ := filepath.Abs(path)
			return abs
		}
	}

	t.Fatal("Could not find Python runner.py")
	return ""
}

// TestADKIntegration verifies that Go and Python agents coexist under
// one supervisor.
//
// The previous version asserted nothing that could fail (GAPI-DIV-053).
// It checked cmd.ProcessState, which is nil until Wait is called and Wait
// was never called, so both of its assertions were unreachable. It also
// exported GAPI_QUIC_ADDR, which no code in the repo reads - a variable
// set by a test and read by nobody is a false signal of wiring, so it is
// gone rather than renamed.
//
// Both agents are now staged into ONE directory and driven through the
// supervisor, which is what "coexist" actually means: discovery routes
// each to its own branch by suffix, and both must reach running.
func TestADKIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("MixedLanguageAgents", func(t *testing.T) {
		stage := t.TempDir()
		goID := stageAgent(t, stage, "go", "fixtures/go/lifecycle.go.service")
		pyID := stageAgent(t, stage, "python", "fixtures/python/lifecycle_agent.py")

		if goID == pyID {
			t.Fatalf("both agents claim the id %q; the test cannot tell them apart", goID)
		}

		h, err := NewHarnessAt(stage)
		if err != nil {
			t.Fatalf("harness: %v", err)
		}
		if err := h.Start(); err != nil {
			t.Fatalf("start supervisor: %v", err)
		}
		defer func() {
			if err := h.Stop(); err != nil {
				t.Errorf("stop supervisor: %v", err)
			}
		}()

		// Liveness comes from the supervisor's own view rather than from
		// a process handle, so it holds whether or not anyone called
		// Wait - the defect this test replaces.
		for _, id := range []string{goID, pyID} {
			if err := h.WaitForState(id, "running", config.TestAgentStartTimeout); err != nil {
				t.Errorf("agent %s did not reach running: %v", id, err)
			}
		}
	})
}
