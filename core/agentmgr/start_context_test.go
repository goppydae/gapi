// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
)

// heartbeatScript writes a line to beatPath roughly every 100ms until it
// is killed. Liveness is then observable from outside the process - no
// signals, no reaching into GoAgent's unexported state - which is what
// lets this test assert "still running" at the public boundary.
func heartbeatScript(t *testing.T, dir, beatPath string) string {
	t.Helper()
	path := filepath.Join(dir, "heartbeat.sh")
	script := "#!/bin/sh\n" +
		"while true; do\n" +
		"  echo beat >> " + beatPath + "\n" +
		"  sleep 0.1\n" +
		"done\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write heartbeat script: %v", err)
	}
	return path
}

func beatCount(t *testing.T, beatPath string) int {
	t.Helper()
	b, err := os.ReadFile(beatPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read heartbeat file: %v", err)
	}
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

// TestGoAgent_ProcessOutlivesStartContext pins GAPI-DIV-028.
//
// The context handed to Start bounds the start operation, not the agent.
// When the agent was spawned with exec.CommandContext, os/exec installed a
// watchdog that SIGKILLed the child the instant that context was done -
// which is immediately after the start call returns. Every agent gapid
// launched died about a millisecond after being declared started, and the
// only downstream evidence was a later verb failing with "agent not
// running".
//
// Cancelling the start context is therefore the whole test: the agent must
// still be running afterwards.
func TestGoAgent_ProcessOutlivesStartContext(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	dir := t.TempDir()
	beatPath := filepath.Join(dir, "beats")
	script := heartbeatScript(t, dir, beatPath)

	agent := NewGoAgent(
		"test_outlives_ctx",
		"service",
		script,
		nil, nil, nil, nil,
		"",
		"", "",
		nil,
		eventbus.NewInprocBus[*anypb.Any](),
		NewMockDependencyResolver(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = agent.Stop(stopCtx)
	})

	// Let it establish a heartbeat, so a later stall is unambiguous.
	time.Sleep(400 * time.Millisecond)
	before := beatCount(t, beatPath)
	if before == 0 {
		t.Fatal("agent never wrote a heartbeat; the fixture did not start")
	}

	// The exact moment that used to be fatal.
	cancel()

	time.Sleep(600 * time.Millisecond)
	after := beatCount(t, beatPath)
	if after <= before {
		t.Errorf("heartbeat stopped after the start context was cancelled: %d beats before, %d after - the agent was killed with its start context (GAPI-DIV-028)", before, after)
	}
}
