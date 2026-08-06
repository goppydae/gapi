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
	"slices"
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

// newQUICPair returns a client connected to a server on addr, and the
// channel the server's remote-event callback writes to. Both ends are
// closed by t.Cleanup.
func newQUICPair(t *testing.T, addr string) (*QUIC, <-chan eventbus.Event[*anypb.Any]) {
	t.Helper()

	cert, err := GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("generate cert: %v", err)
	}

	server, err := NewQUICServer(addr, cert)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("close server: %v", err)
		}
	})

	received := make(chan eventbus.Event[*anypb.Any], 1)
	server.OnRemoteEvent(func(e eventbus.Event[*anypb.Any]) {
		received <- e
	})

	client, err := NewQUICClient(addr, nil, TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("start client: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})

	// Wait for connection to stabilize
	time.Sleep(100 * time.Millisecond)

	return client, received
}

// TestRoutingFieldsSurviveTheWire is the round-trip half of
// GAPI-DIV-102's exit: every routing field the Event declares must come
// back unchanged.
//
// The topic deliberately CONTAINS a '/' and its first segment is a valid
// scope name. Under the old encoding the scope was packed into the topic
// string as a '/'-delimited prefix and recovered by splitting on the
// first '/', so the pair (scope, topic) had no unique encoding: what
// arrives is decided by where the delimiter happens to fall, not by what
// the publisher set.
//
// Namespace and tags are the measured half. Both are declared on the
// Envelope, and before this fix neither was written or read, so an event
// carrying them arrived carrying neither - silently, since nothing in
// the repo sets them yet and no consumer could notice their absence.
func TestRoutingFieldsSurviveTheWire(t *testing.T) {
	client, received := newQUICPair(t, "127.0.0.1:40002")

	sent := eventbus.Event[*anypb.Any]{
		ID:        "routing-1",
		Scope:     "user",
		Namespace: "tenant-b",
		Topic:     "system/ping",
		Source:    "test",
		Tags:      []string{"t1", "t2"},
	}

	if err := client.PublishRemote(context.Background(), sent); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case got := <-received:
		if got.Scope != sent.Scope {
			t.Errorf("scope: want %q, got %q", sent.Scope, got.Scope)
		}
		if got.Namespace != sent.Namespace {
			t.Errorf("namespace: want %q, got %q", sent.Namespace, got.Namespace)
		}
		if got.Topic != sent.Topic {
			t.Errorf("topic: want %q, got %q", sent.Topic, got.Topic)
		}
		if got.Source != sent.Source {
			t.Errorf("source: want %q, got %q", sent.Source, got.Source)
		}
		if !slices.Equal(got.Tags, sent.Tags) {
			t.Errorf("tags: want %v, got %v", sent.Tags, got.Tags)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestWireTopicIsNeverSplit is the structural half of GAPI-DIV-102's
// exit, and it is also the hard cut of operator decision 39 stated as a
// test.
//
// A frame carrying topic "system/ping" and no scope is byte-identical to
// what a PRE-102 sender emits for scope "system", topic "ping". The
// receiver must NOT re-derive a scope from it. It arrives scopeless and
// with the topic whole, which makes it invalid at the bus ingress -
// GAPI-DIV-100's ValidateEvent refuses an event whose scope is not one
// of {system, user, admin} and logs the rejection. That refusal is the
// hard cut: an old sender's frame is dropped loudly rather than
// delivered to a topic its publisher never named.
//
// This is the assertion the deleted splitter cannot pass. It is stated
// behaviourally rather than as a source scan because the property that
// matters is what the receiver DOES with the delimiter, not whether the
// function that used to find it is still spelled out somewhere.
func TestWireTopicIsNeverSplit(t *testing.T) {
	client, received := newQUICPair(t, "127.0.0.1:40003")

	if err := client.PublishRemote(context.Background(), eventbus.Event[*anypb.Any]{
		ID:    "unsplit-1",
		Topic: "system/ping",
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case got := <-received:
		if got.Scope != "" {
			t.Errorf("scope was re-derived from the topic: got %q, want empty", got.Scope)
		}
		if got.Topic != "system/ping" {
			t.Errorf("topic was split: got %q, want %q", got.Topic, "system/ping")
		}
		if err := eventbus.ValidateEvent(got); err == nil {
			t.Error("a scopeless frame must be refused at the bus ingress, but ValidateEvent accepted it")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}
