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
	"crypto/tls"
	"errors"
	"testing"
	"time"

	quic "github.com/quic-go/quic-go"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/eventbus"
)

// A nil RETURN MUST MEAN THE BYTES WENT OUT.
//
// This is the falsifier for the whole change, and it is deterministic
// because the peer is ALREADY CLOSED: OpenStreamSync cannot succeed, so
// the send cannot succeed, so a nil return can only mean the caller
// never waited to find out.
//
// Before the fan-out was joined this returned nil, and the test that
// asserted so (TestPublishLogsSendFailure) had that assertion written
// into it as a documented property. It was the defect: measured on gapi
// #136, ten failing gapictl clients logged `event=dial` and not one
// reached the send goroutine's first line, so nil was returned for a
// send that never STARTED - and every downstream timeout was attributed
// to the wrong component for five occurrences across three pull
// requests.
//
// THE PEER SET IS BUILT DIRECTLY rather than through NewQUICClient,
// because handleConn removes a dead connection when AcceptStream errors.
// Dialling and then closing would race the eviction: sometimes the
// publish finds an empty set and returns ErrNoPeer, which is a DIFFERENT
// path and would make this test pass for the wrong reason roughly half
// the time.
func TestPublishRemoteReportsDeadPeer(t *testing.T) {
	conn := dialThenClose(t)
	q := &QUIC{peers: map[*quic.Conn]struct{}{conn: {}}}

	perr := q.PublishRemote(context.Background(), testPublishEvent(t, "dead-peer"))
	if perr == nil {
		t.Fatal("PublishRemote returned nil for an already-closed peer: nil must mean the bytes went out")
	}

	var incomplete *PublishIncomplete
	if !errors.As(perr, &incomplete) {
		t.Fatalf("PublishRemote returned %T (%v); want *PublishIncomplete so a caller can count peers", perr, perr)
	}
	if incomplete.Peers != 1 || incomplete.Failed != 1 || incomplete.Unconfirmed != 0 {
		t.Errorf("peers=%d failed=%d unconfirmed=%d; a closed peer FAILS, it does not go unconfirmed",
			incomplete.Peers, incomplete.Failed, incomplete.Unconfirmed)
	}

	// The STAGE is the reason to have a typed error at all: "the peer is
	// gone" and "the peer died mid-frame" are different operational
	// facts and a formatted string makes a caller parse for them.
	var send *PeerSendError
	if !errors.As(perr, &send) {
		t.Fatalf("no *PeerSendError inside %v; the per-peer cause must survive the join", perr)
	}
	if send.Stage != stageOpen {
		t.Errorf("stage %q; a closed connection fails at open", send.Stage)
	}
}

