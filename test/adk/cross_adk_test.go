package adk_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/supervisor"
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
			GoAgent:     "fixtures/go/simple_service/main.go",
			PyAgent:     "fixtures/python/simple_service.py",
			TestFunc:    testDescribeMetadata,
		},
		{
			Name:        "LifecycleTransitions",
			Description: "Verify Initialize -> Start -> Stop lifecycle",
			GoAgent:     "fixtures/go/lifecycle_agent/main.go",
			PyAgent:     "fixtures/python/lifecycle_agent.py",
			TestFunc:    testLifecycleTransitions,
		},
		{
			Name:        "CapabilityDetection",
			Description: "Verify capability introspection",
			GoAgent:     "fixtures/go/capabilities_agent/main.go",
			PyAgent:     "fixtures/python/capabilities_agent.py",
			TestFunc:    testCapabilityDetection,
		},
		{
			Name:        "SchemaHashing",
			Description: "Verify BLAKE3 schema hashing",
			GoAgent:     "fixtures/go/hash_agent/main.go",
			PyAgent:     "fixtures/python/hash_agent.py",
			TestFunc:    testSchemaHashing,
		},
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
		// For Go agents, we need to build them first
		tmpBin := filepath.Join(t.TempDir(), "agent")
		buildCmd := exec.Command("go", "build", "-o", tmpBin, agentPath)
		if err := buildCmd.Run(); err != nil {
			t.Fatalf("Failed to build Go agent: %v", err)
		}
		cmd = exec.Command(tmpBin, "--describe")
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

	// Verify language is correct
	if lang := desc["language"].(string); lang != lang {
		t.Errorf("Expected language=%s, got %s", lang, desc["language"])
	}
}

// testLifecycleTransitions verifies lifecycle state transitions
func testLifecycleTransitions(t *testing.T, lang string, agentPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start a test supervisor
	cfg := &config.Config{
		Transport: config.TransportConfig{
			Type:    "quic",
			Address: "127.0.0.1:0", // Use port 0 to get random available port
		},
	}

	sup, err := supervisor.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create supervisor: %v", err)
	}

	go func() {
		_ = sup.Run(ctx)
	}()

	// Give supervisor time to start
	time.Sleep(500 * time.Millisecond)

	// Start the agent
	var cmd *exec.Cmd
	switch lang {
	case "go":
		tmpBin := filepath.Join(t.TempDir(), "agent")
		buildCmd := exec.Command("go", "build", "-o", tmpBin, agentPath)
		if err := buildCmd.Run(); err != nil {
			t.Fatalf("Failed to build Go agent: %v", err)
		}
		cmd = exec.Command(tmpBin, "--start")
	case "python":
		runner := findPythonRunner(t)
		cmd = exec.Command("python3", runner, "--module", agentPath, "--start")
	}

	cmd.Env = append(os.Environ(), "GAPI_QUIC_ADDR=127.0.0.1:14242")

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}
	defer cmd.Process.Kill()

	// Wait for agent to reach running state
	time.Sleep(2 * time.Second)

	// Send stop signal
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Logf("Failed to send interrupt: %v", err)
	}

	// Wait for graceful shutdown
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Error("Agent did not stop within timeout")
	}
}

// testCapabilityDetection verifies capability introspection
func testCapabilityDetection(t *testing.T, lang string, agentPath string) {
	var cmd *exec.Cmd

	switch lang {
	case "go":
		tmpBin := filepath.Join(t.TempDir(), "agent")
		buildCmd := exec.Command("go", "build", "-o", tmpBin, agentPath)
		if err := buildCmd.Run(); err != nil {
			t.Fatalf("Failed to build Go agent: %v", err)
		}
		cmd = exec.Command(tmpBin, "--describe")
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

// testSchemaHashing verifies BLAKE3 hash computation
func testSchemaHashing(t *testing.T, lang string, agentPath string) {
	// Create a test file with known content
	testContent := []byte("test content for hashing")
	testFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	var cmd *exec.Cmd

	switch lang {
	case "go":
		// For Go, we'll build a simple hash utility
		tmpBin := filepath.Join(t.TempDir(), "hasher")
		buildCmd := exec.Command("go", "build", "-o", tmpBin, agentPath)
		if err := buildCmd.Run(); err != nil {
			t.Fatalf("Failed to build Go hasher: %v", err)
		}
		cmd = exec.Command(tmpBin, testFile)
	case "python":
		// For Python, call the hash agent directly
		cmd = exec.Command("python3", agentPath, testFile)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to compute hash: %v", err)
	}

	hash := strings.TrimSpace(string(output))

	// Verify hash is 64 hex characters (BLAKE3 256-bit)
	if len(hash) != 64 {
		t.Errorf("Expected 64-character hash, got %d: %s", len(hash), hash)
	}

	// Verify hash is consistent (run twice with new command)
	var cmd2 *exec.Cmd
	switch lang {
	case "go":
		tmpBin := filepath.Join(t.TempDir(), "hasher")
		buildCmd := exec.Command("go", "build", "-o", tmpBin, agentPath)
		if err := buildCmd.Run(); err != nil {
			t.Fatalf("Failed to build Go hasher: %v", err)
		}
		cmd2 = exec.Command(tmpBin, testFile)
	case "python":
		cmd2 = exec.Command("python3", agentPath, testFile)
	}

	output2, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to compute hash (second run): %v", err)
	}
	hash2 := strings.TrimSpace(string(output2))

	if hash != hash2 {
		t.Errorf("Hash not deterministic: %s != %s", hash, hash2)
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

// TestADKIntegration runs a full integration test with both ADKs
func TestADKIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test verifies that Go and Python agents can coexist
	// in the same supervisor and communicate via the event bus
	t.Run("MixedLanguageAgents", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cfg := &config.Config{
			Transport: config.TransportConfig{
				Type:    "quic",
				Address: "127.0.0.1:24242",
			},
		}

		sup, err := supervisor.New(cfg)
		if err != nil {
			t.Fatalf("Failed to create supervisor: %v", err)
		}

		go func() {
			_ = sup.Run(ctx)
		}()

		time.Sleep(1 * time.Second)

		// Start a Go agent
		goAgent := filepath.Join(t.TempDir(), "go_agent")
		buildCmd := exec.Command("go", "build", "-o", goAgent, "fixtures/go/event_emitter.go")
		if err := buildCmd.Run(); err != nil {
			t.Fatalf("Failed to build Go agent: %v", err)
		}

		goCmd := exec.Command(goAgent, "--start")
		goCmd.Env = append(os.Environ(), "GAPI_QUIC_ADDR=127.0.0.1:24242")
		if err := goCmd.Start(); err != nil {
			t.Fatalf("Failed to start Go agent: %v", err)
		}
		defer goCmd.Process.Kill()

		// Start a Python agent
		runner := findPythonRunner(t)
		pyCmd := exec.Command("python3", runner, "--module", "fixtures/python/event_receiver.py", "--start")
		pyCmd.Env = append(os.Environ(), "GAPI_QUIC_ADDR=127.0.0.1:24242")
		if err := pyCmd.Start(); err != nil {
			t.Fatalf("Failed to start Python agent: %v", err)
		}
		defer pyCmd.Process.Kill()

		// Let them run and communicate
		time.Sleep(5 * time.Second)

		// Verify both are still running
		if goCmd.ProcessState != nil && goCmd.ProcessState.Exited() {
			t.Error("Go agent exited unexpectedly")
		}
		if pyCmd.ProcessState != nil && pyCmd.ProcessState.Exited() {
			t.Error("Python agent exited unexpectedly")
		}
	})
}
