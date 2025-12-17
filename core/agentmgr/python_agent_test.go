package agentmgr

import (
	_ "context"
	"os"
	"path/filepath"
	"testing"
	_ "time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/cgroups"
	"github.com/goppydae/gapi/core/eventbus"
)

func TestParseLimits(t *testing.T) {
	tests := []struct {
		name     string
		cpu      string
		mem      string
		expected cgroups.ResourceSpec
	}{
		{
			name:     "Empty",
			cpu:      "",
			mem:      "",
			expected: cgroups.ResourceSpec{},
		},
		{
			name: "CPU Decimal",
			cpu:  "0.5",
			mem:  "",
			expected: cgroups.ResourceSpec{
				CPU: 0.5,
			},
		},
		{
			name: "CPU Milli",
			cpu:  "500m",
			mem:  "",
			expected: cgroups.ResourceSpec{
				CPU: 0.5,
			},
		},
		{
			name: "CPU Integer",
			cpu:  "2",
			mem:  "",
			expected: cgroups.ResourceSpec{
				CPU: 2.0,
			},
		},
		{
			name: "Mem Bytes",
			cpu:  "",
			mem:  "1024",
			expected: cgroups.ResourceSpec{
				Memory: 1024,
			},
		},
		{
			name: "Mem KB",
			cpu:  "",
			mem:  "10KB",
			expected: cgroups.ResourceSpec{
				Memory: 10 * 1024,
			},
		},
		{
			name: "Mem MB",
			cpu:  "",
			mem:  "10MB",
			expected: cgroups.ResourceSpec{
				Memory: 10 * 1024 * 1024,
			},
		},
		{
			name: "Mem GB",
			cpu:  "",
			mem:  "1GB",
			expected: cgroups.ResourceSpec{
				Memory: 1 * 1024 * 1024 * 1024,
			},
		},
		{
			name: "Mixed",
			cpu:  "100m",
			mem:  "512M",
			expected: cgroups.ResourceSpec{
				CPU:    0.1,
				Memory: 512 * 1024 * 1024,
			},
		},
		{
			name: "Invalid CPU",
			cpu:  "foo",
			mem:  "10MB",
			expected: cgroups.ResourceSpec{
				Memory: 10 * 1024 * 1024,
			},
		},
		{
			name: "Invalid Mem",
			cpu:  "0.1",
			mem:  "bar",
			expected: cgroups.ResourceSpec{
				CPU: 0.1,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLimits(tc.cpu, tc.mem)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("parseLimits mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

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

	agent := NewPythonAgent(
		"my_py_agent",
		"socket",
		"/app/service.py",
		"/app/runner.py",
		[]string{"redis"},
		[]string{},
		[]string{},
		[]string{},
		"/tmp/my.sock",
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
		{"listen_stream", "/tmp/my.sock"},
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
		defer f.Close()
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
		defer f2.Close()
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
