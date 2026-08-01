//go:build linux

package agentmgr

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/procsig"
)

// Stop on a CRIU-restored ("adopted") process, GAPI-DIV-046. Linux-only
// for the same reason core/procsig is: delivery uses pidfd_send_signal
// behind a /proc start-epoch guard and there is no fallback.
//
// These tests assert process state, which is identical for every caller,
// so there are no euid or root skip guards - and no PATH lookups either:
// the long-lived process is this test binary re-execed, so nothing here
// depends on a command being installed.

const adoptHelperEnv = "GAPI_AGENTMGR_ADOPT_HELPER"

// TestAdoptHelperProcess is the body of the child process the adopted
// tests spawn, not a test of its own (the os/exec self-exec pattern).
// Without the environment marker it returns immediately, so a normal run
// neither sleeps nor skips.
func TestAdoptHelperProcess(t *testing.T) {
	if os.Getenv(adoptHelperEnv) != "1" {
		return
	}
	time.Sleep(2 * time.Minute)
}

// adoptedChild is a long-lived process the TEST owns. Owning it is the
// point: the test can wait(2) on it, so "did Stop really terminate it"
// is answered by the kernel's exit status rather than by reimplementing
// liveness detection in the assertion.
type adoptedChild struct {
	cmd   *exec.Cmd
	pid   int
	epoch uint64
	done  chan struct{}
	err   error
}

func startAdoptedChild(t *testing.T) *adoptedChild {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locating test binary: %v", err)
	}
	cmd := exec.Command(exe, "-test.run=^TestAdoptHelperProcess$", "-test.timeout=0")
	cmd.Env = append(os.Environ(), adoptHelperEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting helper process: %v", err)
	}

	c := &adoptedChild{cmd: cmd, pid: cmd.Process.Pid, done: make(chan struct{})}
	go func() {
		c.err = cmd.Wait()
		close(c.done)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-c.done
	})

	epoch, err := procsig.StartEpoch(c.pid)
	if err != nil {
		t.Fatalf("StartEpoch(%d): %v", c.pid, err)
	}
	c.epoch = epoch
	return c
}

// exited reports whether the child has been reaped within d.
func (c *adoptedChild) exited(d time.Duration) bool {
	select {
	case <-c.done:
		return true
	case <-time.After(d):
		return false
	}
}

// adoptedAgent is the surface these tests exercise. GoAgent and
// PythonAgent are a parity contract, so every case runs against both.
type adoptedAgent interface {
	adopt(pid int) error
	Stop(ctx context.Context) error
	Pid() (int, bool)
	ID() string
}

var adoptedRunners = []struct {
	name     string
	newAgent func() adoptedAgent
}{
	{"GoAgent", func() adoptedAgent { return &GoAgent{id: "go-adopted"} }},
	{"PythonAgent", func() adoptedAgent { return &PythonAgent{id: "py-adopted"} }},
}

// storedEpoch reads back what adopt() recorded, and setStoredEpoch
// corrupts it to stage a recycled PID. Both reach into the runner
// structs directly, which an in-package test may do and which is the
// only way to stage a stale epoch against a process that is still alive.
func storedEpoch(t *testing.T, a adoptedAgent) uint64 {
	t.Helper()
	switch v := a.(type) {
	case *GoAgent:
		return v.adoptedEpoch
	case *PythonAgent:
		return v.adoptedEpoch
	}
	t.Fatalf("unknown runner type %T", a)
	return 0
}

func setStoredEpoch(t *testing.T, a adoptedAgent, epoch uint64) {
	t.Helper()
	switch v := a.(type) {
	case *GoAgent:
		v.adoptedEpoch = epoch
	case *PythonAgent:
		v.adoptedEpoch = epoch
	default:
		t.Fatalf("unknown runner type %T", a)
	}
}

func exitWaitStatus(err error) (syscall.WaitStatus, bool) {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return 0, false
	}
	ws, ok := ee.Sys().(syscall.WaitStatus)
	return ws, ok
}

// Stop must actually stop an adopted process. Before GAPI-DIV-046 it
// returned nil having done nothing: the caller was told the agent had
// stopped while it kept running.
func TestStopAdoptedProcessActuallyTerminatesIt(t *testing.T) {
	for _, r := range adoptedRunners {
		t.Run(r.name, func(t *testing.T) {
			child := startAdoptedChild(t)
			a := r.newAgent()
			if err := a.adopt(child.pid); err != nil {
				t.Fatalf("adopt(%d): %v", child.pid, err)
			}
			if got := storedEpoch(t, a); got != child.epoch {
				t.Fatalf("adopt stored epoch %d, want %d captured at adopt time", got, child.epoch)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := a.Stop(ctx); err != nil {
				t.Fatalf("Stop on adopted agent: %v", err)
			}

			if !child.exited(5 * time.Second) {
				t.Fatal("Stop reported success but the adopted process is still running")
			}
			ws, ok := exitWaitStatus(child.err)
			if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
				t.Fatalf("adopted process exit = %v, want terminated by SIGTERM", child.err)
			}
			if pid, running := a.Pid(); running {
				t.Fatalf("Pid still reports %d running after a successful Stop", pid)
			}
		})
	}
}

