package agentmgr

import (
	"google.golang.org/protobuf/types/known/anypb"

	"context"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/lifecycle"
)

func TestTimerAgent_Lifecycle(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	lbus := eventbus.NewInprocBus[*anypb.Any]()

	agent := NewTimerAgent(
		"test_timer",
		"/tmp/test_agent.py",
		"5s",
		"runner.py",
		bus,
		lbus,
	)

	if agent.ID() != "test_timer" {
		t.Errorf("ID() = %q, want %q", agent.ID(), "test_timer")
	}

	if agent.Type() != "timer" {
		t.Errorf("Type() = %q, want %q", agent.Type(), "timer")
	}

	if agent.Lang() != "python" {
		t.Errorf("Lang() = %q, want %q", agent.Lang(), "python")
	}

	// Test Initialize
	ctx := context.Background()
	if err := agent.Initialize(ctx); err != nil {
		t.Errorf("Initialize() error = %v", err)
	}

	// Test Start
	if err := agent.Start(ctx); err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Verify timer is running
	agent.mu.Lock()
	running := agent.cancel != nil
	agent.mu.Unlock()

	if !running {
		t.Error("Timer should be running after Start()")
	}

	// Test double start should error
	if err := agent.Start(ctx); err == nil {
		t.Error("Start() on running timer should error")
	}

	// Test Stop
	if err := agent.Stop(ctx); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Verify timer stopped
	agent.mu.Lock()
	stopped := agent.cancel == nil
	agent.mu.Unlock()

	if !stopped {
		t.Error("Timer should be stopped after Stop()")
	}

	// Test double stop should be no-op
	if err := agent.Stop(ctx); err != nil {
		t.Errorf("Stop() on stopped timer should not error, got %v", err)
	}
}

func TestTimerAgent_Reload(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	lbus := eventbus.NewInprocBus[*anypb.Any]()

	agent := NewTimerAgent("test_timer", "/tmp/test.py", "10s", "runner.py", bus, lbus)

	ctx := context.Background()

	// Start
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Reload should stop and restart
	if err := agent.Reload(ctx); err != nil {
		t.Errorf("Reload() error = %v", err)
	}

	// Should be running again
	agent.mu.Lock()
	running := agent.cancel != nil
	agent.mu.Unlock()

	if !running {
		t.Error("Timer should be running after Reload()")
	}

	// Cleanup
	_ = agent.Stop(ctx)
}

func TestTimerAgent_Reset(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	lbus := eventbus.NewInprocBus[*anypb.Any]()

	agent := NewTimerAgent("test_timer", "/tmp/test.py", "5s", "runner.py", bus, lbus)

	ctx := context.Background()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Reset should stop the timer
	agent.Reset()

	agent.mu.Lock()
	stopped := agent.cancel == nil
	agent.mu.Unlock()

	if !stopped {
		t.Error("Timer should be stopped after Reset()")
	}
}

func TestTimerAgent_InvalidSchedule(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	lbus := eventbus.NewInprocBus[*anypb.Any]()

	agent := NewTimerAgent("test_timer", "/tmp/test.py", "invalid-schedule", "runner.py", bus, lbus)

	ctx := context.Background()
	err := agent.Start(ctx)

	if err == nil {
		t.Error("Start() with invalid schedule should error")
		_ = agent.Stop(ctx)
	}
}

func TestTimerAgent_Describe(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	lbus := eventbus.NewInprocBus[*anypb.Any]()

	agent := NewTimerAgent("my_timer", "/path/to/agent.py", "@hourly", "runner.py", bus, lbus)

	desc := agent.Describe()

	tests := []struct {
		key  string
		want string
	}{
		{"id", "my_timer"},
		{"type", "timer"},
		{"language", "python"},
		{"path", "/path/to/agent.py"},
		{"schedule", "@hourly"},
	}

	for _, tt := range tests {
		if got := desc[tt.key]; got != tt.want {
			t.Errorf("Describe()[%q] = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestTimerAgent_ScheduleExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timer execution test in short mode")
	}

	bus := eventbus.NewInprocBus[*anypb.Any]()
	lbus := eventbus.NewInprocBus[*anypb.Any]()

	// Use a very short interval for testing
	agent := NewTimerAgent("test_timer", "/tmp/test.py", "1s", "runner.py", bus, lbus)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		if err := agent.Stop(context.Background()); err != nil {
			t.Errorf("cleanup Stop: %v", err)
		}
	}()

	// Wait for at least one execution cycle
	// Note: The execute() function will fail since we don't have a real agent,
	// but the timer loop should still run
	time.Sleep(2 * time.Second)

	// Verify timer is still running
	agent.mu.Lock()
	running := agent.cancel != nil
	agent.mu.Unlock()

	if !running {
		t.Error("Timer should still be running")
	}
}

// TestTimerAgent_ControllerStart replicates the supervisor's timer
// auto-start path: ctrl.Apply(ActionStart) must succeed, which requires the
// timer to publish a RUNNING status carrying the controller-set run id on
// the lifecycle bus (GAPI-DIV-021: timers previously published nothing and
// every controller start timed out after 10s).
func TestTimerAgent_ControllerStart(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	lbus := eventbus.NewInprocBus[*anypb.Any]()

	agent := NewTimerAgent(
		"ctrl_timer",
		"/tmp/test_agent.py",
		"1h", // never fires during the test; only the RUNNING transition matters
		"runner.py",
		bus,
		lbus,
	)

	if err := agent.Controller().Apply(lifecycle.ActionStart); err != nil {
		t.Fatalf("controller start failed (no RUNNING status published?): %v", err)
	}
	defer func() {
		if err := agent.Controller().Apply(lifecycle.ActionStop); err != nil {
			t.Errorf("controller stop failed: %v", err)
		}
	}()

	if got := agent.Controller().State(); got != "running" {
		t.Fatalf("controller state = %q, want running", got)
	}
}
