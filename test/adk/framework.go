// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// TestHarness provides utilities for ADK integration testing
type TestHarness struct {
	gapidCmd  *exec.Cmd
	gapictl   string
	agentsDir string
	configDir string
	binDir    string
	ctx       context.Context
	cancel    context.CancelFunc
	// readyAfter is how long Start waited for the daemon's first pong.
	// Recorded on the SUCCESS path deliberately: this suite's flake was a
	// readiness stall that still ended in a pass, so a number only kept
	// on failure would never have shown it (GAPI-DIV-120).
	readyAfter time.Duration
}

// NewHarness creates a harness over the whole fixtures tree.
func NewHarness() (*TestHarness, error) {
	root, err := findProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to find project root: %w", err)
	}
	return NewHarnessAt(filepath.Join(root, "test", "adk", "fixtures"))
}

// NewHarnessAt creates a harness whose discovery is fenced to agentsDir.
//
// A caller that stages exactly the agents its test needs gets two things
// the shared fixtures tree cannot give it. The obvious one is isolation.
// The load-bearing one is that unrelated agents' start timeouts cannot
// starve the agents under test - that is the ~90s TestTimerAgent_Execution
// failure recorded as GAPI-DIV-021, and it came from discovery finding
// more than the test meant it to.
func NewHarnessAt(agentsDir string) (*TestHarness, error) {
	root, err := findProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to find project root: %w", err)
	}

	binDir, err := suiteBinDir()
	if err != nil {
		return nil, err
	}

	return &TestHarness{
		gapictl:   filepath.Join(binDir, "gapictl"),
		agentsDir: agentsDir,
		configDir: filepath.Join(root, "config"),
		binDir:    binDir,
	}, nil
}

// Start starts the gapid supervisor
func (h *TestHarness) Start() error {
	if h.gapidCmd != nil {
		return fmt.Errorf("gapid already running")
	}

	h.ctx, h.cancel = context.WithCancel(context.Background())

	gapidPath := filepath.Join(h.binDir, "gapid")
	h.gapidCmd = exec.CommandContext(h.ctx, gapidPath, "start")

	// Set environment. GAPI_AGENT_PATH is fenced to the fixtures dir
	// ONLY: including the checkout's production agents/ made discovery
	// start whatever example agents happen to live there, and their start
	// timeouts starved the fixture agents' state transitions (the ~90s
	// TestTimerAgent_Execution failure; GAPI-DIV-021).
	//
	// The GAPI_ spellings are literal on purpose: they name the CHILD's
	// namespace, and the child is gapid. This process is a test binary
	// with no product identity of its own, so composing them through
	// core/product here would assert the parent's identity onto the
	// child (GAPI-DIV-061).
	root, _ := findProjectRoot() // Ignore error since we already validated in NewHarness
	h.gapidCmd.Env = append(os.Environ(),
		fmt.Sprintf("GAPI_AGENT_PATH=%s", h.agentsDir),
		// The fence is EXPLICIT since GAPI-DIV-063. AGENT_PATH used to
		// replace the whole search path as a side effect of being set,
		// and this harness depended on that side effect; it is additive
		// now, so without this line the tiers come back underneath and
		// discovery starts whatever agents the checkout happens to hold.
		"GAPI_AGENT_PATH_EXCLUSIVE=1",
		fmt.Sprintf("GAPI_PY_ADK=%s", filepath.Join(root, "adk", "python")),
		// NO ADK_FORCE_DUMMY. GAPI-DIV-099's gate is this suite passing
		// with it REMOVED, and it was set here because the stub was the
		// only Python ADK whose events the supervisor could see. The
		// control channel is a descriptor now, so the real binding
		// reports through the same path the Go ADK does and the stub has
		// nothing left to be load-bearing about.
		//
		// REJECT IT INSTEAD. Removing FORCE is not the same as refusing
		// the stub: productionMode defaults false, so the DummyAdk stayed
		// REACHABLE here, and this change made it DISCARD control events.
		// An unbuilt extension then degraded into 150-second timeouts
		// naming no cause - the cost of a suite that cannot tell "the
		// binding is missing" from "the agent is slow". With this set,
		// that state is a one-line failure at import instead.
		"ADK_REJECT_DUMMY=1",
		"GAPI_CGROUPS_DISABLE=1",
	)

	// Capture output for debugging
	h.gapidCmd.Stdout = os.Stdout
	h.gapidCmd.Stderr = os.Stderr

	// Own process group: Stop kills the whole group, or gapid's agent
	// children (which inherit the test binary's stdout) outlive the kill
	// and trip go test's "Test I/O incomplete" watchdog.
	h.gapidCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := h.gapidCmd.Start(); err != nil {
		return fmt.Errorf("failed to start gapid: %w", err)
	}

	// Wait for gapid to be ready
	if err := h.waitForReady(30 * time.Second); err != nil {
		if serr := h.Stop(); serr != nil {
			return fmt.Errorf("gapid failed to become ready: %w (stop: %w)", err, serr)
		}
		return fmt.Errorf("gapid failed to become ready: %w", err)
	}

	// PRINTED, not merely recorded. A field nothing reads is the defect
	// this repo keeps producing, and the number is only useful if a CI
	// log carries it: readiness here was 30s per supervisor start for
	// months while every run still passed, so nothing but the printed
	// figure would have shown the stall (GAPI-DIV-120).
	fmt.Printf("harness: gapid answered ping after %s\n", h.readyAfter.Round(time.Millisecond))

	return nil
}

