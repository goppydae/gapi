package logoutput

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goppydae/gapi/core/config"
)

// #1: setupFileOutput must derive the parent directory with filepath.Dir, not by
// slicing a byte off the path. For a nested path it should create the full parent
// directory and return a usable writer.
func TestSetupFileOutput_CreatesParentDir(t *testing.T) {
	base := t.TempDir()
	logPath := filepath.Join(base, "nested", "dir", "gapi.log")

	w, err := setupFileOutput(&config.FileOutputConfig{Path: logPath, MaxSize: 1})
	if err != nil {
		t.Fatalf("setupFileOutput failed: %v", err)
	}
	if w == nil {
		t.Fatal("expected a writer")
	}

	wantDir := filepath.Join(base, "nested", "dir")
	info, err := os.Stat(wantDir)
	if err != nil {
		t.Fatalf("expected parent dir %q to exist: %v", wantDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", wantDir)
	}

	// The buggy version would have created ".../gapi.lo" instead.
	if _, err := os.Stat(logPath[:len(logPath)-1]); err == nil {
		t.Fatalf("unexpected truncated directory %q was created", logPath[:len(logPath)-1])
	}
}

// #26: enabling Loki must surface an explicit error rather than silently
// returning os.Stderr.
func TestSetupOutputs_LokiEnabledErrors(t *testing.T) {
	cfg := &config.LoggingConfig{
		Loki: config.LokiOutputConfig{Enabled: true, URL: "http://loki:3100"},
	}
	if _, err := SetupOutputs(cfg); err == nil {
		t.Fatal("expected error when Loki is enabled but unimplemented, got nil")
	}
}
