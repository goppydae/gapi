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
	"errors"
	"testing"

	"github.com/goppydae/gapi/core/eventbus"
)

// A CLIENT HAS ITS PEER THE INSTANT ITS CONSTRUCTOR RETURNS.
//
// NewQUICClient dials synchronously and then hands the connection to
// handleConn in a GOROUTINE, and handleConn is what used to add the peer
// to the set. So there was a window between the constructor returning and
// that goroutine being scheduled in which the transport had a live
// connection and an EMPTY peer set - and a caller's first act is usually
// a request.
//
// A publish in that window returns ErrNoPeer, which eventbus demotes to a
// debug line (GAPI-DIV-095, correct for an announcement), so the request
// vanished with no error anywhere and the caller waited out its whole
// deadline. Measured on gapi #136 job 93050924850: `gapictl status`
// published one event on topic agents/ whose id appears exactly once in
// the run, then failed 30 seconds later.
//
// THE ASSERTION IS ON THE CONSTRUCTOR'S POSTCONDITION, deliberately,
// rather than on a publish succeeding. "The publish worked" can be true
// by luck when the goroutine happens to win; PeerCount() == 1 with no
// waiting in between is the property that makes it true by construction.
// awaitPeer exists elsewhere in these tests for the cases that legitimately
// need to wait; needing it HERE was the defect.
func TestNewQUICClientHasItsPeerOnReturn(t *testing.T) {
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
	defer func() {
		if cerr := client.Close(); cerr != nil {
			t.Errorf("close client: %v", cerr)
		}
	}()

	// NOTHING BETWEEN New AND THIS LINE. No sleep, no poll, no channel -
	// any of those would hide the race this asserts against.
	if n := client.PeerCount(); n != 1 {
		t.Fatalf("PeerCount() = %d immediately after NewQUICClient; want 1, or a caller's first publish races the peer set", n)
	}
}

// AND THE CONSEQUENCE THAT WAS ACTUALLY OBSERVED: the first publish must
// not report ErrNoPeer.
//
// Stated separately from the postcondition above because it is the
// symptom rather than the mechanism, and because it is the one a future
// reader will recognise from the failure text.
func TestClientPublishesImmediatelyWithoutErrNoPeer(t *testing.T) {
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
	defer func() {
		if cerr := client.Close(); cerr != nil {
			t.Errorf("close client: %v", cerr)
		}
	}()

	if perr := client.PublishRemote(context.Background(), testPublishEvent(t, "first-publish")); errors.Is(perr, eventbus.ErrNoPeer) {
		t.Fatalf("first publish after NewQUICClient = %v; a live connection is not an absent peer", perr)
	}
}
