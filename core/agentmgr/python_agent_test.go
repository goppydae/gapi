package agentmgr

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
	protopkg "github.com/goppydae/gapi/pkg/proto"
)

func TestPythonAgent_Constructor(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	agent := NewPythonAgent(
		"test_py_agent",
		"service",
		"/path/to/module.py",
		"/path/to/runner.py",
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
		false,
	)

	if agent.ID() != "test_py_agent" {
		t.Errorf("ID() = %q, want %q", agent.ID(), "test_py_agent")
	}

	if agent.Type() != "service" {
		t.Errorf("Type() = %q, want %q", agent.Type(), "service")
	}

	if agent.Lang() != "python" {
		t.Errorf("Lang() = %q, want %q", agent.Lang(), "python")
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

func TestPythonAgent_Describe(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	sockPath := filepath.Join(t.TempDir(), "my.sock")
	agent := NewPythonAgent(
		"my_py_agent",
		"socket",
		"/app/service.py",
		"/app/runner.py",
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
		false,
	)

	desc := agent.Describe()

	tests := []struct {
		key  string
		want string
	}{
		{"id", "my_py_agent"},
		{"type", "socket"},
		{"language", "python"},
		{"path", "/app/service.py"},
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

func TestPythonAgent_Controller(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	agent := NewPythonAgent(
		"test_agent",
		"service",
		"/app/test.py",
		"/app/runner.py",
		nil, nil, nil, nil,
		"",
		"", "",
		nil,
		bus,
		depResolver,
		false,
	)

	ctrl := agent.Controller()
	if ctrl == nil {
		t.Error("Controller() returned nil")
	}
}

func TestPythonAgent_EnsureListener(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test_py.sock")

	agent := NewPythonAgent(
		"test_agent",
		"socket",
		"/app/test.py",
		"/app/runner.py",
		nil, nil, nil, nil,
		sockPath,
		"", "",
		nil,
		bus,
		depResolver,
		false,
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

func TestPythonAgent_GetString(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		wantVal  string
		wantBool bool
	}{
		{
			name:     "Key exists",
			m:        map[string]any{"foo": "bar"},
			key:      "foo",
			wantVal:  "bar",
			wantBool: true,
		},
		{
			name:     "Key missing",
			m:        map[string]any{"foo": "bar"},
			key:      "baz",
			wantVal:  "",
			wantBool: false,
		},
		{
			name:     "Wrong type",
			m:        map[string]any{"foo": 123},
			key:      "foo",
			wantVal:  "",
			wantBool: false,
		},
		{
			name:     "Empty map",
			m:        map[string]any{},
			key:      "foo",
			wantVal:  "",
			wantBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotBool := getString(tt.m, tt.key)
			if gotVal != tt.wantVal || gotBool != tt.wantBool {
				t.Errorf("getString() = (%q, %v), want (%q, %v)",
					gotVal, gotBool, tt.wantVal, tt.wantBool)
			}
		})
	}
}

func TestPythonAgent_PublishStatus(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	depResolver := NewMockDependencyResolver()

	agent := NewPythonAgent(
		"test_agent",
		"service",
		"/app/test.py",
		"/app/runner.py",
		nil, nil, nil, nil,
		"",
		"", "",
		nil,
		bus,
		depResolver,
		false,
	)

	// Publish a status
	agent.publishStatus("running", "Agent started successfully")

	// Verify event was published
}

// TestPythonAgent_UnexpectedExitReported mirrors the GoAgent watcher
// contract (GAPI-DIV-026): a service process that dies without Stop is
// reaped and reported FAILED - cross-ADK parity for exit handling.
func TestPythonAgent_UnexpectedExitReported(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not in PATH: %v", err)
	}

	bus := eventbus.NewInprocBus[*anypb.Any]()

	tmpDir := t.TempDir()
	runner := filepath.Join(tmpDir, "runner.py")
	if err := os.WriteFile(runner, []byte("import time\ntime.sleep(30)\n"), 0o644); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}

	agent := NewPythonAgent(
		"test_py_unexpected_exit", "service", "unused.py", runner,
		nil, nil, nil, nil, "", "", "", nil, bus, NewMockDependencyResolver(), false,
	)

	failed := make(chan string, 4)
	if err := bus.Subscribe("system", "", eventbus.TopicAgentLifecycleStatus, func(e eventbus.Event[*anypb.Any]) {
		var st protopkg.LifecycleStatus
		if e.Payload == nil || e.Payload.UnmarshalTo(&st) != nil {
			return
		}
		if st.AgentId == "test_py_unexpected_exit" && st.State == "FAILED" {
			failed <- st.Message
		}
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := agent.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	pid, running := agent.Pid()
	if !running {
		t.Fatal("agent not running after Start")
	}

	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill: %v", err)
	}

	select {
	case msg := <-failed:
		t.Logf("unexpected exit reported: %s", msg)
	case <-time.After(3 * time.Second):
		t.Fatal("no FAILED status within 3s of the process dying")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, running := agent.Pid(); !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent still reports a running process after unexpected exit")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestPythonAgent_StopDoesNotDoubleReportFailure: Stop-initiated exits
// stay quiet, exactly like the GoAgent watcher.
func TestPythonAgent_StopDoesNotDoubleReportFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 not in PATH: %v", err)
	}

	bus := eventbus.NewInprocBus[*anypb.Any]()

	tmpDir := t.TempDir()
	runner := filepath.Join(tmpDir, "runner.py")
	if err := os.WriteFile(runner, []byte("import time\ntime.sleep(30)\n"), 0o644); err != nil {
		t.Fatalf("write fake runner: %v", err)
	}

	agent := NewPythonAgent(
		"test_py_stop_no_double", "service", "unused.py", runner,
		nil, nil, nil, nil, "", "", "", nil, bus, NewMockDependencyResolver(), false,
	)

	failed := make(chan string, 4)
	if err := bus.Subscribe("system", "", eventbus.TopicAgentLifecycleStatus, func(e eventbus.Event[*anypb.Any]) {
		var st protopkg.LifecycleStatus
		if e.Payload == nil || e.Payload.UnmarshalTo(&st) != nil {
			return
		}
		if st.AgentId == "test_py_stop_no_double" && st.State == "FAILED" {
			failed <- st.Message
		}
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := agent.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := agent.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case msg := <-failed:
		t.Fatalf("Stop-initiated exit was reported as FAILED: %s", msg)
	case <-time.After(500 * time.Millisecond):
	}
}
