// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// GAPI-DIV-070. A daemon on a non-default port is unreachable by its own
// control binary, because the port exists only in the daemon's process
// environment and no runtime address file is written anywhere.

// The user tier outranks the system one, so a developer's daemon does not
// have to write /run - which it cannot, unprivileged. This is the case
// the entry reproduced: a Paseo worktree daemon on 127.0.0.1:14307.
func TestControlAddrFiles_UserRuntimeDirOutranksRun(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/probe-run")

	paths := ControlAddrFiles()
	if len(paths) < 2 {
		t.Fatalf("want both a user and a system tier, got %v", paths)
	}
	if want := filepath.Join("/tmp/probe-run", "gapi", controlAddrBase); paths[0] != want {
		t.Errorf("highest tier = %q, want %q", paths[0], want)
	}
	if want := filepath.Join("/run", "gapi", controlAddrBase); paths[len(paths)-1] != want {
		t.Errorf("lowest tier = %q, want %q", paths[len(paths)-1], want)
	}
}

// Under systemd there is no XDG_RUNTIME_DIR, and /run is then the only
// tier. A missing variable is not an error: it is the normal shape of a
// system daemon.
func TestControlAddrFiles_SystemDaemonHasRunOnly(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	mustUnset(t, "XDG_RUNTIME_DIR")

	paths := ControlAddrFiles()
	want := filepath.Join("/run", "gapi", controlAddrBase)
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("system daemon tiers = %v, want exactly [%s]", paths, want)
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
	if want := filepath.Join(dir, "gapi", controlAddrBase); written != want {
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

// No file is the ordinary case - no daemon has run - and must be
// reported as "nothing to say" rather than as a failure, or every
// control invocation on a clean host starts with an error.
func TestReadControlAddr_AbsentIsNotAnError(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	addr, from, err := ReadControlAddr()
	if err != nil {
		t.Fatalf("a missing address file is not an error, got %v", err)
	}
	if addr != "" || from != "" {
		t.Errorf("want no address and no source, got %q from %q", addr, from)
	}
}

// A daemon that died without cleaning up leaves a file naming a port
// nothing is listening on. The reader cannot detect that - only a dial
// can - so it must at least report WHERE the address came from, which is
// what lets the caller say "the file says X and nothing is there"
// instead of emitting a bare timeout (the entry's residual).
func TestReadControlAddr_CreditsItsSource(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	stale := filepath.Join(dir, "gapi")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stale, controlAddrBase), []byte("127.0.0.1:9\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	addr, from, err := ReadControlAddr()
	if err != nil {
		t.Fatalf("ReadControlAddr: %v", err)
	}
	if addr != "127.0.0.1:9" {
		t.Errorf("trailing newline not trimmed: %q", addr)
	}
	if from == "" {
		t.Error("reader must name the file it answered from, so a failure can name it too")
	}
}

// RemoveControlAddr is the shutdown half. It must not fail when there is
// nothing to remove, because shutdown runs on paths where the write
// never happened.
func TestRemoveControlAddr_IdempotentWhenAbsent(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if err := RemoveControlAddr(); err != nil {
		t.Fatalf("removing an absent address file must succeed, got %v", err)
	}
}
