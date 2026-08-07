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

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
)

// A QUIC server with nothing dialled into it is the state a single-node
// daemon spends its whole life in. Publishing there must report "no peer",
// not io.ErrUnexpectedEOF (GAPI-DIV-095).
func TestPublishRemoteWithoutPeerReportsNoPeer(t *testing.T) {
	cert, err := GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}

	q, err := NewQUICServer("127.0.0.1:0", cert)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() {
		if cerr := q.Close(); cerr != nil {
			t.Errorf("close transport: %v", cerr)
		}
	}()

	ev := eventbus.NewEvent[*anypb.Any]("system", "", "test.msg", "test", nil)

	if perr := q.PublishRemote(context.Background(), ev); !errors.Is(perr, eventbus.ErrNoPeer) {
		t.Fatalf("PublishRemote with no peer = %v, want ErrNoPeer", perr)
	}

	// The Broadcast half of this test is gone with the method
	// (GAPI-DIV-106). It asserted that Broadcast reported the same thing
	// as PublishRemote, which it necessarily did - it called it. There is
	// one operation now, and TestPublishAfterEveryPeerLeavesReportsNoPeer
	// covers the case this one cannot: an empty set that once had a peer,
	// rather than one that never did.
}
