// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GAPI-DIV-070. A daemon on a non-default port is unreachable by its own
// control binary, because the port exists only in the daemon's process
// environment and no runtime address file is written anywhere.

// plantAddr writes an entry as if a daemon with that pid had published
// it, for the cases a single test process cannot produce for itself.
func plantAddr(t *testing.T, dir string, pid int, addr string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%d%s", pid, controlAddrExt))
	if err := os.WriteFile(path, []byte(addr+"\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// The user tier outranks the system one, so a developer's daemon does not
// have to write /run - which it cannot, unprivileged. This is the case
// the entry reproduced: a Paseo worktree daemon on 127.0.0.1:14307.
func TestControlAddrDirs_UserRuntimeDirOutranksRun(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/probe-run")

	dirs := ControlAddrDirs()
	if len(dirs) < 2 {
		t.Fatalf("want both a user and a system tier, got %v", dirs)
	}
	if want := filepath.Join("/tmp/probe-run", "gapi", controlAddrSubdir); dirs[0] != want {
		t.Errorf("highest tier = %q, want %q", dirs[0], want)
	}
	if want := filepath.Join("/run", "gapi", controlAddrSubdir); dirs[len(dirs)-1] != want {
		t.Errorf("lowest tier = %q, want %q", dirs[len(dirs)-1], want)
	}
}

// Under systemd there is no XDG_RUNTIME_DIR, and /run is then the only
// tier. A missing variable is not an error: it is the normal shape of a
// system daemon.
func TestControlAddrDirs_SystemDaemonHasRunOnly(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	mustUnset(t, "XDG_RUNTIME_DIR")

	dirs := ControlAddrDirs()
	want := filepath.Join("/run", "gapi", controlAddrSubdir)
	if len(dirs) != 1 || dirs[0] != want {
		t.Fatalf("system daemon tiers = %v, want exactly [%s]", dirs, want)
	}
}

// The round trip is the contract: what the daemon publishes is what the
// client reads, and the reader reports WHICH file answered so a failure
// can name it.
func TestWriteReadControlAddr_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	written, err := WriteControlAddr("127.0.0.1:14307")
	if err != nil {
		t.Fatalf("WriteControlAddr: %v", err)
	}
	if want := controlAddrFile(filepath.Join(dir, "gapi", controlAddrSubdir)); written != want {
		t.Errorf("wrote to %q, want %q", written, want)
	}

	addr, from, err := ReadControlAddr()
	if err != nil {
		t.Fatalf("ReadControlAddr: %v", err)
	}
	if addr != "127.0.0.1:14307" {
		t.Errorf("read %q, want 127.0.0.1:14307", addr)
	}
	if from != written {
		t.Errorf("reader credited %q, want %q", from, written)
	}
}

// No entry is the ordinary case - no daemon has run - and must be
// reported as "nothing to say" rather than as a failure, or every
// control invocation on a clean host starts with an error.
func TestReadControlAddr_AbsentIsNotAnError(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	addr, from, err := ReadControlAddr()
	if err != nil {
		t.Fatalf("a missing address dir is not an error, got %v", err)
	}
	if addr != "" || from != "" {
		t.Errorf("want no address and no source, got %q from %q", addr, from)
	}
}

// THE REGRESSION FOR THE DEFECT CI CAUGHT. The first version of this
// wrote ONE shared file, so two daemons on one host clobbered each
// other: `go test ./...` runs test/adk and test/e2e concurrently, both
// start a daemon, and a client dialled a port belonging to a daemon it
// had never heard of.
//
// One entry per pid means a second daemon cannot overwrite the first.
func TestWriteControlAddr_SecondDaemonDoesNotOverwriteTheFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	sub := filepath.Join(dir, "gapi", controlAddrSubdir)

	// A live peer: this process's own parent is alive by construction.
	other := os.Getppid()
	plantAddr(t, sub, other, "127.0.0.1:40301")

	mine, err := WriteControlAddr("127.0.0.1:14242")
	if err != nil {
		t.Fatalf("WriteControlAddr: %v", err)
	}

	live, err := LiveControlAddrs()
	if err != nil {
		t.Fatalf("LiveControlAddrs: %v", err)
	}
	if len(live) != 2 {
		t.Fatalf("a second daemon overwrote the first: %d live entries, want 2: %+v", len(live), live)
	}
	if _, err := os.Stat(mine); err != nil {
		t.Errorf("this daemon's own entry is missing: %v", err)
	}
}

// With two daemons up, a client given no address CANNOT know which is
// meant. Choosing one would be a coin flip that looks like a decision,
// so the reader refuses and names both.
func TestReadControlAddr_AmbiguousWhenTwoDaemonsAreLive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	sub := filepath.Join(dir, "gapi", controlAddrSubdir)

	plantAddr(t, sub, os.Getppid(), "127.0.0.1:40301")
	if _, err := WriteControlAddr("127.0.0.1:14242"); err != nil {
		t.Fatalf("WriteControlAddr: %v", err)
	}

	_, _, err := ReadControlAddr()
	var amb *ErrAmbiguousControlAddr
	if !errors.As(err, &amb) {
		t.Fatalf("two live daemons must be reported as ambiguous, got %v", err)
	}
	if len(amb.Candidates) != 2 {
		t.Fatalf("ambiguity names %d candidates, want 2: %+v", len(amb.Candidates), amb.Candidates)
	}
	for _, want := range []string{"127.0.0.1:40301", "127.0.0.1:14242"} {
		if !strings.Contains(amb.Error(), want) {
			t.Errorf("the ambiguity message does not name %s: %s", want, amb.Error())
		}
	}
}

