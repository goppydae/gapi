package logging_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/logging"
)

func TestKmsg_FormatAndPriority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kmsg")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("create fake kmsg: %v", err)
	}
	k := logging.NewKmsg(path)
	k.Log(logging.KmsgInfo, "phase 0 begins")
	k.Log(logging.KmsgErr, "mount failed")

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "<6>gapid: phase 0 begins\n<3>gapid: mount failed\n"
	if string(got) != want {
		t.Fatalf("kmsg content = %q, want %q", got, want)
	}
}

// kmsg has a 976-byte line limit; longer messages are truncated, not
// split or dropped.
func TestKmsg_LineLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kmsg")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("create fake kmsg: %v", err)
	}
	k := logging.NewKmsg(path)
	k.Log(logging.KmsgInfo, strings.Repeat("x", 2000))

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("long message produced %d lines, want 1", len(lines))
	}
	if len(lines[0]) > 976 {
		t.Fatalf("line length %d exceeds the 976-byte kmsg limit", len(lines[0]))
	}
}

// An unwritable device degrades silently: Phase 0 logging must never
// be able to fail the boot it is narrating. (A missing file at a
// writable path is created - injected paths in containers start
// absent.)
func TestKmsg_UnwritableDeviceIsSilent(t *testing.T) {
	k := logging.NewKmsg(filepath.Join(t.TempDir(), "no-such-dir", "kmsg"))
	k.Log(logging.KmsgInfo, "into the void") // must not panic
}
