// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package transport

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
)

// TWO CONTROL CLIENTS ARE THE NORMAL CASE, NOT AN EXOTIC ONE, WHICH IS
// WHY THIS IS THE CLOSING TEST FOR GAPI-DIV-106.
//
// A single operator running the TUI already holds two concurrent
// connections: core/client.New dials ONCE for the long-lived status
// poller (core/tui/tui.go), and core/tui/actions.go opens a SECOND
// client per lifecycle action, subscribes, and waits for the matching
// event. So "refuse the second connection" was never an available exit -
// it would break the TUI - and addressing every peer is the only one.
//
// The defect this asserts against: handleConn assigned q.conn
// unconditionally, so the field was last-writer-wins with no set, no
// eviction and no log. The second client silently displaced the first,
// and only SERVER-INITIATED pushes use q.conn - which is why it went
// unnoticed. Nothing errored anywhere; the displaced peer simply stopped
// hearing.
func TestServerPublishReachesEveryPeer(t *testing.T) {
	cert, err := GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}

	server, err := NewQUICServer("127.0.0.1:0", cert)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() {
		if cerr := server.Close(); cerr != nil {
			t.Errorf("close server: %v", cerr)
		}
	}()

	addr := server.Addr()

	// Buffered, so a receive that DOES happen cannot be lost to timing
	// and read as the defect under test.
	gotA := make(chan eventbus.Event[*anypb.Any], 1)
	gotB := make(chan eventbus.Event[*anypb.Any], 1)

	clientA, err := NewQUICClient(addr, nil, TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("dial client A: %v", err)
	}
	defer func() {
		if cerr := clientA.Close(); cerr != nil {
			t.Errorf("close client A: %v", cerr)
		}
	}()
	clientA.OnRemoteEvent(func(e eventbus.Event[*anypb.Any]) { gotA <- e })

	clientB, err := NewQUICClient(addr, nil, TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("dial client B: %v", err)
	}
	defer func() {
		if cerr := clientB.Close(); cerr != nil {
			t.Errorf("close client B: %v", cerr)
		}
	}()
	clientB.OnRemoteEvent(func(e eventbus.Event[*anypb.Any]) { gotB <- e })

	// Both handshakes must be ACCEPTED SERVER-SIDE before the publish,
	// or a miss means "not connected yet" rather than "displaced" - two
	// different failures with one symptom.
	waitForPeers(t, server, 2)

	ev := eventbus.Event[*anypb.Any]{
		ID:     "peerset-1",
		Scope:  "system",
		Topic:  "test/peerset",
		Source: "gapid",
	}
	if perr := server.PublishRemote(context.Background(), ev); perr != nil {
		t.Fatalf("PublishRemote to two peers: %v", perr)
	}

	// Named per peer. "one of them got it" is the assertion that would
	// have passed against the defect.
	awaitEvent(t, "client A", gotA, ev.ID)
	awaitEvent(t, "client B", gotB, ev.ID)
}

// A publish with every peer gone still reports ErrNoPeer. The nil check
// became an emptiness check when q.conn became a set, and GAPI-DIV-095's
// reasoning is unchanged: nothing was read and nothing went wrong, there
// is simply nobody to send to.
func TestPublishAfterEveryPeerLeavesReportsNoPeer(t *testing.T) {
	cert, err := GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}

	server, err := NewQUICServer("127.0.0.1:0", cert)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() {
		if cerr := server.Close(); cerr != nil {
			t.Errorf("close server: %v", cerr)
		}
	}()

	client, err := NewQUICClient(server.Addr(), nil, TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	clientA := client
	clientA.OnRemoteEvent(func(eventbus.Event[*anypb.Any]) {})

	waitForPeers(t, server, 1)

	if cerr := client.Close(); cerr != nil {
		t.Fatalf("close client: %v", cerr)
	}

	// The peer is removed where the connection dies - handleConn's
	// AcceptStream loop - not on publish failure. Evicting on a failed
	// send would make a slow peer indistinguishable from a dead one.
	waitForPeers(t, server, 0)

	ev := eventbus.NewEvent[*anypb.Any]("system", "", "test.msg", "gapid", nil)
	if perr := server.PublishRemote(context.Background(), ev); perr == nil {
		t.Fatal("PublishRemote after the only peer left = nil, want ErrNoPeer")
	}
}

func waitForPeers(t *testing.T, q *QUIC, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := q.PeerCount(); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d peer(s); have %d", want, q.PeerCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func awaitEvent(t *testing.T, who string, ch <-chan eventbus.Event[*anypb.Any], wantID string) {
	t.Helper()
	select {
	case e := <-ch:
		if e.ID != wantID {
			t.Errorf("%s received event %q, want %q", who, e.ID, wantID)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("%s received nothing; a server publish must reach every connected peer", who)
	}
}
