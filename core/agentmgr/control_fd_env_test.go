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
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
)

// THE ACTIVATION ENVIRONMENT IS A CONTRACT WITH THE ADK, AND NOTHING
// ASSERTED IT.
//
// LISTEN_FDS is a COUNT OF LISTENERS. The control descriptor is not a
// listener, and counting it told an agent with no socket that it had one
// - which makes adk/go/agent's ErrNoInheritedListener unreachable and
// sends listenerAt at fd 3, the control channel itself. The suite was
// blind to it because nothing in-tree calls Listener().
//
// These tests read the CHILD'S ACTUAL ENVIRONMENT rather than the
// supervisor's intent: the defect was a count computed one line too late,
// which every test of the supervisor's own variables would have agreed
// with.

// envDumpAgent writes its environment to out and exits.
func envDumpAgent(t *testing.T, out string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "envdump.sh")
	body := "#!/bin/sh\nenv > " + out + "\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write agent script: %v", err)
	}
	return script
}

// childEnv starts a oneshot agent with the given listen spec and returns
// the environment its child actually received.
func childEnv(t *testing.T, listenSpec string) map[string]string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "env.txt")
	script := envDumpAgent(t, out)

	agent := NewGoAgent(
		"env-dump", "oneshot", script,
		nil, nil, nil, nil,
		listenSpec,
		"", "", nil,
		eventbus.NewInprocBus[*anypb.Any](),
		NewMockDependencyResolver(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read child env: %v", err)
	}
	env := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok {
			env[k] = v
		}
	}
	return env
}

// TestNoListenerMeansNoListenFDs is the regression.
//
// An agent with no socket must see NO LISTEN_FDS AT ALL. An absent
// variable is how the ADK knows there was no activation; LISTEN_FDS=0 is
// a different claim and LISTEN_FDS=1 is a false one.
func TestNoListenerMeansNoListenFDs(t *testing.T) {
	env := childEnv(t, "")

	if v, ok := env["LISTEN_FDS"]; ok {
		t.Fatalf("agent with no socket was told LISTEN_FDS=%q", v)
	}
	if v, ok := env["LISTEN_PID"]; ok {
		t.Fatalf("agent with no socket was told LISTEN_PID=%q", v)
	}
	// The control descriptor is still passed, and it is fd 3 because
	// nothing precedes it.
	if got := env["ADK_CONTROL_FD"]; got != "3" {
		t.Fatalf("ADK_CONTROL_FD = %q, want 3", got)
	}
}

// TestListenerCountExcludesTheControlDescriptor pins the count itself.
func TestListenerCountExcludesTheControlDescriptor(t *testing.T) {
	env := childEnv(t, "127.0.0.1:0")

	if got := env["LISTEN_FDS"]; got != "1" {
		t.Fatalf("LISTEN_FDS = %q, want 1 - the control descriptor is not a listener", got)
	}
	if got := env["LISTEN_PID"]; got != "self" {
		t.Fatalf("LISTEN_PID = %q, want self", got)
	}
	// One listener at fd 3 puts the control descriptor at fd 4.
	if got := env["ADK_CONTROL_FD"]; got != "4" {
		t.Fatalf("ADK_CONTROL_FD = %q, want 4", got)
	}
}
