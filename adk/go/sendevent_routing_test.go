// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk_test

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	adk "github.com/goppydae/gapi/adk/go"
	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/transport"
)

// TestSendEventReachesASubscriber is GAPI-DIV-100's gate.
//
// It asserts DELIVERY, not survival of a field. The defect it guards
// was invisible for exactly as long as core/transport/quic_test.go was
// the only test of this path: that test publishes a scopeless event,
// asserts the received ID, and never attaches an EventBus - so it stops
// one layer above the key lookup where the drop happens.
//
// SendEvent is the only channel an out-of-process agent has for
// reporting its own state, in either language: the Python runner binds
// straight to it through gopy. An event it publishes that no subscriber
// can be holding is an agent with no event channel at all.
func TestSendEventReachesASubscriber(t *testing.T) {
	server, err := newTestServer(t)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}

	bus := eventbus.NewEventBus[*anypb.Any](server)
	t.Cleanup(func() {
		if cerr := bus.Close(); cerr != nil {
			t.Errorf("close bus: %v", cerr)
		}
	})

	// The subscription the supervisor actually holds
	// (core/lifecycle/controller.go).
	got := make(chan eventbus.Event[*anypb.Any], 1)
	if serr := bus.Subscribe("system", "", eventbus.TopicAgentLifecycleStatus, func(e eventbus.Event[*anypb.Any]) {
		select {
		case got <- e:
		default:
		}
	}); serr != nil {
		t.Fatalf("subscribe: %v", serr)
	}

	// No StopQUIC exists and none is added for a test: gopy binds every
	// exported symbol in this package, so the surface is a constraint
	// rather than a convenience (design/adk-architecture.md). The client
	// is package-level and outlives the test, which is why this is the
	// only test here that starts one.
	if serr := adk.StartQUIC(server.Addr()); serr != nil {
		t.Fatalf("StartQUIC: %v", serr)
	}

	adk.SendEvent(`{"event":"ready","state":"running","id":"routing-probe","run_id":"run-1"}`)

	select {
	case e := <-got:
		if e.Topic != eventbus.TopicAgentLifecycleStatus {
			t.Errorf("topic = %q, want %q", e.Topic, eventbus.TopicAgentLifecycleStatus)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SendEvent published nothing a subscriber could receive")
	}
}

// newTestServer starts a QUIC server on an ephemeral port.
func newTestServer(t *testing.T) (*transport.QUIC, error) {
	t.Helper()

	cert, err := transport.GenerateInsecureSelfSignedCert()
	if err != nil {
		return nil, err
	}
	server, err := transport.NewQUICServer("127.0.0.1:0", cert)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		if cerr := server.Close(); cerr != nil {
			t.Errorf("close server: %v", cerr)
		}
	})
	return server, nil
}