// A daemon killed with SIGKILL never removes its entry. Keying by pid
// makes that DETECTABLE rather than merely reportable: the reader skips
// it, so one crash does not misdirect every later client.
func TestLiveControlAddrs_SkipsAnEntryWhoseProcessIsGone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	sub := filepath.Join(dir, "gapi", controlAddrSubdir)

	// A pid the kernel rejects outright, so it can never be running.
	plantAddr(t, sub, 0x7FFFFFFF, "127.0.0.1:9")
	if _, err := WriteControlAddr("127.0.0.1:14242"); err != nil {
		t.Fatalf("WriteControlAddr: %v", err)
	}

	addr, _, err := ReadControlAddr()
	if err != nil {
		t.Fatalf("a stale entry must not make the read ambiguous or failed: %v", err)
	}
	if addr != "127.0.0.1:14242" {
		t.Errorf("read %q, want the live daemon's 127.0.0.1:14242", addr)
	}
}

// Shutdown removes only THIS process's entry. The shared-file version
// removed every tier's file, so one daemon stopping unpublished another
// that was still running.
func TestRemoveControlAddr_LeavesOtherDaemonsPublished(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	sub := filepath.Join(dir, "gapi", controlAddrSubdir)

	peer := plantAddr(t, sub, os.Getppid(), "127.0.0.1:40301")
	if _, err := WriteControlAddr("127.0.0.1:14242"); err != nil {
		t.Fatalf("WriteControlAddr: %v", err)
	}

	if err := RemoveControlAddr(); err != nil {
		t.Fatalf("RemoveControlAddr: %v", err)
	}
	if _, err := os.Stat(peer); err != nil {
		t.Fatalf("shutdown unpublished another live daemon: %v", err)
	}

	addr, _, err := ReadControlAddr()
	if err != nil {
		t.Fatalf("ReadControlAddr: %v", err)
	}
	if addr != "127.0.0.1:40301" {
		t.Errorf("the surviving daemon reads back as %q, want 127.0.0.1:40301", addr)
	}
}

// RemoveControlAddr is also the shutdown half for a daemon that failed
// before it ever published, so absence must succeed.
func TestRemoveControlAddr_IdempotentWhenAbsent(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if err := RemoveControlAddr(); err != nil {
		t.Fatalf("removing an absent address entry must succeed, got %v", err)
	}
}
