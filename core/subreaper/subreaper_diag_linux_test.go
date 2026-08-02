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
	"testing"
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
//
// SECOND PASS, after the fourth occurrence turned out to be diagnostic.
// That failure showed wait4(-1) returning ECHILD at event 0 with the
// subreaper flag set and the orphan already gone from /proc, which says
// the orphan never entered this process's child set at all - a
// reparenting failure rather than a reaping one. Everything above reads
// the orphan AFTER the deadline, by which time whatever happened to it
// has already been erased. So this pass adds two things the first one
// could not have: snapshots of the orphan taken on the HAPPY path at
// the moments the answer still exists (orphanTrace), and the /proc
// ancestor chain of this process, which names the ancestors that could
// have taken the orphan instead. The probes are the only instrumentation
// here that runs on a green run; both sit inside windows the test
// already spends sleeping.

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

// procStatus is the parsed head of /proc/<pid>/stat.
type procStatus struct {
	// Pid is the process this snapshot describes, carried so a chain of
	// them reads without an index.
	Pid int
	// Present is false when /proc/<pid> is gone, meaning the process
	// has been fully reaped by somebody.
	Present bool
	// Comm is the executable name from the parenthesised stat field. It
	// costs nothing extra to parse and it is what makes an ancestor
	// chain legible: "which ancestor" is a question about identity, not
	// about pid numbers that differ every run.
	Comm string
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

// String renders one snapshot as a single diagnostic line.
func (st procStatus) String() string {
	switch {
	case !st.Present:
		return fmt.Sprintf("%d GONE from /proc", st.Pid)
	case st.Err != nil:
		return fmt.Sprintf("%d UNREADABLE: %v", st.Pid, st.Err)
	default:
		return fmt.Sprintf("%d (%s) state=%s ppid=%d", st.Pid, st.Comm, st.State, st.PPid)
	}
}

// readProcStatus snapshots /proc/<pid>/stat.
func readProcStatus(pid int) procStatus {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		if os.IsNotExist(err) {
			return procStatus{Pid: pid, Present: false}
		}
		return procStatus{Pid: pid, Present: true, Err: err}
	}
	return parseProcStat(pid, string(raw))
}

// parseProcStat parses a /proc/<pid>/stat line. The comm field is
// wrapped in parentheses and may itself contain spaces and parentheses,
// so it is taken between the FIRST '(' and the LAST ')', and the fixed
// fields after it are taken relative to that last ')' rather than by
// splitting the whole line.
//
// It is split from the read so it can be tested. This parser only ever
// runs on a failure path that has never once occurred locally, which
// means a defect in it would be discovered by producing a garbage dump
// on the single CI occurrence the whole entry is waiting for - the flake
// would be spent and still unexplained. The diagnostic is the artifact
// here, so it gets the same treatment as production code.
func parseProcStat(pid int, line string) procStatus {
	commStart := strings.Index(line, "(")
	commEnd := strings.LastIndex(line, ")")
	if commStart < 0 || commEnd < commStart {
		return procStatus{Pid: pid, Present: true, Err: fmt.Errorf("no comm field in %q", line)}
	}
	st := procStatus{Pid: pid, Present: true, Comm: line[commStart+1 : commEnd]}
	fields := strings.Fields(line[commEnd+1:])
	if len(fields) < 2 {
		st.Err = fmt.Errorf("short stat tail %q", line[commEnd+1:])
		return st
	}
	st.State = fields[0]
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		st.Err = fmt.Errorf("parse ppid %q: %w", fields[1], err)
		return st
	}
	st.PPid = ppid
	return st
}

// ancestry walks the /proc parent chain from pid upward, so a dump names
// the runner's process tree rather than only this process's place in it.
//
// The surviving hypothesis is that some ancestor other than us takes the
// orphan. The child-subreaper flag of another process cannot be read -
// prctl is self-only, and the flag is not exposed in /proc (checked) -
// so the chain plus each comm is as close as this gets: it identifies
// the candidates by name and lets the log be compared against the
// runner's known supervision, which the failing run reported as
// system.slice/hosted-compute-agent.service.
func ancestry(pid int) []procStatus {
	// A malformed or racing /proc could otherwise loop; the chain on any
	// real host is single digits deep.
	const maxDepth = 32
	chain := make([]procStatus, 0, 8)
	for range maxDepth {
		st := readProcStatus(pid)
		chain = append(chain, st)
		if !st.Present || st.Err != nil || st.PPid <= 0 || st.PPid == st.Pid {
			break
		}
		pid = st.PPid
	}
	return chain
}

// orphanProbe is a /proc snapshot of the orphan taken at a named moment
// on the HAPPY path, before anything has failed.
type orphanProbe struct {
	// At names the moment: what had just happened when this was taken.
	At string
	// Status is the snapshot.
	Status procStatus
}

// orphanTrace accumulates what the test knew about its orphan before the
// reap loop's own record begins.
//
// The fourth occurrence found the orphan already GONE from /proc at the
// deadline, so the facts that would name what happened to it - was it
// ever ours, and if not whose was it - had been destroyed three seconds
// before anything looked. A probe taken while the answer still exists is
// the only way to carry it into the dump.
type orphanTrace struct {
	// Pid is the orphan, as reported by the shell's $!.
	Pid int
	// Shell is the resolved sh the orphan was spawned from. Whether the
	// intermediate could have reaped its own background job before
	// exiting - which would also produce ECHILD with no orphan in /proc,
	// and without any reparenting failure - depends on which shell this
	// is, and the runner's sh is not this host's.
	Shell string

	probes []orphanProbe
}

// probe snapshots the orphan now, labelled with what has just happened.
func (o *orphanTrace) probe(at string) {
	o.probes = append(o.probes, orphanProbe{At: at, Status: readProcStatus(o.Pid)})
}

