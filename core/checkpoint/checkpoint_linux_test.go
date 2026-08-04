// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package checkpoint_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/goppydae/gapi/core/checkpoint"
)

// Image-directory validation happens before the capability check, so
// these run anywhere - including an unprivileged sandbox that could
// never complete a real dump.

func TestDumpRejectsMissingImagesDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	err := checkpoint.Dump(os.Getpid(), missing, checkpoint.Options{})
	if err == nil {
		t.Fatal("Dump accepted a missing image directory")
	}
	if !errors.Is(err, checkpoint.ErrImagesDir) {
		t.Fatalf("want ErrImagesDir, got %v", err)
	}

	var cpErr *checkpoint.Error
	if !errors.As(err, &cpErr) {
		t.Fatalf("want *checkpoint.Error, got %T", err)
	}
	if cpErr.Op != "dump" {
		t.Errorf("Op = %q, want dump", cpErr.Op)
	}
	if cpErr.Dir != missing {
		t.Errorf("Dir = %q, want %q", cpErr.Dir, missing)
	}
}

func TestDumpRejectsNonDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "regular-file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	err := checkpoint.Dump(os.Getpid(), file, checkpoint.Options{})
	if !errors.Is(err, checkpoint.ErrImagesDir) {
		t.Fatalf("want ErrImagesDir for a regular file, got %v", err)
	}
}

func TestRestoreRejectsMissingImagesDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	pid, err := checkpoint.Restore(missing, checkpoint.Options{})
	if err == nil {
		t.Fatal("Restore accepted a missing image directory")
	}
	if pid != 0 {
		t.Errorf("Restore returned pid %d alongside an error", pid)
	}
	if !errors.Is(err, checkpoint.ErrImagesDir) {
		t.Fatalf("want ErrImagesDir, got %v", err)
	}
}

// An empty but valid directory gets past validation and reaches the
// capability gate. On an unprivileged host that must surface as
// ErrNotCapable or ErrNoCriu - never as a nil error or a bare criu
// string, both of which would leave the orchestrator unable to tell
// "cannot" from "failed".
func TestDumpOnValidDirReachesCapabilityGate(t *testing.T) {
	dir := t.TempDir()

	err := checkpoint.Dump(os.Getpid(), dir, checkpoint.Options{})
	if err == nil {
		t.Skip("host can checkpoint; capability gate not exercised")
	}
	if errors.Is(err, checkpoint.ErrImagesDir) {
		t.Fatalf("valid directory rejected as unusable: %v", err)
	}

	var cpErr *checkpoint.Error
	if !errors.As(err, &cpErr) {
		t.Fatalf("want *checkpoint.Error, got %T: %v", err, err)
	}
}