// ONE DEAD PEER MUST NOT COST A LIVE ONE ITS EVENT.
//
// The property the old asynchronous design existed to protect is that no
// peer's send is sequenced behind another's failure. Joining the fan-out
// could easily have destroyed it - a sequential loop that waits per peer
// would pass the test above and silently reintroduce head-of-line
// blocking - so it is asserted directly: the dead peer is FIRST in the
// map iteration as often as not, and the live peer receives regardless.
func TestPublishRemoteReachesLivePeerDespiteDeadOne(t *testing.T) {
	dead := dialThenClose(t)
	server, live := livePeerConn(t)

	received := make(chan eventbus.Event[*anypb.Any], 1)
	server.OnRemoteEvent(func(e eventbus.Event[*anypb.Any]) { received <- e })

	q := &QUIC{peers: map[*quic.Conn]struct{}{dead: {}, live: {}}}

	ev := testPublishEvent(t, "mixed-peers")
	perr := q.PublishRemote(context.Background(), ev)

	var incomplete *PublishIncomplete
	if !errors.As(perr, &incomplete) {
		t.Fatalf("PublishRemote = %v; one dead peer of two must report incomplete", perr)
	}
	if incomplete.Peers != 2 || incomplete.Failed != 1 {
		t.Errorf("peers=%d failed=%d; want 2 addressed and exactly 1 failed", incomplete.Peers, incomplete.Failed)
	}

	select {
	case got := <-received:
		if got.ID != ev.ID {
			t.Errorf("live peer received event %s; want %s", got.ID, ev.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("live peer received nothing within 5s: a dead peer blocked a healthy send")
	}
}

// THE WAIT IS BOUNDED, AND BY THE CONFIRMATION WINDOW RATHER THAN BY THE
// STREAM TIMEOUT.
//
// This is the cost of joining, and it is the reason the old code refused
// to wait: one unresponsive peer must not hold every publisher. The peer
// here grants NO stream credit (MaxIncomingStreams: 0), so OpenStreamSync
// blocks - deterministically, with no sleep and no load dependence -
// until its own QUICStreamTimeout of 10s.
//
// The assertion has two halves and both matter. The publish must return
// EARLY, well inside that 10s, or a stalled subscriber stalls the daemon
// for ten seconds a beat. And it must return UNCONFIRMED rather than
// FAILED, because the send is still running and may yet complete: the
// honest claim is "this is not known to have arrived", not "this did not
// arrive".
func TestPublishRemoteBoundsTheWaitOnAStalledPeer(t *testing.T) {
	conn := dialNoStreamCredit(t)
	q := &QUIC{peers: map[*quic.Conn]struct{}{conn: {}}}

	start := time.Now()
	perr := q.PublishRemote(context.Background(), testPublishEvent(t, "stalled-peer"))
	elapsed := time.Since(start)

	if elapsed >= config.QUICStreamTimeout {
		t.Fatalf("PublishRemote blocked %v; the confirmation window must bound the wait well below QUICStreamTimeout (%v)",
			elapsed, config.QUICStreamTimeout)
	}
	if elapsed < config.PublishConfirmTimeout {
		t.Errorf("PublishRemote returned after %v, inside the %v window; a stalled peer must be waited for, not skipped",
			elapsed, config.PublishConfirmTimeout)
	}

	var incomplete *PublishIncomplete
	if !errors.As(perr, &incomplete) {
		t.Fatalf("PublishRemote = %v; a stalled peer must report incomplete", perr)
	}
	if incomplete.Unconfirmed != 1 || incomplete.Failed != 0 {
		t.Errorf("failed=%d unconfirmed=%d; a send still in flight is unconfirmed, not failed",
			incomplete.Failed, incomplete.Unconfirmed)
	}
}

// EVERY PEER CONFIRMING IS THE ONLY WAY TO nil, and it has to be
// asserted or the change could be satisfied by never returning nil at
// all.
func TestPublishRemoteConfirmsALivePeer(t *testing.T) {
	_, live := livePeerConn(t)
	q := &QUIC{peers: map[*quic.Conn]struct{}{live: {}}}

	if perr := q.PublishRemote(context.Background(), testPublishEvent(t, "live-peer")); perr != nil {
		t.Fatalf("PublishRemote to one live peer = %v; want nil", perr)
	}
}

func testPublishEvent(t *testing.T, topic string) eventbus.Event[*anypb.Any] {
	t.Helper()

	payload, err := anypb.New(&anypb.Any{})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return eventbus.NewEvent("system", "", topic, "test", payload)
}

// livePeerConn returns a server transport and a raw connection to it,
// owned by the caller's peer set. The server is a real QUIC transport so
// that arrival can be observed through OnRemoteEvent rather than
// inferred.
func livePeerConn(t *testing.T) (*QUIC, *quic.Conn) {
	t.Helper()

	cert, err := GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	server, err := NewQUICServer("127.0.0.1:0", cert)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		if cerr := server.Close(); cerr != nil {
			t.Errorf("close server: %v", cerr)
		}
	})

	tlsConf, err := CreateClientTLSConfig(TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("client tls config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, server.Addr(), tlsConf, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseWithError(0, "closed by test") })
	return server, conn
}

// dialNoStreamCredit returns a connection whose peer will never permit a
// stream to be opened, so OpenStreamSync blocks rather than failing.
//
// A NEGATIVE MaxIncomingStreams is the whole mechanism, and it is why
// this test needs its own listener instead of NewQUICServer. A peer that
// refuses stream credit reproduces "the connection is up and the stream
// never happens" WITHOUT a sleep, a throttle or a dependence on machine
// load - which is the shape of the CI failure this change answers, where
// every orphaned port carried accept_conn and zero accept_stream.
//
// IT MUST BE NEGATIVE, NOT ZERO. quic-go reads 0 as "unset, use the
// default of 100", so a listener asking for zero streams grants a
// hundred. Written as 0 first, this test reported the publish returning
// in 143us with no error - which looks like the join failing and was the
// stall never happening.
//
// The Config value is deliberately -1 rather than
// MaxIncomingStreams: 0 with a comment; the quic-go contract is that
// negative denies, and encoding it as a literal keeps the intent in the
// code rather than in the prose beside it.
func dialNoStreamCredit(t *testing.T) *quic.Conn {
	t.Helper()

	cert, err := GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}
	ln, err := quic.ListenAddr("127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{ALPNGapiQUIC},
	}, &quic.Config{MaxIncomingStreams: -1})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		if cerr := ln.Close(); cerr != nil {
			t.Errorf("close listener: %v", cerr)
		}
	})

	// The listener must still ACCEPT, or the handshake never completes
	// and the dial below fails for the wrong reason. Accepting and then
	// doing nothing is exactly the daemon state being reproduced.
	accepted := make(chan struct{})
	go func() {
		defer close(accepted)
		if _, aerr := ln.Accept(context.Background()); aerr != nil {
			return
		}
	}()

	tlsConf, err := CreateClientTLSConfig(TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("client tls config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, ln.Addr().String(), tlsConf, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseWithError(0, "closed by test") })
	<-accepted
	return conn
}
