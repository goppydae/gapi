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

	"github.com/goppydae/gapi/core/eventbus"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestQUICHandshake(t *testing.T) {
	addr := "127.0.0.1:40001"

	cert, err := GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}

	server, err := NewQUICServer(addr, cert)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		if err := server.Close(); err != nil {
			t.Errorf("close server: %v", err)
		}
	}()

	received := make(chan eventbus.Event[*anypb.Any], 1)
	server.OnRemoteEvent(func(e eventbus.Event[*anypb.Any]) {
		received <- e
	})

	client, err := NewQUICClient(addr, nil, TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	}()

	// Wait for connection to stabilize
	time.Sleep(100 * time.Millisecond)

	// SCOPED, and every routing field is asserted (GAPI-DIV-100).
	//
	// This test used to publish a scopeless event and assert the ID
	// alone. That is a handshake assertion, and the name of the test is
	// honest about it - but it was also the ONLY test of this path, so
	// the fields that decide whether an event can be delivered went
	// unchecked. A scopeless publish round-trips through here happily
	// while being undeliverable at the far end, which is exactly the
	// defect that survived: bytes moved, so the test was green.
	testEvent := eventbus.Event[*anypb.Any]{
		ID:    "test-123",
		Scope: "system",
		Topic: "test/topic",
	}

	if err := client.PublishRemote(context.Background(), testEvent); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	select {
	case ev := <-received:
		if ev.ID != testEvent.ID {
			t.Errorf("Expected event ID %s, got %s", testEvent.ID, ev.ID)
		}
		if ev.Scope != testEvent.Scope {
			t.Errorf("Expected scope %q, got %q", testEvent.Scope, ev.Scope)
		}
		if ev.Topic != testEvent.Topic {
			t.Errorf("Expected topic %q, got %q", testEvent.Topic, ev.Topic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}
