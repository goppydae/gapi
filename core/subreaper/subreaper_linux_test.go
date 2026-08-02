//go:build linux

package subreaper_test

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/subreaper"
)

// binPath resolves a binary from PATH: hardcoded FHS paths differ
// between sandboxes and NixOS hosts.
func binPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not in PATH: %v", name, err)
	}
	return p
}

// reapRecorder collects what the loop delivered and what it saw while
// delivering it. Both halves are read from the test goroutine on the
// failure path, so both are guarded by the same mutex.
type reapRecorder struct {
	mu     sync.Mutex
	reaped map[int]syscall.WaitStatus
	events []subreaper.DrainEvent
}

func newReapRecorder() *reapRecorder {
	return &reapRecorder{reaped: map[int]syscall.WaitStatus{}}
}

func (r *reapRecorder) notify(pid int, ws syscall.WaitStatus) {
	r.mu.Lock()
	r.reaped[pid] = ws
	r.mu.Unlock()
}

func (r *reapRecorder) observe(ev subreaper.DrainEvent) {
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
}

// status reports what the loop delivered for pid, if anything.
func (r *reapRecorder) status(pid int) (syscall.WaitStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ws, ok := r.reaped[pid]
	return ws, ok
}

// report renders the GAPI-DIV-043 failure dump from a consistent
// snapshot of both halves, against what the test observed on the way in.
func (r *reapRecorder) report(o orphanTrace) string {
	r.mu.Lock()
	events := make([]subreaper.DrainEvent, len(r.events))
	copy(events, r.events)
	reaped := make(map[int]syscall.WaitStatus, len(r.reaped))
	maps.Copy(reaped, r.reaped)
	r.mu.Unlock()
	return diagnose(o, events, reaped)
}

// A double-forked orphan is reparented to the subreaper and reaped by
// the reap loop with its true wait status - the PID-1 obligation,
// testable unprivileged because any process may be a subreaper.
func TestReapLoop_ReapsOrphanWithStatus(t *testing.T) {
	if err := subreaper.BecomeSubreaper(); err != nil {
		t.Fatalf("BecomeSubreaper: %v", err)
	}

	sigchld := make(chan os.Signal, 8)
	signal.Notify(sigchld, syscall.SIGCHLD)
	defer signal.Stop(sigchld)

	rec := newReapRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go subreaper.ReapLoopWithObserver(ctx, sigchld, rec.notify, rec.observe)

	// The child backgrounds a subshell (the orphan-to-be) that exits 7,
	// prints its pid, and exits immediately - orphaning the subshell
	// onto us.
	trace := orphanTrace{Shell: binPath(t, "sh")}
	out, err := exec.Command(trace.Shell, "-c",
		"( sleep 0.2; exit 7 ) & echo $!").Output()
	if err != nil {
		// The reap loop may collect the direct child before Output's
		// own wait does; ECHILD here is expected, not a failure.
		if !strings.Contains(err.Error(), "no child processes") && len(out) == 0 {
			t.Fatalf("spawn double-forker: %v", err)
		}
	}
	orphanPid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse orphan pid from %q: %v", out, err)
	}
	// Output waited for the intermediate, so reparenting has already
	// happened and the orphan is still sleeping: this reads the
	// reparenting target while it is a live process rather than a
	// memory. GAPI-DIV-043.
	trace.Pid = orphanPid
	trace.probe("intermediate exited")

	deadline := time.Now().Add(5 * time.Second)
	for {
		ws, ok := rec.status(orphanPid)
		if ok {
			if !ws.Exited() || ws.ExitStatus() != 7 {
				t.Fatalf("orphan %d wait status = %v, want exit 7%s",
					orphanPid, ws, rec.report(trace))
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("orphan %d was not reaped within 5s%s",
				orphanPid, rec.report(trace))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The loop must drain zombies that died before it started (a missed
// SIGCHLD edge must not strand a zombie).
func TestReapLoop_DrainsPreexistingZombies(t *testing.T) {
	if err := subreaper.BecomeSubreaper(); err != nil {
		t.Fatalf("BecomeSubreaper: %v", err)
	}

	trace := orphanTrace{Shell: binPath(t, "sh")}
	out, err := exec.Command(trace.Shell, "-c",
		"( exit 3 ) & echo $!").Output()
	if err != nil && len(out) == 0 {
		t.Fatalf("spawn: %v", err)
	}
	orphanPid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse pid: %v", err)
	}
	// The two probes bracket the window in which this test does nothing,
	// which is the window the fourth occurrence's evidence points at: no
	// reap loop is running yet, so anything that happens to the orphan
	// here is not ours. The first says whether it ever became our child;
	// the second whether it survived to the loop's first wait. By the
	// deadline both facts are gone. GAPI-DIV-043.
	trace.Pid = orphanPid
	trace.probe("intermediate exited")
	time.Sleep(200 * time.Millisecond) // let the orphan die first
	trace.probe("after the 200ms wait, pre-loop")

	sigchld := make(chan os.Signal, 8)
	signal.Notify(sigchld, syscall.SIGCHLD)
	defer signal.Stop(sigchld)

	rec := newReapRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go subreaper.ReapLoopWithObserver(ctx, sigchld, rec.notify, rec.observe)

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, ok := rec.status(orphanPid); ok {
			return
		}
		if time.Now().After(deadline) {
			// GAPI-DIV-043: this is the flake. The dump is the whole
			// point of the entry's exit - it has never reproduced
			// locally, so this text is the only evidence there will be.
			t.Fatalf("pre-existing zombie %d not drained%s",
				orphanPid, rec.report(trace))
		}
		time.Sleep(20 * time.Millisecond)
	}
}
