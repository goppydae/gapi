// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agent

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestListener_TypedErrorWhenNotSocketActivated(t *testing.T) {
	t.Setenv("LISTEN_FDS", "")
	mustUnset(t, "LISTEN_FDS")

	_, err := Listener()
	if !errors.Is(err, ErrNoInheritedListener) {
		t.Fatalf("error %v, want ErrNoInheritedListener", err)
	}
}

func TestListener_RejectsMalformedActivationEnvironment(t *testing.T) {
	cases := []struct {
		name          string
		fds, pid      string
		wantTypedKind bool
	}{
		{name: "zero descriptors", fds: "0", wantTypedKind: true},
		{name: "not a number", fds: "many", wantTypedKind: false},
		{name: "another process", fds: "1", pid: "1", wantTypedKind: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LISTEN_FDS", tc.fds)
			if tc.pid != "" {
				t.Setenv("LISTEN_PID", tc.pid)
			} else {
				mustUnset(t, "LISTEN_PID")
			}

			_, err := Listener()
			if err == nil {
				t.Fatal("no error")
			}
			if got := errors.Is(err, ErrNoInheritedListener); got != tc.wantTypedKind {
				t.Errorf("errors.Is(err, ErrNoInheritedListener) = %v, want %v (err: %v)",
					got, tc.wantTypedKind, err)
			}
		})
	}
}

// TestListener_ServesAConnectionQueuedBeforeTheAgentStarted is the
// decisive test for GAPI-DIV-055, and the ordering is the entire point.
//
// The connection is made BEFORE the agent process exists, so it sits in
// the kernel's accept queue on the listener THIS process bound. An agent
// that binds its own socket - which is what the scaffolded Go socket
// agent did - can never see that connection, whether its bind fails with
// EADDRINUSE or races and wins. A test that connects after the agent is
// up cannot tell the two apart, which is why the defect survived.
func TestListener_ServesAConnectionQueuedBeforeTheAgentStarted(t *testing.T) {
	if os.Getenv("GAPI_ADK_LISTENER_CHILD") == "1" {
		listenerChild()
		return
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	defer func() { _ = ln.Close() }()

	tcp, ok := ln.(*net.TCPListener)
	if !ok {
		t.Fatalf("listener is %T, want *net.TCPListener", ln)
	}
	lf, err := tcp.File()
	if err != nil {
		t.Fatalf("listener fd: %v", err)
	}
	defer func() { _ = lf.Close() }()

	// Queue the connection FIRST. Nothing is accepting yet.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Now start the agent, handing it the listener as fd 3.
	cmd := exec.Command(os.Args[0], "-test.run=TestListener_ServesAConnectionQueuedBeforeTheAgentStarted")
	cmd.Env = append(os.Environ(),
		"GAPI_ADK_LISTENER_CHILD=1",
		"LISTEN_FDS=1",
		"LISTEN_PID=self",
	)
	cmd.ExtraFiles = []*os.File{lf} // ExtraFiles[0] is fd 3 in the child
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	if err := conn.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		t.Fatalf("deadline: %v", err)
	}
	if _, err := io.WriteString(conn, "ping\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, len("pong\n"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("the queued connection was never served: %v", err)
	}
	if string(buf) != "pong\n" {
		t.Errorf("read %q, want %q", buf, "pong\n")
	}
}

// listenerChild is the agent half of the test above: it takes the
// inherited listener through the ADK and serves exactly one connection.
// It NEVER binds an address - that is the property under test.
func listenerChild() {
	ln, err := Listener()
	if err != nil {
		fmt.Fprintf(os.Stderr, "child: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ln.Close() }()

	c, err := ln.Accept()
	if err != nil {
		fmt.Fprintf(os.Stderr, "child accept: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = c.Close() }()

	buf := make([]byte, len("ping\n"))
	if _, err := io.ReadFull(c, buf); err != nil {
		fmt.Fprintf(os.Stderr, "child read: %v\n", err)
		os.Exit(1)
	}
	if _, err := io.WriteString(c, "pong\n"); err != nil {
		fmt.Fprintf(os.Stderr, "child write: %v\n", err)
		os.Exit(1)
	}
}

// TestListenerAt_BoundsTheIndex covers the count-versus-descriptor
// confusion the ecosystem documentation warns about: LISTEN_FDS is how
// MANY were passed, so index 1 with a count of 1 is out of range even
// though fd 4 might exist for unrelated reasons.
func TestListenerAt_BoundsTheIndex(t *testing.T) {
	t.Setenv("LISTEN_FDS", "1")
	t.Setenv("LISTEN_PID", strconv.Itoa(os.Getpid()))

	if _, err := ListenerAt(1); err == nil {
		t.Error("ListenerAt(1) with LISTEN_FDS=1 returned no error")
	}
}

// mustUnset removes an environment variable, failing the test if it
// cannot - an ignored error would leave the activation environment set
// and make the negative assertions above meaningless.
func mustUnset(t *testing.T, key string) {
	t.Helper()
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}
