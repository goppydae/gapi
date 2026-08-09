// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

// package adk, not adk_test: these cases drive the harness in
// framework.go and read its retained daemon log, both unexported.
package adk

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/config"
)

// The two demonstrations GAPI-DIV-127 closes on (see
// docs/superpowers/plans/2026-08-08-schema-hash-skew.md, Task 9).
//
// THIS FILE REPLACES THE CROSS-ADK "SchemaHashing" CASE, which measured
// nothing and passed through all eight implementing tasks unchanged: the
// Python fixture hashed with SHA-256 under a "Verify BLAKE3" label, the
// two languages were never compared to each other, and the only
// assertions were 64 hex characters and stability across two runs -
// which any hash function satisfies. It hashed an arbitrary temp file
// rather than the contract. Its fixtures went with it.

// TestSchemaSkewIsReportedAtRegistration drives a skewed agent through
// DISCOVERY, which is the only thing that exercises setupAgents' call to
// reportSchemaSkew.
//
// IT GOES THROUGH THE DAEMON RATHER THAN CALLING THE REPORTER. The unit
// cases in core/supervisor construct a Supervisor and call
// reportSchemaSkew directly, so they stay green with the call site in
// setupAgents deleted - a test that survives the removal of its subject
// is not a test, and this repository wrote that defect twice in one
// session. Neutering that call reddens this case and nothing else.
//
// The plan recorded this call site as covered only by a manual `agent
// reload` drill. It is a real test instead; the drill still runs,
// because reload is the trigger an operator actually uses and this
// covers daemon start.
func TestSchemaSkewIsReportedAtRegistration(t *testing.T) {
	stage := t.TempDir()
	stageSkewAgent(t, stage)

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

	// POLLED RATHER THAN READ ONCE. registerLivenessHandlers runs BEFORE
	// setupAgents, deliberately (GAPI-DIV-120), so the daemon answers the
	// harness's readiness ping while discovery is still running. Reading
	// the log the instant Start returns is a race the report usually
	// loses.
	out := waitForRegistrationReport(t, h, registrationReportRE, config.TestAgentStartTimeout)

	// MATCHED ON THE REGISTRATION FORM SPECIFICALLY, and that is not
	// pedantry - it is the difference between this being a test and not.
	//
	// The daemon has TWO detection sites and both emit this sentence. A
	// first version of this case matched the message anywhere in the log,
	// and it PASSED with setupAgents' call deleted: the agent went on to
	// start, its LifecycleStatus carried the same forced hash, and
	// core/agentmgr reported it a few milliseconds later. Measured, not
	// reasoned about.
	//
	// The two are told apart by the run id. Registration has none,
	// because the agent has not started, so schemaskew.Report renders
	// "agent X was built against"; the status path renders
	// "agent X (run <uuid>) was built against". Requiring the id and the
	// verb to be ADJACENT makes only the registration site match.
	line := findLine(out, registrationReportRE)
	if line == "" {
		t.Fatalf("no registration-time skew report for skew_agent; "+
			"setupAgents' call to reportSchemaSkew did not fire.\n"+
			"(a report carrying a run id is the status path, not this one)\n%s", out)
	}
	m := registrationReportRE.FindStringSubmatch(line)

	// EXACTLY ONE "module", ASSERTED ON THE REAL DAEMON'S RECORD. The
	// supervisor's logger is scoped With(Module("supervisor")) and the
	// Reporter adds a module naming the detection site, so handing the
	// reporter that logger emitted the key TWICE. Duplicate keys are legal
	// JSON and every consumer resolves them differently, so the record
	// stayed parseable and stopped meaning one thing.
	//
	// IT IS ASSERTED HERE RATHER THAN IN core/supervisor's UNIT TESTS
	// BECAUSE THOSE CANNOT SEE IT. They build a Reporter directly with an
	// unscoped logger, which is not what New does - so they were green
	// throughout. Only a record produced by the real daemon carries the
	// wiring under test.
	if n := strings.Count(line, `"module":`); n != 1 {
		t.Errorf("the skew report carries %d \"module\" keys, want 1:\n%s", n, line)
	}

	// ASSERTED ON CONTENT, NOT ON A COUNT. The report is only useful if
	// an operator can tell WHICH two contracts disagree.
	if got := m[1]; got != forcedSchemaHash {
		t.Errorf("the report names %s as the agent's contract, want %s", got, forcedSchemaHash)
	}

	// THE DAEMON'S OWN HASH IS READ OUT OF THE REPORT, not computed here.
	// schemahash.Contract() in THIS binary answers af1349b9... - BLAKE3 of
	// no input - because a test binary links no gapi.v1 descriptors. An
	// assertion against it would compare the daemon to a process that has
	// no contract at all, and would have pinned the empty digest as the
	// expected value of a passing test.
	if got := m[2]; got == forcedSchemaHash {
		t.Errorf("the daemon reported the agent's hash as its own (%s); "+
			"the comparison has nothing to discriminate", got)
	}

	if t.Failed() {
		t.Logf("daemon log:\n%s", out)
	}
}