// A stored epoch that no longer matches means the PID was recycled: the
// process now holding it is a stranger, and the guard's whole purpose is
// that it receives nothing. Note this needs a LIVE process with a wrong
// stored epoch - letting the adopted process exit instead yields
// ErrProcessGone, which is a different path.
func TestStopAdoptedRefusesStaleEpochAndDeliversNothing(t *testing.T) {
	for _, r := range adoptedRunners {
		t.Run(r.name, func(t *testing.T) {
			child := startAdoptedChild(t)
			a := r.newAgent()
			if err := a.adopt(child.pid); err != nil {
				t.Fatalf("adopt(%d): %v", child.pid, err)
			}
			setStoredEpoch(t, a, child.epoch+12345)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := a.Stop(ctx)
			if !errors.Is(err, procsig.ErrStaleEpoch) {
				t.Fatalf("Stop with a stale stored epoch = %v, want ErrStaleEpoch", err)
			}

			if child.exited(250 * time.Millisecond) {
				t.Fatalf("refused delivery still killed the process: %v", child.err)
			}
		})
	}
}

// An adopted process that exited and was reaped before Stop is a
// successful stop, not a failure: Stop's contract is that the process is
// not running, and it is not. PID reuse cannot realistically intervene
// here - Linux allocates PIDs sequentially to pid_max, so wrapping
// within these few milliseconds would take millions of forks - and if it
// did, the epoch guard would refuse rather than signal the new occupant.
func TestStopAdoptedAlreadyGoneIsSuccess(t *testing.T) {
	for _, r := range adoptedRunners {
		t.Run(r.name, func(t *testing.T) {
			child := startAdoptedChild(t)
			a := r.newAgent()
			if err := a.adopt(child.pid); err != nil {
				t.Fatalf("adopt(%d): %v", child.pid, err)
			}

			// The supervisor never saw this exit; the test reaps it.
			if err := child.cmd.Process.Kill(); err != nil {
				t.Fatalf("killing helper: %v", err)
			}
			if !child.exited(5 * time.Second) {
				t.Fatal("helper process did not exit after SIGKILL")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := a.Stop(ctx); err != nil {
				t.Fatalf("Stop on an already-gone adopted process = %v, want nil", err)
			}
			if pid, running := a.Pid(); running {
				t.Fatalf("Pid still reports %d running after Stop", pid)
			}
		})
	}
}

// Adopted-claim expiry, GAPI-DIV-048.

// shortenAdoptedWatch makes the exit watcher poll fast enough that a test
// resolves in milliseconds. It is restored before the test returns; every
// watcher started under it is joined by then, and newAdoptedWatch reads
// the interval in the calling goroutine, so there is no race with the
// restore.
func shortenAdoptedWatch(t *testing.T) {
	t.Helper()
	prev := adoptedWatchInterval
	adoptedWatchInterval = 2 * time.Millisecond
	t.Cleanup(func() { adoptedWatchInterval = prev })
}

// waitForClaimCleared polls until the agent stops reporting a running
// process, or fails at the deadline. Polling rather than sleeping a fixed
// duration is deliberate: the watcher is asynchronous, and a fixed sleep
// would be a flake waiting to happen in either direction.
func waitForClaimCleared(t *testing.T, a adoptedAgent, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		pid, running := a.Pid()
		if !running {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("agent still claims pid %d as running %v after it exited; the claim never expired", pid, within)
		}
		time.Sleep(time.Millisecond)
	}
}

// recordSink captures slog records. NotifyExited only logs, so its log is
// the entire observable surface of an attribution decision; capturing it
// is what lets the test below assert on that decision without touching
// notify_exited.go.
type recordSink struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (s *recordSink) Enabled(context.Context, slog.Level) bool { return true }

func (s *recordSink) Handle(_ context.Context, r slog.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs = append(s.recs, r.Clone())
	return nil
}

func (s *recordSink) WithAttrs([]slog.Attr) slog.Handler { return s }
func (s *recordSink) WithGroup(string) slog.Handler      { return s }

// take drains what has been captured so far.
func (s *recordSink) take() []slog.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.recs
	s.recs = nil
	return out
}

func captureRecords(t *testing.T) *recordSink {
	t.Helper()
	s := &recordSink{}
	prev := slog.Default()
	slog.SetDefault(slog.New(s))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return s
}

