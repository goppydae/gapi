package adk

import (
	"testing"
	"time"

	"github.com/goppydae/gapi/core/config"
)

func TestPythonAgent_StartStop(t *testing.T) {
	h, err := NewHarness()
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}

	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start gapid: %v", err)
	}
	defer func() {
		if err := h.Stop(); err != nil {
			t.Errorf("stop harness: %v", err)
		}
	}()

	agentID := "test_simple_service" // the fixture agent; the search path is fenced to fixtures

	// Agent should start automatically
	if err := h.WaitForState(agentID, "running", config.TestAgentStartTimeout); err != nil {
		t.Errorf("Agent did not start: %v", err)
	}

	// Stop the agent
	if err := h.SendLifecycleAction(agentID, "stop"); err != nil {
		t.Errorf("Failed to stop agent: %v", err)
	}

	// Wait for stopped state
	if err := h.WaitForState(agentID, "stopped", config.TestAgentStopTimeout); err != nil {
		t.Errorf("Agent did not stop: %v", err)
	}
}

func TestPythonAgent_Reload(t *testing.T) {
	h, err := NewHarness()
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}

	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start gapid: %v", err)
	}
	defer func() {
		if err := h.Stop(); err != nil {
			t.Errorf("stop harness: %v", err)
		}
	}()

	agentID := "test_simple_service" // the fixture agent; the search path is fenced to fixtures

	// Wait for running
	if err := h.WaitForState(agentID, "running", config.TestAgentStopTimeout); err != nil {
		t.Fatalf("Agent did not start: %v", err)
	}

	// Reload the agent
	if err := h.SendLifecycleAction(agentID, "reload"); err != nil {
		t.Errorf("Failed to reload agent: %v", err)
	}

	// Should still be running after reload
	time.Sleep(1 * time.Second)
	state, err := h.GetAgentState(agentID)
	if err != nil {
		t.Errorf("Failed to get agent state: %v", err)
	}
	if state != "running" {
		t.Errorf("Expected running after reload, got %s", state)
	}
}

func TestTimerAgent_Execution(t *testing.T) {
	h, err := NewHarness()
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}

	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start gapid: %v", err)
	}
	defer func() {
		if err := h.Stop(); err != nil {
			t.Errorf("stop harness: %v", err)
		}
	}()

	agentID := "simple_timer"

	// Timer should auto-start
	if err := h.WaitForState(agentID, "running", config.TestAgentStopTimeout); err != nil {
		t.Errorf("Timer agent did not start: %v", err)
	}

	// Stop the timer
	if err := h.SendLifecycleAction(agentID, "stop"); err != nil {
		t.Errorf("Failed to stop timer: %v", err)
	}

	if err := h.WaitForState(agentID, "stopped", config.TestAgentStopTimeout); err != nil {
		t.Errorf("Timer did not stop: %v", err)
	}
}