// TestSchemaHashParityAcrossADKs is the claim that makes the mechanism
// cross-language: a Go agent and a Python agent, built and run
// separately, report the SAME contract for the same tree.
//
// This is what the retired "SchemaHashing" case was named for and never
// did - it ran each language against itself and compared neither to the
// other.
func TestSchemaHashParityAcrossADKs(t *testing.T) {
	goHash := describedSchemaHash(t, "go", "fixtures/go/simple.go.service")
	pyHash := describedSchemaHash(t, "python", "fixtures/python/simple_service.py")

	// NON-EMPTY FIRST, AND THAT ORDER IS LOAD-BEARING. Two agents that
	// both compute nothing report identical empty strings and would
	// satisfy an equality check alone - which is GAPI-DIV-127's first
	// defect passing its own test.
	if goHash == "" {
		t.Fatal("the Go agent reports no schema hash - GAPI-DIV-127's second defect")
	}
	if pyHash == "" {
		t.Fatal("the Python agent reports no schema hash")
	}

	if goHash != pyHash {
		t.Fatalf("the two ADKs disagree about the contract they share:\n"+
			"  go     %s\n  python %s", goHash, pyHash)
	}
}

// forcedSchemaHash is the value skew_agent.py forces. Duplicated here
// rather than parsed out of the fixture: a test that reads its expected
// value from the thing under test cannot disagree with it.
const forcedSchemaHash = "0000000000000000000000000000000000000000000000000000000000000000"

// registrationReportRE matches the WARN setupAgents produces, and only
// that one, capturing the agent's contract and the daemon's.
//
// "skew_agent was" is ADJACENT on purpose: the status path renders
// "skew_agent (run <uuid>) was", so this pattern cannot match it. The
// level is in the pattern for the same reason the hashes are - a report
// demoted to INFO is one an operator does not see, and the entry's whole
// claim is that a mismatch is visible.
var registrationReportRE = regexp.MustCompile(
	`"level":"WARN".*agent skew_agent was built against protobuf contract ` +
		`([0-9a-f]{64}); this daemon carries ([0-9a-f]{64})\. ` +
		`The agent is NOT refused`)

// stageSkewAgent puts the mismatching Python agent into dir under a name
// discovery routes to the Python service branch, and returns its id.
//
// The suffix carries the type: discovery routes on the file name, so a
// fixture copied as "skew_agent.py" is not a service at all.
func stageSkewAgent(t *testing.T, dir string) string {
	t.Helper()

	src := "fixtures/python/skew_agent.py"
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	dst := filepath.Join(dir, "skew_agent.py.service")
	if err := os.WriteFile(dst, body, 0600); err != nil {
		t.Fatalf("stage %s: %v", dst, err)
	}
	return "skew_agent"
}

// findLine returns the single log line want matches, or "".
//
// The daemon writes one JSON record per line, and an assertion about a
// record's KEYS has to be scoped to that record: counting "module"
// across the whole log would count every other line's.
func findLine(out string, want *regexp.Regexp) string {
	for _, line := range strings.Split(out, "\n") {
		if want.MatchString(line) {
			return line
		}
	}
	return ""
}

// discoveryDone is the last line setupAgents writes. Once it appears, a
// registration-time report that has not been emitted never will be.
const discoveryDone = "agent setup complete"

// waitForRegistrationReport polls the harness's retained daemon output
// until want matches, and returns everything captured. It returns rather
// than failing, so the caller owns the failure message: "no
// registration-time report" is a more useful sentence than "pattern not
// found".
//
// POLLED BECAUSE THE DAEMON IS READY BEFORE IT IS DONE.
// registerLivenessHandlers runs ahead of setupAgents by design
// (GAPI-DIV-120), so the harness's ping is answered while discovery is
// still walking the path.
//
// BOUNDED ON DISCOVERY FINISHING RATHER THAN ON THE CLOCK. Waiting the
// full agent-start timeout made the fault-injection run take 120 seconds
// to report a failure that was decided in 50 milliseconds, and a
// revert-proof that takes two minutes is one nobody runs. The deadline
// stays as the outer bound for a daemon that wedges before discovery.
func waitForRegistrationReport(t *testing.T, h *TestHarness, want *regexp.Regexp, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		out := h.daemonLog.String()
		switch {
		case want.MatchString(out):
			return out
		case strings.Contains(out, discoveryDone):
			t.Log("discovery finished without a registration-time report")
			return out
		case time.Now().After(deadline):
			t.Logf("discovery did not finish within %s", timeout)
			return out
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// describedSchemaHash runs one agent's --describe and returns the
// schema_hash it reports.
//
// Each language goes through the path an operator uses: a Go agent is
// built by `gapictl agent build`, because a Go agent file has no main
// and `go build` on it cannot work; a Python agent's source IS its
// artifact and runs under the runner.
func describedSchemaHash(t *testing.T, lang, agentPath string) string {
	t.Helper()

	var cmd *exec.Cmd
	switch lang {
	case "go":
		cmd = exec.Command(buildGoFixture(t, agentPath), "--describe")
	case "python":
		cmd = exec.Command("python3", findPythonRunner(t), "--module", agentPath, "--describe")
	default:
		t.Fatalf("unknown language: %s", lang)
	}

	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s --describe: %v", lang, err)
	}

	var metadata struct {
		Describe struct {
			SchemaHash string `json:"schema_hash"`
		} `json:"describe"`
	}
	if err := json.Unmarshal(output, &metadata); err != nil {
		t.Fatalf("parse %s describe output: %v\n%s", lang, err, output)
	}
	return metadata.Describe.SchemaHash
}
