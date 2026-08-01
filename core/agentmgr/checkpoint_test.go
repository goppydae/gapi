package agentmgr

import (
	"context"
	"errors"
	"testing"

	"github.com/goppydae/gapi/core/lifecycle"
)

// The capability must be discoverable by type assertion, because that
// is how an orchestrator decides at admission whether a workload can
// ever be migrated.
func TestProcessRunnersImplementCheckpointer(t *testing.T) {
	var _ lifecycle.Checkpointer = (*GoAgent)(nil)
	var _ lifecycle.Checkpointer = (*PythonAgent)(nil)
}

// TimerAgent runs in-process, so there is nothing to dump. Claiming the
// capability would make an orchestrator schedule a migration that could
// never complete, which is worse than declining it.
func TestTimerAgentDoesNotClaimCheckpointer(t *testing.T) {
	var a any = (*TimerAgent)(nil)
	if _, ok := a.(lifecycle.Checkpointer); ok {
		t.Fatal("TimerAgent claims Checkpointer but has no separate process to dump")
	}
}

// Asking to checkpoint a runner that is not running is the caller's
// mistake, and must be distinguishable from a CRIU failure so the
// orchestrator can tell "retry later" from "this host cannot".
func TestCheckpointWithoutProcessIsTyped(t *testing.T) {
	dir := t.TempDir()

	goAgent := &GoAgent{id: "go-1"}
	if err := goAgent.Checkpoint(context.Background(), dir); !errors.Is(err, ErrNoProcess) {
		t.Errorf("GoAgent: want ErrNoProcess, got %v", err)
	}

	pyAgent := &PythonAgent{id: "py-1"}
	if err := pyAgent.Checkpoint(context.Background(), dir); !errors.Is(err, ErrNoProcess) {
		t.Errorf("PythonAgent: want ErrNoProcess, got %v", err)
	}
}

// Adoption is what makes a restored process visible to the reap loop:
// NotifyExited matches agents by Pid(), so an adopted process that does
// not surface there would exit unnoticed.
func TestAdoptedPidIsReportedByPid(t *testing.T) {
	a := &GoAgent{id: "go-1"}
	if _, running := a.Pid(); running {
		t.Fatal("fresh agent reports a running process")
	}

	// The error is the epoch capture failing for a pid with no /proc
	// entry (GAPI-DIV-046); this test is about the pid being recorded
	// and reported regardless, which is what the reap loop matches on.
	_ = a.adopt(4242)

	pid, running := a.Pid()
	if !running {
		t.Fatal("adopted process is not reported as running")
	}
	if pid != 4242 {
		t.Fatalf("Pid = %d, want 4242", pid)
	}
}

func TestAdoptedPidIsReportedByPidPython(t *testing.T) {
	a := &PythonAgent{id: "py-1"}
	_ = a.adopt(4243)

	pid, running := a.Pid()
	if !running || pid != 4243 {
		t.Fatalf("Pid = (%d, %v), want (4243, true)", pid, running)
	}
}
