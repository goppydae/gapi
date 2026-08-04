// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package shutdown_test

import (
	"testing"
	"time"

	"github.com/goppydae/gapi/core/mounts"
	"github.com/goppydae/gapi/core/shutdown"
)

type recorder struct {
	calls     []string
	rebootCmd int
	stopDelay time.Duration
}

func (r *recorder) StopAll() error {
	if r.stopDelay > 0 {
		time.Sleep(r.stopDelay)
	}
	r.calls = append(r.calls, "stopall")
	return nil
}
func (r *recorder) Sync() { r.calls = append(r.calls, "sync") }
func (r *recorder) Unmount(target string, flags int) error {
	r.calls = append(r.calls, "umount:"+target)
	return nil
}
func (r *recorder) Reboot(cmd int) error {
	r.calls = append(r.calls, "reboot")
	r.rebootCmd = cmd
	return nil
}

var testMounts = []mounts.MountSpec{
	{Source: "proc", Target: "/proc"},
	{Source: "tmpfs", Target: "/run"},
}

// The teardown order is the contract: agents stopped, pages synced,
// mounts detached in reverse order, then reboot(2) - nothing may run
// after sync that could dirty pages again.
func TestSystemShutdown_PhaseOrder(t *testing.T) {
	r := &recorder{}
	if err := shutdown.SystemShutdown(r, r, testMounts, shutdown.PowerOff, time.Second); err != nil {
		t.Fatalf("SystemShutdown: %v", err)
	}
	want := []string{"stopall", "sync", "umount:/run", "umount:/proc", "reboot"}
	if len(r.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", r.calls, want)
	}
	for i := range want {
		if r.calls[i] != want[i] {
			t.Fatalf("calls[%d] = %s, want %s (%v)", i, r.calls[i], want[i], r.calls)
		}
	}
}

// Each action maps to its reboot(2) command.
func TestSystemShutdown_ActionMapping(t *testing.T) {
	for _, tc := range []struct {
		action shutdown.Action
		want   int
	}{
		{shutdown.PowerOff, shutdown.CmdPowerOff},
		{shutdown.Reboot, shutdown.CmdRestart},
		{shutdown.Halt, shutdown.CmdHalt},
	} {
		r := &recorder{}
		if err := shutdown.SystemShutdown(r, r, nil, tc.action, time.Second); err != nil {
			t.Fatalf("SystemShutdown(%v): %v", tc.action, err)
		}
		if r.rebootCmd != tc.want {
			t.Fatalf("action %v rebooted with %#x, want %#x", tc.action, r.rebootCmd, tc.want)
		}
	}
}

// A hung agent must not block sync: StopAll is bounded by the grace
// period and shutdown proceeds without it.
func TestSystemShutdown_HungStopAllBounded(t *testing.T) {
	r := &recorder{stopDelay: 2 * time.Second}
	start := time.Now()
	if err := shutdown.SystemShutdown(r, r, nil, shutdown.Halt, 100*time.Millisecond); err != nil {
		t.Fatalf("SystemShutdown: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("shutdown blocked %v on a hung StopAll (grace 100ms)", elapsed)
	}
	// sync and reboot still ran.
	var sawSync, sawReboot bool
	for _, c := range r.calls {
		if c == "sync" {
			sawSync = true
		}
		if c == "reboot" {
			sawReboot = true
		}
	}
	if !sawSync || !sawReboot {
		t.Fatalf("shutdown did not complete past the hung StopAll: %v", r.calls)
	}
}
