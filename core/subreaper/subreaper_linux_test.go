//go:build linux

package subreaper_test

import (
	"context"
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

	var mu sync.Mutex
	reaped := map[int]syscall.WaitStatus{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go subreaper.ReapLoop(ctx, sigchld, func(pid int, ws syscall.WaitStatus) {
		mu.Lock()
		reaped[pid] = ws
		mu.Unlock()
	})

	// The child backgrounds a subshell (the orphan-to-be) that exits 7,
	// prints its pid, and exits immediately - orphaning the subshell
	// onto us.
	out, err := exec.Command(binPath(t, "sh"), "-c",
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

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		ws, ok := reaped[orphanPid]
		mu.Unlock()
		if ok {
			if !ws.Exited() || ws.ExitStatus() != 7 {
				t.Fatalf("orphan %d wait status = %v, want exit 7", orphanPid, ws)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("orphan %d was not reaped within 5s", orphanPid)
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

	out, err := exec.Command(binPath(t, "sh"), "-c",
		"( exit 3 ) & echo $!").Output()
	if err != nil && len(out) == 0 {
		t.Fatalf("spawn: %v", err)
	}
	orphanPid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse pid: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // let the orphan die first

	sigchld := make(chan os.Signal, 8)
	signal.Notify(sigchld, syscall.SIGCHLD)
	defer signal.Stop(sigchld)

	var mu sync.Mutex
	reaped := map[int]syscall.WaitStatus{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go subreaper.ReapLoop(ctx, sigchld, func(pid int, ws syscall.WaitStatus) {
		mu.Lock()
		reaped[pid] = ws
		mu.Unlock()
	})

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		_, ok := reaped[orphanPid]
		mu.Unlock()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pre-existing zombie %d not drained", orphanPid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
