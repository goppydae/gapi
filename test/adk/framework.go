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
}

// NewHarness creates a new test harness
func NewHarness() (*TestHarness, error) {
	// Get project root
	root, err := findProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to find project root: %w", err)
	}

	return &TestHarness{
		gapictl:   filepath.Join(root, "bin", "gapictl"),
		agentsDir: filepath.Join(root, "test", "adk", "fixtures"),
		configDir: filepath.Join(root, "config"),
		binDir:    filepath.Join(root, "bin"),
	}, nil
}

// Start starts the gapid supervisor
func (h *TestHarness) Start() error {
	if h.gapidCmd != nil {
		return fmt.Errorf("gapid already running")
	}

	h.ctx, h.cancel = context.WithCancel(context.Background())

	gapidPath := filepath.Join(h.binDir, "gapid")
	h.gapidCmd = exec.CommandContext(h.ctx, gapidPath)

	// Set environment. RUNTIME_AGENT_PATH is fenced to the fixtures dir
	// ONLY: including the checkout's production agents/ made discovery
	// start whatever example agents happen to live there, and their start
	// timeouts starved the fixture agents' state transitions (the ~90s
	// TestTimerAgent_Execution failure; GAPI-DIV-021).
	root, _ := findProjectRoot() // Ignore error since we already validated in NewHarness
	h.gapidCmd.Env = append(os.Environ(),
		fmt.Sprintf("RUNTIME_AGENT_PATH=%s", h.agentsDir),
		fmt.Sprintf("RUNTIME_PY_RUNNER=%s", filepath.Join(root, "adk", "python", "agent", "runner.py")),
		"RUNTIME_FORCE_DUMMY_ADK=1",
		"RUNTIME_CGROUPS_DISABLE=1",
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

// waitForReady waits for gapid to be responsive using gapictl ping
func (h *TestHarness) waitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Check process liveness
		if h.gapidCmd.ProcessState != nil && h.gapidCmd.ProcessState.Exited() {
			return fmt.Errorf("gapid process exited unexpectedly")
		}

		cmd := exec.Command(h.gapictl, "ping")
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return context.DeadlineExceeded
}
