// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package safeio

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_EmptyRejected(t *testing.T) {
	if _, err := Resolve(""); err == nil {
		t.Fatal("Resolve(\"\") should fail")
	}
}

func TestReadFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("payload"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("ReadFile = %q, want %q", got, "payload")
	}
}

func TestCreate_ThenOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	f, err := Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.WriteString("x"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	g, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestReplaceOwnerOnly_CreatesOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")

	if err := ReplaceOwnerOnly(path, []byte("k")); err != nil {
		t.Fatalf("ReplaceOwnerOnly: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %#o, want %#o", perm, 0o600)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "k" {
		t.Errorf("content = %q, want %q", got, "k")
	}
	assertOnlyFile(t, dir, path)
}

// TestReplaceOwnerOnly_ReplacesExistingLooseFile holds a descriptor on a
// 0644 file and requires it to still read the old bytes afterwards. That
// is only true if the destination was replaced; writing through it would
// expose the new content at the old mode for the length of the write.
func TestReplaceOwnerOnly_ReplacesExistingLooseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	const stale = "stale"
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	held, err := os.Open(path)
	if err != nil {
		t.Fatalf("hold open: %v", err)
	}
	defer func() {
		if err := held.Close(); err != nil {
			t.Errorf("close held descriptor: %v", err)
		}
	}()

	if err := ReplaceOwnerOnly(path, []byte("fresh")); err != nil {
		t.Fatalf("ReplaceOwnerOnly: %v", err)
	}

	got, err := io.ReadAll(held)
	if err != nil {
		t.Fatalf("read held descriptor: %v", err)
	}
	if string(got) != stale {
		t.Fatalf("held descriptor reads %q, want %q: the file was written through, not replaced", got, stale)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %#o after replacing a 0644 file, want %#o", perm, 0o600)
	}
	assertOnlyFile(t, dir, path)
}

// TestReplaceOwnerOnly_NoTempLeftOnFailure uses a destination that is a
// directory, so the rename fails after the temp file already holds the
// data. A failed write of key material must not leave that data behind.
func TestReplaceOwnerOnly_NoTempLeftOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "occupied")
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := ReplaceOwnerOnly(path, []byte("k")); err == nil {
		t.Fatal("ReplaceOwnerOnly over a directory should fail")
	}
	assertOnlyFile(t, dir, path)
}

func TestReplaceOwnerOnly_EmptyPathRejected(t *testing.T) {
	if err := ReplaceOwnerOnly("", []byte("k")); err == nil {
		t.Fatal("ReplaceOwnerOnly(\"\") should fail")
	}
}

// assertOnlyFile fails unless dir contains exactly the named entries,
// which is how a leftover temp file shows up.
func assertOnlyFile(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, filepath.Join(dir, e.Name()))
	}
	if len(got) != len(want) {
		t.Fatalf("directory holds %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("directory holds %v, want %v", got, want)
		}
	}
}

func TestReadFileUnder_ConfinedOK(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b.txt")
	if err := os.MkdirAll(filepath.Dir(sub), 0750); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(sub, []byte("ok"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := ReadFileUnder(root, sub)
	if err != nil {
		t.Fatalf("ReadFileUnder: %v", err)
	}
	if string(got) != "ok" {
		t.Errorf("ReadFileUnder = %q, want %q", got, "ok")
	}
}

func TestResolveUnder_EscapeRejected(t *testing.T) {
	root := t.TempDir()

	cases := []string{
		filepath.Join(root, "..", "escape.txt"),
		filepath.Join(root, "a", "..", "..", "escape.txt"),
		"/etc/passwd",
	}
	for _, path := range cases {
		if _, err := ResolveUnder(root, path); err == nil {
			t.Errorf("ResolveUnder(%q, %q) should fail", root, path)
		} else if !strings.Contains(err.Error(), "escapes root") {
			t.Errorf("ResolveUnder(%q, %q) error = %v, want escape error", root, path, err)
		}
	}
}

func TestResolveUnder_SiblingPrefixRejected(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "a")
	sibling := filepath.Join(base, "ab", "f.txt")
	if err := os.MkdirAll(root, 0750); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// "ab" shares the string prefix "a" but is not under root "a".
	if _, err := ResolveUnder(root, sibling); err == nil {
		t.Errorf("ResolveUnder(%q, %q) should fail", root, sibling)
	}
}

func TestResolveUnder_RootItselfAllowed(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveUnder(root, root); err != nil {
		t.Errorf("ResolveUnder(root, root) = %v, want nil", err)
	}
}