// Stop stops the gapid supervisor
func (h *TestHarness) Stop() error {
	if h.cancel != nil {
		h.cancel()
	}

	if h.gapidCmd != nil && h.gapidCmd.Process != nil {
		// Kill the whole process group so agent children die with gapid.
		if err := syscall.Kill(-h.gapidCmd.Process.Pid, syscall.SIGKILL); err != nil {
			if err := h.gapidCmd.Process.Kill(); err != nil {
				return fmt.Errorf("failed to kill gapid: %w", err)
			}
		}
		if err := h.gapidCmd.Wait(); err != nil {
			// The group was just SIGKILLed, so a signal-death exit is the
			// expected outcome; anything else is a real Stop failure.
			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				h.gapidCmd = nil
				return fmt.Errorf("wait for gapid: %w", err)
			}
		}
		h.gapidCmd = nil
	}

	return nil
}

// GetAgentState returns the current state of an agent
func (h *TestHarness) GetAgentState(id string) (string, error) {
	cmd := exec.Command(h.gapictl, "agent", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gapictl status failed: %w\nOutput: %s", err, output)
	}

	// Parse output to find agent state
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, id) {
			// Format: " - agent_id (type) [state] ..."
			// Example: " - simple_service (service) [running]"
			if idx := strings.Index(line, "["); idx != -1 {
				end := strings.Index(line[idx:], "]")
				if end != -1 {
					state := line[idx+1 : idx+end]
					return state, nil
				}
			}
			// No bracketed state on the line: rely on brackets only, as
			// gapictl strictly emits them.
		}
	}

	return "", fmt.Errorf("agent %s not found", id)
}

// SendLifecycleAction sends a lifecycle action to an agent
func (h *TestHarness) SendLifecycleAction(id, action string) error {
	cmd := exec.Command(h.gapictl, "lifecycle", action, id)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("lifecycle %s %s failed: %w\nOutput: %s", action, id, err, output)
	}
	return nil
}

// WaitForState waits for an agent to reach a specific state
func (h *TestHarness) WaitForState(id, expectedState string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		state, err := h.GetAgentState(id)
		if err == nil && strings.EqualFold(state, expectedState) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Final check before erroring
	currentState, err := h.GetAgentState(id)
	if err == nil && strings.EqualFold(currentState, expectedState) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("timeout waiting for %s to reach state %s (last error: %w)", id, expectedState, err)
	}
	return fmt.Errorf("timeout waiting for %s to reach state %s (current: %s)", id, expectedState, currentState)
}

// findProjectRoot finds the project root directory
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up until we find go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find project root")
		}
		dir = parent
	}
}

// notReadyError reports a readiness timeout as DATA, so a failing run
// says what was observed rather than only that time ran out.
//
// GAPI-DIV-120 was diagnosed from a local reproduction rather than from
// CI, because every CI occurrence produced a bare deadline error: it
// named neither how many probes were attempted nor what the last one
// said. The numbers below are what makes the next occurrence readable
// from the log alone.
type notReadyError struct {
	Elapsed  time.Duration
	Attempts int
	// LastOutput is the final probe's combined output. `gapictl ping`
	// prints its own diagnosis - which address it dialled and where it
	// looked - and that line is the difference between "no daemon" and
	// "daemon present, not answering".
	LastOutput string
	LastErr    error
}

func (e *notReadyError) Error() string {
	return fmt.Sprintf(
		"gapid did not answer ping within %s: %d probe(s), last error: %v; last probe said: %s",
		e.Elapsed.Round(time.Millisecond), e.Attempts, e.LastErr, strings.TrimSpace(e.LastOutput))
}

func (e *notReadyError) Unwrap() error { return e.LastErr }

// probeTimeout bounds ONE readiness probe.
//
// Shorter than the readiness budget on purpose. `gapictl ping` carries
// its own 30s deadline, so with a 30s budget and no per-probe bound this
// loop got exactly ONE attempt and never polled at all - a retry loop
// that cannot retry. Bounding the probe here is what makes the budget a
// number of attempts rather than a single blocking call.
const probeTimeout = 5 * time.Second

// waitForReady waits for gapid to answer a ping, and reports what it saw
// if it never does.
func (h *TestHarness) waitForReady(timeout time.Duration) error {
	start := time.Now()
	deadline := start.Add(timeout)

	var attempts int
	var lastOutput string
	var lastErr error

	for time.Now().Before(deadline) {
		// Check process liveness
		if h.gapidCmd.ProcessState != nil && h.gapidCmd.ProcessState.Exited() {
			return fmt.Errorf("gapid process exited unexpectedly after %s (%d probes)",
				time.Since(start).Round(time.Millisecond), attempts)
		}

		attempts++
		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		out, err := exec.CommandContext(ctx, h.gapictl, "ping").CombinedOutput()
		cancel()
		if err == nil {
			// Recorded on success too: a ping that took seconds is the
			// early warning for the failure this instrumentation exists
			// to explain, and it is invisible if only failures report.
			h.readyAfter = time.Since(start)
			return nil
		}
		lastOutput, lastErr = string(out), err

		time.Sleep(1 * time.Second)
	}

	return &notReadyError{
		Elapsed:    time.Since(start),
		Attempts:   attempts,
		LastOutput: lastOutput,
		LastErr:    lastErr,
	}
}
