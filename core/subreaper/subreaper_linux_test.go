// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package subreaper_test

import (
	"context"
	"fmt"
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

// helperEnv puts this test binary into one of its double-fork roles.
// Empty means "run the tests".
const helperEnv = "GAPI_SUBREAPER_HELPER"

// TestMain lets the test binary re-exec itself as the two halves of a
// double fork, which is what replaced the shell these tests used to
// spawn (GAPI-DIV-043).
//
// The shell was the defect. `sh -c '( exit 3 ) & echo $!'` relies on the
// shell exiting BEFORE its own job-control SIGCHLD handler reaps the
// backgrounded subshell - and when the shell wins that race the orphan
// is reaped by its own parent, never reparents to the subreaper, and the
// test fails having proved nothing about the reap loop. Measured on the
// same nix-store bash the CI runner uses: 0 of 300 with the script as
// written, 9 of 300 when the shell lingers 10ms before exiting, and 300
// of 300 when it is told to `wait`. The flake was that race, resolving
// differently on a loaded runner.
//
// The middle role below can never do that, and not by timing: it calls
// Start and then exits, so there is no Wait anywhere for the grandchild
// to be collected by. The orphan reaches the subreaper because nothing
// else is entitled to it.
func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "middle":
		middleMain()
	case "grandchild":
		grandchildMain()
	default:
		os.Exit(m.Run())
	}
}

// middleMain is the disappearing parent: spawn the grandchild, print its
// pid, and exit WITHOUT waiting for it.
func middleMain() {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperEnv+"=grandchild")
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "spawn grandchild:", err)
		os.Exit(1)
	}
	fmt.Println(cmd.Process.Pid)
	os.Exit(0)
}

// grandchildMain is the orphan. GAPI_SUBREAPER_LINGER holds it alive
// long enough to be reparented while still running; without it the
// process exits immediately and is reparented as a zombie.
func grandchildMain() {
	if d := os.Getenv("GAPI_SUBREAPER_LINGER"); d != "" {
		delay, err := time.ParseDuration(d)
		if err != nil {
			fmt.Fprintln(os.Stderr, "parse linger:", err)
			os.Exit(1)
		}
		time.Sleep(delay)
		os.Exit(7)
	}
	os.Exit(3)
}

// spawnOrphan runs the middle role and returns the orphan's pid. Output
// waits for the middle, so on return the orphan has already been
// reparented - which is what makes the first probe meaningful.
func spawnOrphan(t *testing.T, linger string) (orphanTrace, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperEnv+"=middle")
	if linger != "" {
		cmd.Env = append(cmd.Env, "GAPI_SUBREAPER_LINGER="+linger)
	}
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		t.Fatalf("spawn double-forker: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse orphan pid from %q: %v", out, err)
	}
	trace := orphanTrace{Pid: pid, Intermediate: os.Args[0]}
	trace.probe("intermediate exited")
	return trace, pid
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

	// The middle spawns the orphan-to-be, which lingers 200ms and exits
	// 7, and the middle exits immediately - orphaning it onto us while
	// it is still running.
	trace, orphanPid := spawnOrphan(t, "200ms")

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

	// The orphan exits immediately, so it is already a zombie by the
	// time the middle exits and reparents it to us.
	//
	// The two probes bracket the window in which this test does nothing:
	// no reap loop is running yet, so anything that happens to the
	// orphan here is not ours. The first says whether it ever became our
	// child; the second whether it survived to the loop's first wait. By
	// the deadline both facts are gone, which is how the fifth
	// occurrence was diagnosed. GAPI-DIV-043.
	trace, orphanPid := spawnOrphan(t, "")
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
			// GAPI-DIV-043: this is where the flake landed five times.
			// The dump is what diagnosed it, and it stays: the premise
			// it checks - that an orphan in our subtree reaches us - is
			// only as good as the intermediate never reaping, and
			// nothing in this file's control flow enforces that for a
			// future harness.
			t.Fatalf("pre-existing zombie %d not drained%s",
				orphanPid, rec.report(trace))
		}
		time.Sleep(20 * time.Millisecond)
	}
}
