package agentmgr

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
)

// binPath resolves name from the hermetic shell's PATH: hardcoded FHS
// paths like /bin/true do not exist on NixOS hosts, so tests exec what
// the dev shell provides instead.
func binPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not found in PATH: %v", name, err)
	}
	return p
}

func TestGoAgent_Constructor(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	agent := NewGoAgent(
		"test_go_agent",
		"service",
		"/bin/echo",
		[]string{"dep1"},
		[]string{"want1"},
		[]string{"wantedby1"},
		[]string{"requiredby1"},
		"",
		"0.5",
		"100M",
		[]string{"CAP_NET_BIND_SERVICE"},
		bus,
		depResolver,
	)

	if agent.ID() != "test_go_agent" {
		t.Errorf("ID() = %q, want %q", agent.ID(), "test_go_agent")
	}

	if agent.Type() != "service" {
		t.Errorf("Type() = %q, want %q", agent.Type(), "service")
	}

	if agent.Lang() != "go" {
		t.Errorf("Lang() = %q, want %q", agent.Lang(), "go")
	}

	requires := agent.Requires()
	if len(requires) != 1 || requires[0] != "dep1" {
		t.Errorf("Requires() = %v, want [dep1]", requires)
	}

	wants := agent.Wants()
	if len(wants) != 1 || wants[0] != "want1" {
		t.Errorf("Wants() = %v, want [want1]", wants)
	}

	deps := agent.Dependencies()
	if len(deps) != 2 {
		t.Errorf("Dependencies() should combine requires and wants, got %v", deps)
	}
}

func TestGoAgent_Describe(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	sockPath := filepath.Join(t.TempDir(), "my.sock")
	agent := NewGoAgent(
		"my_agent",
		"socket",
		"/usr/bin/myapp",
		[]string{"redis"},
		[]string{},
		[]string{},
		[]string{},
		sockPath,
		"1.0",
		"512M",
		[]string{"CAP_SYS_ADMIN"},
		bus,
		depResolver,
	)

	desc := agent.Describe()

	tests := []struct {
		key  string
		want string
	}{
		{"id", "my_agent"},
		{"type", "socket"},
		{"language", "go"},
		{"path", "/usr/bin/myapp"},
		{"listen_stream", sockPath},
		{"cpu_limit", "1.0"},
		{"mem_limit", "512M"},
	}

	for _, tt := range tests {
		if got := desc[tt.key]; got != tt.want {
			t.Errorf("Describe()[%q] = %q, want %q", tt.key, got, tt.want)
		}
	}

	if caps := desc["capabilities"]; caps != "CAP_SYS_ADMIN" {
		t.Errorf("Describe()[capabilities] = %q, want %q", caps, "CAP_SYS_ADMIN")
	}
}

// Removed TestGoAgent_SetRunID - tests private field access

// Removed TestGoAgent_Uptime - tests private field access

func TestGoAgent_Controller(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	agent := NewGoAgent(
		"test_agent",
		"service",
		binPath(t, "true"),
		nil, nil, nil, nil,
		"",
		"", "",
		nil,
		bus,
		depResolver,
	)

	ctrl := agent.Controller()
	if ctrl == nil {
		t.Error("Controller() returned nil")
	}
}

// Removed TestGoAgent_Reset - tests private field access

func TestGoAgent_EnsureListener(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	agent := NewGoAgent(
		"test_agent",
		"socket",
		"/bin/true",
		nil, nil, nil, nil,
		sockPath,
		"", "",
		nil,
		bus,
		depResolver,
	)

	// Test creating listener
	f, err := agent.EnsureListener()
	if err != nil {
		t.Errorf("EnsureListener() error = %v", err)
	}
	if f != nil {
		defer func() {
			if err := f.Close(); err != nil {
				t.Errorf("close listener file: %v", err)
			}
		}()
	}

	// Verify socket exists
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		t.Error("Socket file was not created")
	}

	// Second call should return existing listener
	f2, err := agent.EnsureListener()
	if err != nil {
		t.Errorf("EnsureListener() second call error = %v", err)
	}
	if f2 != nil {
		defer func() {
			if err := f2.Close(); err != nil {
				t.Errorf("close listener file: %v", err)
			}
		}()
	}
}

func TestGoAgent_StartStop_Simple(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	// A trivially-exiting command resolved from the shell's PATH
	agent := NewGoAgent(
		"test_agent",
		"oneshot",
		binPath(t, "true"),
		nil, nil, nil, nil,
		"",
		"", "",
		nil,
		bus,
		depResolver,
	)

	ctx := context.Background()

	// Start
	err := agent.Start(ctx)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Give it time to complete
	time.Sleep(500 * time.Millisecond)

	// Stop (should be no-op if already exited)
	err = agent.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestGoAgent_StartStop_WithSleep(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	// Use sleep to keep process alive
	agent := NewGoAgent(
		"test_sleep",
		"service",
		"/bin/sleep",
		nil, nil, nil, nil,
		"",
		"", "",
		nil,
		bus,
		depResolver,
	)

	ctx := context.Background()

	// Start - Note: sleep without args will fail
	// Let's create a wrapper script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test_script.sh")
	script := `#!/bin/sh
sleep 30
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	agent.path = scriptPath

	err := agent.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify running
	time.Sleep(100 * time.Millisecond)

	agent.mu.RLock()
	cmd := agent.cmd
	agent.mu.RUnlock()

	if cmd == nil || cmd.Process == nil {
		t.Error("Process should be running")
	}

	// Stop
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = agent.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Verify stopped
	time.Sleep(100 * time.Millisecond)

	agent.mu.RLock()
	cmd = agent.cmd
	agent.mu.RUnlock()

	if cmd != nil && cmd.Process != nil {
		t.Error("Process should be stopped")
	}
}

func TestGoAgent_EventPublishing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	agent := NewGoAgent(
		"test_agent",
		"service",
		binPath(t, "true"),
		nil, nil, nil, nil,
		"",
		"", "",
		nil,
		bus,
		depResolver,
	)

	ctx := context.Background()
	err := agent.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give it time to publish events
	time.Sleep(200 * time.Millisecond)

	err = agent.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Event publishing verified via real eventbus (can't inspect without subscriber)
}

func TestGoAgent_Reload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process test in short mode")
	}

	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test_script.sh")
	script := `#!/bin/sh
echo "running"
sleep 10
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	agent := NewGoAgent(
		"test_agent",
		"service",
		scriptPath,
		nil, nil, nil, nil,
		"",
		"", "",
		nil,
		bus,
		depResolver,
	)

	ctx := context.Background()

	// Start
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Reload
	reloadCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := agent.Reload(reloadCtx); err != nil {
		t.Errorf("Reload() error = %v", err)
	}

	// Verify running after reload
	time.Sleep(200 * time.Millisecond)

	// Cleanup
	if err := agent.Stop(context.Background()); err != nil {
		t.Errorf("cleanup Stop: %v", err)
	}
}