// reapedAs reads NotifyExited's decision for pid out of the captured
// records: the agent id it attributed the reap to, or ok=false for the
// unknown-pid branch. Neither branch appearing is itself a failure - it
// would mean the assertion is measuring nothing.
func reapedAs(t *testing.T, recs []slog.Record, pid int) (string, bool) {
	t.Helper()
	for _, r := range recs {
		var agentID string
		var recPid int
		r.Attrs(func(at slog.Attr) bool {
			switch at.Key {
			case "agent_id":
				agentID = at.Value.String()
			case "pid":
				recPid = int(at.Value.Int64())
			}
			return true
		})
		if recPid != pid {
			continue
		}
		switch r.Message {
		case "agent process reaped":
			return agentID, true
		case "adopted orphan reaped":
			return "", false
		}
	}
	t.Fatalf("NotifyExited logged neither branch for pid %d; captured %d records", pid, len(recs))
	return "", false
}

// An adopted process that exits on its own must expire the agent's claim
// on its PID. Before GAPI-DIV-048 adoptedPid was cleared only by Stop, so
// an unattended exit left the agent reporting a dead PID as running
// forever - and Pid() is what NotifyExited matches on.
func TestAdoptedProcessExitExpiresTheClaim(t *testing.T) {
	for _, r := range adoptedRunners {
		t.Run(r.name, func(t *testing.T) {
			shortenAdoptedWatch(t)
			child := startAdoptedChild(t)
			a := r.newAgent()
			if err := a.adopt(child.pid); err != nil {
				t.Fatalf("adopt(%d): %v", child.pid, err)
			}
			if pid, running := a.Pid(); !running || pid != child.pid {
				t.Fatalf("after adopt Pid() = (%d, %v), want (%d, true)", pid, running, child.pid)
			}

			// The exit is unattended: nothing calls Stop, and the test -
			// not the supervisor - reaps the process.
			if err := child.cmd.Process.Kill(); err != nil {
				t.Fatalf("killing helper: %v", err)
			}
			if !child.exited(5 * time.Second) {
				t.Fatal("helper process did not exit after SIGKILL")
			}

			waitForClaimCleared(t, a, 10*time.Second)
		})
	}
}

// The point of the expiry: once the claim is gone, a later reap of that
// recycled PID must not be logged as this agent's exit under a wait
// status that belongs to a stranger.
func TestStaleAdoptedClaimIsNotAttributedByNotifyExited(t *testing.T) {
	for _, r := range adoptedRunners {
		t.Run(r.name, func(t *testing.T) {
			shortenAdoptedWatch(t)
			sink := captureRecords(t)
			child := startAdoptedChild(t)
			a := r.newAgent()
			if err := a.adopt(child.pid); err != nil {
				t.Fatalf("adopt(%d): %v", child.pid, err)
			}
			ag, ok := a.(Agent)
			if !ok {
				t.Fatalf("runner %T does not satisfy Agent", a)
			}
			am := &AgentManager{agents: map[string]Agent{ag.ID(): ag}}

			// Positive control. Without it a broken assertion that never
			// sees an attribution would pass the negative case for free.
			sink.take()
			am.NotifyExited(child.pid, syscall.WaitStatus(0))
			if id, attributed := reapedAs(t, sink.take(), child.pid); !attributed || id != ag.ID() {
				t.Fatalf("live claim: reap of pid %d attributed to (%q, %v), want (%q, true)", child.pid, id, attributed, ag.ID())
			}

			if err := child.cmd.Process.Kill(); err != nil {
				t.Fatalf("killing helper: %v", err)
			}
			if !child.exited(5 * time.Second) {
				t.Fatal("helper process did not exit after SIGKILL")
			}
			waitForClaimCleared(t, a, 10*time.Second)

			// Same PID, now recycled to somebody else. It is an unknown
			// pid to this manager, and must be logged as one.
			sink.take()
			am.NotifyExited(child.pid, syscall.WaitStatus(0))
			if id, attributed := reapedAs(t, sink.take(), child.pid); attributed {
				t.Fatalf("stale claim: reap of recycled pid %d was still logged as %q's exit", child.pid, id)
			}
		})
	}
}

// adopt() must fail loudly when it cannot establish the epoch: a restore
// whose process cannot be identified has not produced a supervisable
// agent, and Restore propagates that rather than swallowing it.
func TestAdoptWithoutEpochIsAnError(t *testing.T) {
	for _, r := range adoptedRunners {
		t.Run(r.name, func(t *testing.T) {
			a := r.newAgent()
			// PID 0 is never a signalable process from userspace.
			if err := a.adopt(0); !errors.Is(err, procsig.ErrProcessGone) {
				t.Fatalf("adopt(0) = %v, want ErrProcessGone", err)
			}
		})
	}
}
