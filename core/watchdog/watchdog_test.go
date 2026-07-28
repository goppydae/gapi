package watchdog_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/watchdog"
)

// Kicks write keepalive bytes and Close performs the magic close
// ('V'), which is what tells the device to disarm rather than reset
// the machine when the supervisor exits deliberately.
func TestWatchdog_KickAndMagicClose(t *testing.T) {
	dev := filepath.Join(t.TempDir(), "watchdog")
	if err := os.WriteFile(dev, nil, 0o600); err != nil {
		t.Fatalf("create fake device: %v", err)
	}

	w, err := watchdog.Open(dev, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := w.Kick(); err != nil {
		t.Fatalf("Kick: %v", err)
	}
	if err := w.Kick(); err != nil {
		t.Fatalf("Kick: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := os.ReadFile(dev)
	if err != nil {
		t.Fatalf("read device: %v", err)
	}
	if string(got) != "11V" {
		t.Fatalf("device bytes = %q, want two kicks then magic close (11V)", got)
	}
}

// Run kicks on the interval until the context ends, then magic-closes.
func TestWatchdog_RunKeepalive(t *testing.T) {
	dev := filepath.Join(t.TempDir(), "watchdog")
	if err := os.WriteFile(dev, nil, 0o600); err != nil {
		t.Fatalf("create fake device: %v", err)
	}

	w, err := watchdog.Open(dev, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	time.Sleep(120 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after cancel")
	}

	got, err := os.ReadFile(dev)
	if err != nil {
		t.Fatalf("read device: %v", err)
	}
	s := string(got)
	if len(s) < 3 || s[len(s)-1] != 'V' {
		t.Fatalf("device bytes = %q, want multiple kicks ending in magic close", s)
	}
}

// A missing device is a configuration error, reported loudly.
func TestWatchdog_MissingDevice(t *testing.T) {
	if _, err := watchdog.Open(filepath.Join(t.TempDir(), "absent"), time.Second); err == nil {
		t.Fatal("Open of a missing device succeeded")
	}
}
