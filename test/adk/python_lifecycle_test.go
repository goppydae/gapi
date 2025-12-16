package adk

import (
	"testing"
	"time"
)

func TestPythonAgent_StartStop(t *testing.T) {
	h, err := NewHarness()
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}

	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start gapid: %v", err)
	}
	defer h.Stop()

	agentID := "simple_service"

	// Agent should start automatically
	if err := h.WaitForState(agentID, "running", 5*time.Second); err != nil {
		t.Errorf("Agent did not start: %v", err)
	}

	// Stop the agent
	if err := h.SendLifecycleAction(agentID, "stop"); err != nil {
		t.Errorf("Failed to stop agent: %v", err)
	}

	// Wait for stopped state
	if err := h.WaitForState(agentID, "stopped", 15*time.Second); err != nil {
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
	defer h.Stop()

	agentID := "simple_service"

	// Wait for running
	if err := h.WaitForState(agentID, "running", 15*time.Second); err != nil {
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
	defer h.Stop()

	agentID := "simple_timer"

	// Timer should auto-start
	if err := h.WaitForState(agentID, "running", 15*time.Second); err != nil {
		t.Errorf("Timer agent did not start: %v", err)
	}

	// Stop the timer
	if err := h.SendLifecycleAction(agentID, "stop"); err != nil {
		t.Errorf("Failed to stop timer: %v", err)
	}

	if err := h.WaitForState(agentID, "stopped", 15*time.Second); err != nil {
		t.Errorf("Timer did not stop: %v", err)
	}
}
