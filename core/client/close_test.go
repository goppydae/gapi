// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/transport"
)

// THE ASSERTION IS DAEMON-SIDE, AND GAPI-DIV-124's EXIT REQUIRES THAT
// RATHER THAN A CLIENT-SIDE ONE.
//
// "Close returned nil" says nothing about the defect. The defect is that
// the DAEMON kept the peer: a gapictl process exited without a graceful
// QUIC close, so the daemon learned of it only when MaxIdleTimeout
// expired - config.QUICIdleTimeout, 60 seconds - and until then fanned
// every reply out to every dead control invocation of the last minute.
// So the thing to observe is the server's peer set returning to its
// prior size.
//
// IT FAILS TODAY BY TAKING SIXTY SECONDS, which is what makes it a gate
// rather than a restatement. Without Close the peer survives until the
// idle timeout, so the bounded wait below expires and the test reports
// the count it actually saw.
func TestClientCloseReleasesTheDaemonsPeer(t *testing.T) {
	cert, err := transport.GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	server, err := transport.NewQUICServer("127.0.0.1:0", cert)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() {
		if cerr := server.Close(); cerr != nil {
			t.Errorf("close server: %v", cerr)
		}
	}()

	before := server.PeerCount()

	cfg := &config.Config{Transport: config.TransportConfig{
		Type:               "quic",
		Address:            server.Addr(),
		InsecureSkipVerify: true,
	}}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	// The daemon adds the peer from its accept loop, so attachment is
	// awaited rather than assumed - the same reason awaitPeer exists in
	// the transport tests.
	awaitPeerCount(t, server, before+1, 5*time.Second, "client attached")

	if cerr := c.Close(); cerr != nil {
		t.Fatalf("close client: %v", cerr)
	}

	// BOUNDED WELL UNDER QUICIdleTimeout, deliberately. A wait as long as
	// the idle timeout would pass whether or not Close does anything,
	// which is the shape of gate this repository keeps having to remove.
	deadline := 5 * time.Second
	if deadline >= config.QUICIdleTimeout {
		t.Fatalf("test deadline %v is not below QUICIdleTimeout %v; it would pass without the fix",
			deadline, config.QUICIdleTimeout)
	}
	awaitPeerCount(t, server, before, deadline, "client closed")
}

// A CLIENT BUILT FROM SOMEBODY ELSE'S BUS MUST NOT CLOSE IT. NewFromBus
// is exported for in-process use, where the caller owns the bus and may
// still be using it; a Close that shut it down would destroy a daemon's
// own event bus from a helper that merely borrowed it.
func TestCloseDoesNotShutABorrowedBus(t *testing.T) {
	bus := newTestDaemon(t)
	c := NewFromBus(bus)

	if err := c.Close(); err != nil {
		t.Fatalf("close on a borrowed bus: %v", err)
	}

	// Still usable: a ping through the same bus must still be answered,
	// which it cannot be if the client shut the bus down.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := NewFromBus(bus).Ping(ctx); err != nil {
		t.Fatalf("borrowed bus is unusable after the client closed it: %v", err)
	}
}

// Close is deferred by every caller and some paths close explicitly, so
// it has to tolerate being called twice.
func TestCloseIsIdempotent(t *testing.T) {
	cert, err := transport.GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	server, err := transport.NewQUICServer("127.0.0.1:0", cert)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = server.Close() }()

	cfg := &config.Config{Transport: config.TransportConfig{
		Type:               "quic",
		Address:            server.Addr(),
		InsecureSkipVerify: true,
	}}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func awaitPeerCount(t *testing.T, q *transport.QUIC, want int, within time.Duration, what string) {
	t.Helper()

	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if q.PeerCount() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s: daemon PeerCount is %d after %v, want %d", what, q.PeerCount(), within, want)
}
