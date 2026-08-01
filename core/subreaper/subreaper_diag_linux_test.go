//go:build linux

package subreaper_test

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/goppydae/gapi/core/subreaper"
)

// This file is the instrumentation GAPI-DIV-043's exit requires. That
// entry records a flake that has never reproduced locally - 0 failures
// in 260 runs against roughly 8 percent on a CI runner - so the
// diagnosis has to be committed and then read out of a CI log. Every
// field below is one the exit names: the Wait4 errno, whether the pid
// still exists, its /proc state and parent pid, and this process's own
// subreaper flag.
//
// It is deliberately allocation-cheap and runs only on the failure
// path. A green run must cost nothing observable, because perturbing
// the timing of an intermittent failure can make it stop happening
// before it has been explained.

// isSubreaper reports whether this process currently carries the child
// subreaper flag. PR_GET_CHILD_SUBREAPER writes a C int through the
// arg2 pointer rather than returning it, hence the int32 (exact kernel
// width) and the KeepAlive across the uintptr conversion.
//
// This lives in test code rather than beside BecomeSubreaper on
// purpose. Reading the flag needs unsafe, which is gapi's only
// hand-written use of it; putting it in the package would trip gosec
// G103 and the only repo-wide answer available is excluding G103
// everywhere - muting a rule that currently has zero findings, for a
// diagnostic. The flag is not in /proc (checked: no subreaper field in
// /proc/<pid>/status), so prctl is the only read path there is.
func isSubreaper() (bool, error) {
	var flag int32
	err := unix.Prctl(unix.PR_GET_CHILD_SUBREAPER, uintptr(unsafe.Pointer(&flag)), 0, 0, 0)
	runtime.KeepAlive(&flag)
	if err != nil {
		return false, err
	}
	return flag != 0, nil
}

// procStatus is the parsed third and fourth fields of /proc/<pid>/stat.
type procStatus struct {
	// Present is false when /proc/<pid> is gone, meaning the process
	// has been fully reaped by somebody.
	Present bool
	// State is the single-letter kernel state: Z is a zombie awaiting
	// a wait, R or S mean it has not exited at all.
	State string
	// PPid is the parent. If this is not our pid, reparenting to the
	// subreaper never happened and the reap loop was never entitled to
	// collect it.
	PPid int
	// Err records why the read or parse failed, when it did.
	Err error
}

// readProcStatus parses /proc/<pid>/stat. The comm field is wrapped in
// parentheses and may itself contain spaces and parentheses, so the
// fixed fields are taken relative to the LAST ')' rather than by
// splitting the whole line.
func readProcStatus(pid int) procStatus {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		if os.IsNotExist(err) {
			return procStatus{Present: false}
		}
		return procStatus{Present: true, Err: err}
	}
	line := string(raw)
	commEnd := strings.LastIndex(line, ")")
	if commEnd < 0 {
		return procStatus{Present: true, Err: fmt.Errorf("no comm terminator in %q", line)}
	}
	fields := strings.Fields(line[commEnd+1:])
	if len(fields) < 2 {
		return procStatus{Present: true, Err: fmt.Errorf("short stat tail %q", line[commEnd+1:])}
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return procStatus{Present: true, State: fields[0], Err: fmt.Errorf("parse ppid %q: %w", fields[1], err)}
	}
	return procStatus{Present: true, State: fields[0], PPid: ppid}
}

// formatStatus renders a wait status without making the reader decode
// the packed integer.
func formatStatus(ws syscall.WaitStatus) string {
	switch {
	case ws.Exited():
		return fmt.Sprintf("exited(%d)", ws.ExitStatus())
	case ws.Signaled():
		return fmt.Sprintf("signaled(%v)", ws.Signal())
	case ws.Stopped():
		return fmt.Sprintf("stopped(%v)", ws.StopSignal())
	default:
		return fmt.Sprintf("raw(0x%x)", int(ws))
	}
}

// diagnose renders the full failure report for an orphan that was not
// reaped. events is every Wait4 outcome the loop saw, in order; reaped
// is what did get delivered to notify.
func diagnose(orphanPid int, events []subreaper.DrainEvent, reaped map[int]syscall.WaitStatus) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n--- GAPI-DIV-043 diagnostics ---\n")
	fmt.Fprintf(&b, "self pid: %d\n", os.Getpid())

	flag, err := isSubreaper()
	if err != nil {
		fmt.Fprintf(&b, "self child_subreaper: UNREADABLE: %v\n", err)
	} else {
		fmt.Fprintf(&b, "self child_subreaper: %t\n", flag)
	}

	st := readProcStatus(orphanPid)
	switch {
	case !st.Present:
		fmt.Fprintf(&b, "orphan %d: GONE from /proc - reaped by somebody, not delivered to notify\n", orphanPid)
	case st.Err != nil:
		fmt.Fprintf(&b, "orphan %d: /proc/%d/stat unreadable: %v\n", orphanPid, orphanPid, st.Err)
	default:
		fmt.Fprintf(&b, "orphan %d: state=%s ppid=%d (reparented to us: %t)\n",
			orphanPid, st.State, st.PPid, st.PPid == os.Getpid())
	}

	pids := make([]int, 0, len(reaped))
	for pid := range reaped {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	fmt.Fprintf(&b, "delivered to notify (%d): ", len(pids))
	if len(pids) == 0 {
		fmt.Fprintf(&b, "none - the loop reaped nothing at all\n")
	} else {
		parts := make([]string, 0, len(pids))
		for _, pid := range pids {
			parts = append(parts, fmt.Sprintf("%d=%s", pid, formatStatus(reaped[pid])))
		}
		fmt.Fprintf(&b, "%s\n", strings.Join(parts, " "))
	}

	fmt.Fprintf(&b, "wait4 events (%d):\n", len(events))
	for i, ev := range events {
		errno := "nil"
		if ev.Err != nil {
			errno = ev.Err.Error()
		}
		status := "-"
		if ev.Pid > 0 {
			status = formatStatus(ev.Status)
		}
		fmt.Fprintf(&b, "  %2d %-7s pid=%-7d status=%-12s err=%s\n",
			i, ev.Trigger, ev.Pid, status, errno)
	}
	fmt.Fprintf(&b, "--- end diagnostics ---\n")
	return b.String()
}