// The stat parser must survive a comm containing spaces and
// parentheses, because that is the field an ancestor on a CI runner is
// most likely to have and the least likely to be seen locally.
func TestParseProcStat(t *testing.T) {
	// A naive strings.Fields split of the whole line reads the second
	// and third whitespace-separated tokens as comm and state, so for
	// the adversarial case it yields comm="(my" state="(weird)" ppid=0
	// - every wanted value below disagrees with that.
	tests := []struct {
		name  string
		line  string
		comm  string
		state string
		ppid  int
	}{
		{
			name:  "ordinary",
			line:  "432495 (sh) Z 432449 432288 0 -1 0 4194560\n",
			comm:  "sh",
			state: "Z",
			ppid:  432449,
		},
		{
			name:  "comm with spaces and parens",
			line:  "1234 (my (weird) proc) S 99 1 0 -1 0 0\n",
			comm:  "my (weird) proc",
			state: "S",
			ppid:  99,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := parseProcStat(1234, tc.line)
			if st.Err != nil {
				t.Fatalf("parseProcStat: %v", st.Err)
			}
			if st.Comm != tc.comm || st.State != tc.state || st.PPid != tc.ppid {
				t.Errorf("comm/state/ppid = %q/%q/%d, want %q/%q/%d",
					st.Comm, st.State, st.PPid, tc.comm, tc.state, tc.ppid)
			}
		})
	}
}

// A pid that cannot exist must read as GONE rather than as an error,
// because GONE is the branch the fourth occurrence took and the dump
// distinguishes "somebody reaped it" from "we could not look" by it.
func TestReadProcStatus_AbsentPid(t *testing.T) {
	// One past the kernel's maximum is never allocated, so this needs no
	// process and carries no pid-reuse race.
	raw, err := os.ReadFile("/proc/sys/kernel/pid_max")
	if err != nil {
		t.Skipf("pid_max unreadable: %v", err)
	}
	pidMax, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse pid_max %q: %v", raw, err)
	}

	st := readProcStatus(pidMax + 1)
	if st.Present || st.Err != nil {
		t.Fatalf("pid %d: present=%t err=%v, want absent with no error",
			pidMax+1, st.Present, st.Err)
	}
	if got := st.String(); !strings.Contains(got, "GONE") {
		t.Errorf("String() = %q, want it to say GONE", got)
	}
}

// The ancestor walk must terminate and must report a chain the kernel
// agrees with. os.Getppid is an independent read of the same fact, so
// this fails if the ppid field is being taken from the wrong column.
func TestAncestry(t *testing.T) {
	chain := ancestry(os.Getpid())
	if len(chain) < 2 {
		t.Fatalf("chain = %v, want at least this process and its parent", chain)
	}
	if chain[0].Pid != os.Getpid() {
		t.Errorf("chain[0].Pid = %d, want %d", chain[0].Pid, os.Getpid())
	}
	if chain[0].PPid != os.Getppid() {
		t.Errorf("chain[0].PPid = %d, want os.Getppid() = %d", chain[0].PPid, os.Getppid())
	}
	if chain[1].Pid != os.Getppid() {
		t.Errorf("chain[1].Pid = %d, want the walk to step to %d", chain[1].Pid, os.Getppid())
	}
	last := chain[len(chain)-1]
	if last.Present && last.Err == nil && last.PPid > 0 && last.PPid != last.Pid {
		t.Errorf("walk stopped at %s with a live parent still to visit - depth cap hit", last)
	}
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
// reaped. o carries what the test observed before the failure, events is
// every Wait4 outcome the loop saw, in order, and reaped is what did get
// delivered to notify.
func diagnose(o orphanTrace, events []subreaper.DrainEvent, reaped map[int]syscall.WaitStatus) string {
	self := os.Getpid()
	var b strings.Builder
	fmt.Fprintf(&b, "\n--- GAPI-DIV-043 diagnostics ---\n")
	fmt.Fprintf(&b, "self pid: %d\n", self)
	fmt.Fprintf(&b, "orphan spawned from shell: %s\n", o.Shell)

	flag, err := isSubreaper()
	if err != nil {
		fmt.Fprintf(&b, "self child_subreaper: UNREADABLE: %v\n", err)
	} else {
		fmt.Fprintf(&b, "self child_subreaper: %t\n", flag)
	}

	// The happy-path probes first: they are the only lines here that can
	// still distinguish "reparented elsewhere" from "reaped before we
	// were ever a candidate", and by dump time both look like GONE.
	fmt.Fprintf(&b, "orphan probes (%d):\n", len(o.probes))
	for _, p := range o.probes {
		reparented := ""
		if p.Status.Present && p.Status.Err == nil {
			reparented = fmt.Sprintf(" (reparented to us: %t)", p.Status.PPid == self)
		}
		fmt.Fprintf(&b, "  %-34s %s%s\n", p.At, p.Status, reparented)
	}

	st := readProcStatus(o.Pid)
	switch {
	case !st.Present:
		fmt.Fprintf(&b, "orphan %d at failure: GONE from /proc - reaped by somebody, not delivered to notify\n", o.Pid)
	case st.Err != nil:
		fmt.Fprintf(&b, "orphan %d at failure: /proc/%d/stat unreadable: %v\n", o.Pid, o.Pid, st.Err)
	default:
		fmt.Fprintf(&b, "orphan %d at failure: %s (reparented to us: %t)\n",
			o.Pid, st, st.PPid == self)
	}

	fmt.Fprintf(&b, "our ancestry (nearest first):\n")
	for _, a := range ancestry(self) {
		fmt.Fprintf(&b, "  %s\n", a)
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
