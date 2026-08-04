// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package pid1_test

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/pid1"
)

func waitFlag(t *testing.T, name string, flag *atomic.Int32) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for flag.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%s handler not invoked within 3s", name)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Every PID-1 signal dispatches to its explicit handler - including
// SIGTERM, which the kernel suppresses for pid 1 unless a handler is
// installed (the silent-drop this package exists to prevent).
func TestInstall_DispatchesExplicitSemantics(t *testing.T) {
	var shutdown, reload, reap, debug, emergency atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := pid1.Install(ctx, pid1.Handlers{
		Shutdown:  func(os.Signal) { shutdown.Store(1) },
		Reload:    func() { reload.Store(1) },
		Reap:      func() { reap.Store(1) },
		Debug:     func() { debug.Store(1) },
		Emergency: func() { emergency.Store(1) },
	})
	defer stop()

	self := os.Getpid()
	for _, tc := range []struct {
		sig  syscall.Signal
		name string
		flag *atomic.Int32
	}{
		{syscall.SIGHUP, "reload", &reload},
		{syscall.SIGUSR1, "debug", &debug},
		{syscall.SIGUSR2, "emergency", &emergency},
		{syscall.SIGCHLD, "reap", &reap},
		{syscall.SIGTERM, "shutdown", &shutdown},
	} {
		if err := syscall.Kill(self, tc.sig); err != nil {
			t.Fatalf("kill self with %v: %v", tc.sig, err)
		}
		waitFlag(t, tc.name, tc.flag)
	}
}

// Handlers left nil are inert: an unwired signal is absorbed (never a
// process kill), because PID 1 has no implicit defaults to fall back
// on.
func TestInstall_NilHandlersAbsorb(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := pid1.Install(ctx, pid1.Handlers{})
	defer stop()

	// SIGTERM with no handler must not kill the test process.
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	// Reaching here is the assertion.
}
