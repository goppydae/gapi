package checkpoint_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/checkpoint"
)

// Available must report a TYPED reason or nothing at all. A caller uses
// this to decide whether a host can host migratable work, so an opaque
// error would force string matching at the decision point.
func TestAvailableReportsTypedReason(t *testing.T) {
	err := checkpoint.Available()
	if err == nil {
		return // capable host; nothing further to assert here
	}
	known := []error{
		checkpoint.ErrUnsupported,
		checkpoint.ErrNoCriu,
		checkpoint.ErrNotCapable,
	}
	for _, sentinel := range known {
		if errors.Is(err, sentinel) {
			return
		}
	}
	t.Fatalf("Available returned an untyped error: %v", err)
}

func TestErrorUnwrapsToSentinel(t *testing.T) {
	err := &checkpoint.Error{
		Op:  "dump",
		Pid: 4242,
		Dir: "/tmp/images",
		Err: checkpoint.ErrNotCapable,
	}
	if !errors.Is(err, checkpoint.ErrNotCapable) {
		t.Fatal("Error does not unwrap to its sentinel")
	}
}

// The message must name the subject and the directory: these failures
// are read from logs during a migration that is already going wrong.
func TestErrorMessageNamesSubjectAndDir(t *testing.T) {
	withPid := (&checkpoint.Error{
		Op: "dump", Pid: 4242, Dir: "/tmp/images", Err: checkpoint.ErrNotCapable,
	}).Error()
	for _, want := range []string{"dump", "4242", "/tmp/images"} {
		if !strings.Contains(withPid, want) {
			t.Errorf("message %q missing %q", withPid, want)
		}
	}

	// Restore has no subject pid; the message must not claim "pid 0".
	noPid := (&checkpoint.Error{
		Op: "restore", Dir: "/tmp/images", Err: checkpoint.ErrNotCapable,
	}).Error()
	if strings.Contains(noPid, "pid 0") {
		t.Errorf("restore message invents a pid: %q", noPid)
	}
}

// The zero Options must be the safe migration case. If LeaveRunning
// ever defaults true, a dump would leave the source executing past the
// point its image captured, and the image would stop being a sound
// rollback point the moment the destination restored.
func TestZeroOptionsLeaveRunningIsFalse(t *testing.T) {
	var opt checkpoint.Options
	if opt.LeaveRunning {
		t.Fatal("zero Options has LeaveRunning true; the source would survive its own dump")
	}
}
