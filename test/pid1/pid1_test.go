// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

//go:build pid1

// The PID-1 e2e (GAPI-DIV-027): gapid runs as PID 1 of a rootless
// podman container - real kernel semantics for the init obligations.
// Asserted: an orphaned grandchild is reaped with its true status, and
// SIGTERM to init yields the graceful teardown order and a clean exit.
// reboot(2) is not permitted in a rootless container, so the executor
// falls through to exit - which IS container-init poweroff.
package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var built struct {
	rootfs string // contains /gapid and /agents/orphanmaker
}

func TestMain(m *testing.M) {
	if _, err := exec.LookPath("podman"); err != nil {
		fmt.Println("SKIP: podman not in PATH; run mage testPid1 inside the dev shell")
		os.Exit(0)
	}

	rootfs, err := os.MkdirTemp("", "gapid-pid1-rootfs-")
	if err != nil {
		fmt.Println("mkdtemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(rootfs)
	for _, d := range []string{"agents", "tmp", "proc", "dev", "sys", "run", "etc"} {
		if err := os.MkdirAll(filepath.Join(rootfs, d), 0o755); err != nil {
			fmt.Println("mkdir:", err)
			os.Exit(1)
		}
	}

	// Binaries are built by the dev shell's toolchain and run against
	// the bind-mounted /nix/store, so no static-linking gamble.
	build := func(out, pkg string) bool {
		cmd := exec.Command("go", "build", "-o", out, pkg)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Println("build", pkg, ":", err)
			return false
		}
		return true
	}
	if !build(filepath.Join(rootfs, "gapid"), "../../cmd/gapid") {
		os.Exit(1)
	}
	if !build(filepath.Join(rootfs, "agents", "orphanmaker"), "./fixtures/orphanmaker") {
		os.Exit(1)
	}
	built.rootfs = rootfs

	// Probe that rootless podman can actually create containers here;
	// sandboxes without user namespaces cannot, and the operator host
	// is where this suite then runs (LOUD skip, not silence).
	probe := exec.Command("podman", "run", "--rm",
		"-v", "/nix/store:/nix/store:ro",
		"--rootfs", rootfs+":O", "/gapid", "version")
	if out, err := probe.CombinedOutput(); err != nil {
		fmt.Printf("SKIP: rootless podman cannot run containers in this environment: %v\n%s\n", err, out)
		fmt.Println("Run mage testPid1 on the operator host to execute the PID-1 proof.")
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestPid1EndToEnd(t *testing.T) {
	name := fmt.Sprintf("gapid-pid1-%d", os.Getpid())
	tmpVol := t.TempDir()

	logPath := filepath.Join(t.TempDir(), "container.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create log: %v", err)
	}

	run := exec.Command("podman", "run", "--rm",
		"--name", name,
		"-v", "/nix/store:/nix/store:ro",
		"-v", tmpVol+":/tmp",
		"--env", "GAPI_AGENT_PATH=/agents",
		// Explicit since GAPI-DIV-063 made AGENT_PATH additive: the
		// guest must see /agents and nothing else.
		"--env", "GAPI_AGENT_PATH_EXCLUSIVE=1",
		"--env", "GAPI_KMSG_PATH=/tmp/kmsg",
		"--rootfs", built.rootfs+":O",
		"/gapid", "--pid1", "--no-early-mounts", "--log-level", "debug",
	)
	run.Stdout, run.Stderr = logFile, logFile
	if err := run.Start(); err != nil {
		t.Fatalf("start container: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("podman", "rm", "-f", name).Run()
		_ = logFile.Close()
	})

	waitForLog := func(needle, what string, timeout time.Duration) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for {
			data, _ := os.ReadFile(logPath)
			if strings.Contains(string(data), needle) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: %q not seen within %s; log:\n%s", what, needle, timeout, tail(string(data), 4000))
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// --- Scenario 1: boot. Phase 0 completes - the kmsg narration
	// lands in the injected device.
	deadline := time.Now().Add(30 * time.Second)
	for {
		kmsg, _ := os.ReadFile(filepath.Join(tmpVol, "kmsg"))
		if strings.Contains(string(kmsg), "phase 0 complete") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("kmsg narration missing phase 0 complete: %q", kmsg)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// --- Scenario 2: orphan reaping - THE PID-1 obligation. Seeing the
	// grandchild's true status (exit 7 -> wait status 1792) in the reap
	// log proves the whole chain: discovery found the fixture, the
	// runtime spawned it, it double-forked, the orphan reparented to
	// gapid, and the subreaper loop collected it. (The fixture is not a
	// full ADK agent, so the lifecycle controller's running-state
	// handshake times out separately - by design out of scope here.)
	waitForLog(`"wait_status":1792`, "orphan reaped with true status", 60*time.Second)
	waitForLog("adopted orphan reaped", "orphan classified as adopted", 5*time.Second)

	// --- Scenario 3: SIGTERM to init. The explicit handler must catch
	// it (the kernel suppresses the default for pid 1), tear down
	// gracefully, and exit 0 - reboot(2) is refused rootless, and the
	// executor's exit IS container poweroff.
	if out, err := exec.Command("podman", "kill", "--signal", "SIGTERM", name).CombinedOutput(); err != nil {
		t.Fatalf("podman kill: %v: %s", err, out)
	}

	done := make(chan error, 1)
	go func() { done <- run.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			data, _ := os.ReadFile(logPath)
			t.Fatalf("container exited non-zero after SIGTERM: %v; log:\n%s", err, tail(string(data), 4000))
		}
	case <-time.After(30 * time.Second):
		t.Fatal("container did not exit within 30s of SIGTERM")
	}

	data, _ := os.ReadFile(logPath)
	logText := string(data)
	for _, needle := range []string{
		"pid1 shutdown requested",
		"exited cleanly",
		"reboot unavailable, exiting as container init",
	} {
		if !strings.Contains(logText, needle) {
			t.Fatalf("teardown narration missing %q; log:\n%s", needle, tail(logText, 4000))
		}
	}
	t.Logf("pid1 e2e: boot, orphan reaping (status 1792), and SIGTERM teardown all verified as container init")
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}
