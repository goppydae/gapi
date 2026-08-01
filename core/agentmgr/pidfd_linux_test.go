//go:build linux

package agentmgr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
)

// pidfdTargets maps each open pidfd in this process to the pid it
// pins, read from /proc/self/fdinfo/<fd> ("Pid:" line).
//
// This is how the handle is observable at all: os.Process keeps it in
// an unexported field, so reaching for it directly would mean
// reflecting on runtime internals and would break on a field rename
// without the guarantee having changed. The kernel's own view of our
// file descriptors cannot drift from reality that way.
func pidfdTargets(t *testing.T) map[string]int {
	t.Helper()
	out := map[string]int{}
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("read /proc/self/fd: %v", err)
	}
	for _, e := range ents {
		link, err := os.Readlink(filepath.Join("/proc/self/fd", e.Name()))
		if err != nil || link != "anon_inode:[pidfd]" {
			// Racy by nature: fds close under us as other tests finish.
			continue
		}
		info, err := os.ReadFile(filepath.Join("/proc/self/fdinfo", e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(info), "\n") {
			rest, ok := strings.CutPrefix(line, "Pid:")
			if !ok {
				continue
			}
			pid, err := strconv.Atoi(strings.TrimSpace(rest))
			if err == nil {
				out[e.Name()] = pid
			}
			break
		}
	}
	return out
}

// TestGoAgent_SpawnCarriesPidfd closes the standing half of
// GAPI-DIV-016.
//
// The entry's original claim - that core/agentmgr signals by PID with
// no epoch guard - turned out to be false on this toolchain.
// os.Process.signal dispatches to pidfd_send_signal whenever p.handle
// is non-nil, and os/exec binds that handle with pidfd_open at fork.
// That is a STRONGER guarantee than core/procsig's epoch comparison: a
// handle cannot refer to a recycled PID at all, whereas an epoch check
// compares two numbers and hopes they still mean something.
//
// What remained true is that the guarantee is INHERITED. Nothing in
// gapi asserted it. If p.handle is ever nil - an older kernel, or a
// toolchain that stops opening the pidfd - os falls back to pidSignal,
// a raw kill by PID, which is precisely the hazard this entry was
// opened about. The failure is silent: agents still start, stop still
// returns nil, and the race only shows up as a signal delivered to
// whatever process inherited the number.
//
// So the assertion is deliberately about the handle rather than about
// signalling working. Signalling appears to work in both cases; that
// is the whole problem.
func TestGoAgent_SpawnCarriesPidfd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	dir := t.TempDir()
	script := heartbeatScript(t, dir, filepath.Join(dir, "beats"))

	agent := NewGoAgent(
		"test_spawn_carries_pidfd",
		"service",
		script,
		nil, nil, nil, nil,
		"",
		"", "",
		nil,
		eventbus.NewInprocBus[*anypb.Any](),
		NewMockDependencyResolver(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = agent.Stop(stopCtx)
	})

	pid, ok := agent.Pid()
	if !ok || pid <= 0 {
		t.Fatalf("agent.Pid() = %d, %t; want a live pid", pid, ok)
	}

	// The handle is held for the child's lifetime and released by Wait,
	// so this must hold while the agent is running.
	targets := pidfdTargets(t)
	for _, target := range targets {
		if target == pid {
			return
		}
	}

	var found []string
	for fd, target := range targets {
		found = append(found, fmt.Sprintf("fd %s -> pid %d", fd, target))
	}
	if len(found) == 0 {
		found = append(found, "none at all")
	}
	t.Fatalf("agent pid %d is pinned by no pidfd in this process; signal delivery has "+
		"silently fallen back to kill-by-PID and the PID-recycling hazard of "+
		"GAPI-DIV-016 is open again. pidfds held: %s", pid, strings.Join(found, ", "))
}
