// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package mounts_test

import (
	"errors"
	"testing"

	"github.com/goppydae/gapi/core/mounts"
)

type fakeMounter struct {
	mounted map[string]bool
	mkdirs  []string
	calls   []string
	failOn  string
}

func newFakeMounter(pre ...string) *fakeMounter {
	m := &fakeMounter{mounted: map[string]bool{}}
	for _, t := range pre {
		m.mounted[t] = true
	}
	return m
}

func (f *fakeMounter) Mount(source, target, fstype string, flags uintptr, data string) error {
	if target == f.failOn {
		return errors.New("mount refused")
	}
	f.calls = append(f.calls, target)
	f.mounted[target] = true
	return nil
}

func (f *fakeMounter) IsMounted(target string) (bool, error) { return f.mounted[target], nil }
func (f *fakeMounter) MkdirAll(path string) error {
	f.mkdirs = append(f.mkdirs, path)
	return nil
}

// The executor mounts the doc's table in declared order - ordering is
// load-bearing (cgroup2 assumes /sys exists, cgroups.Init assumes
// /sys/fs/cgroup is mounted).
func TestMountEarly_OrderedExecution(t *testing.T) {
	f := newFakeMounter()
	if err := mounts.MountEarly(f, mounts.EarlyMounts); err != nil {
		t.Fatalf("MountEarly: %v", err)
	}
	want := []string{"/dev", "/proc", "/sys", "/sys/fs/cgroup", "/run", "/tmp"}
	if len(f.calls) != len(want) {
		t.Fatalf("mounted %v, want %v", f.calls, want)
	}
	for i, target := range want {
		if f.calls[i] != target {
			t.Fatalf("mount order[%d] = %s, want %s (got %v)", i, f.calls[i], target, f.calls)
		}
	}
}

// Already-mounted targets are skipped, not remounted: container
// runtimes premount /proc and /sys, and PID-1 boot must be idempotent
// over them.
func TestMountEarly_IdempotentOverPremounted(t *testing.T) {
	f := newFakeMounter("/proc", "/sys")
	if err := mounts.MountEarly(f, mounts.EarlyMounts); err != nil {
		t.Fatalf("MountEarly: %v", err)
	}
	for _, target := range f.calls {
		if target == "/proc" || target == "/sys" {
			t.Fatalf("premounted %s was mounted again", target)
		}
	}
}

// A mount failure is loud and stops the sequence: continuing past a
// missing /proc corrupts everything after it.
func TestMountEarly_FailsClosed(t *testing.T) {
	f := newFakeMounter()
	f.failOn = "/proc"
	err := mounts.MountEarly(f, mounts.EarlyMounts)
	if err == nil {
		t.Fatal("mount failure was swallowed")
	}
	for _, target := range f.calls {
		if target == "/sys" || target == "/run" {
			t.Fatalf("sequence continued past the failure to %s", target)
		}
	}
}
