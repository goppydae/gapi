// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package eventbus

import (
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
)

// THE TWO PUBLISHES MUST DISAGREE, AND THAT DISAGREEMENT IS THE FEATURE.
//
// Publish returns nil for a remote send that failed. That is deliberate
// for an announcement - GAPI-DIV-095 demoted ErrNoPeer precisely because
// a daemon publishing with nobody attached has not failed - and it is
// fatal for a REQUEST, whose sender is about to wait for a reply.
//
// Measured on gapi #136 job 93050924850: `gapictl status` published one
// event on topic agents/ whose id appears exactly once in the whole run,
// then reported "context deadline exceeded" 30 seconds later.
// Client.AgentStatus publishes once and waits, so one demoted ErrNoPeer
// costs the entire deadline and the timeout is then attributed to the
// daemon. This table is what stops that from being reintroduced by
// someone tidying two methods into one.
//
// A TEST THAT ONLY CHECKED PublishRequest WOULD PASS WITH Publish MADE
// STRICT TOO, which would undo GAPI-DIV-095 - so both columns are
// asserted on every case.
func TestPublishRequestReportsWhatPublishDemotes(t *testing.T) {
	tests := []struct {
		name            string
		transportErr    error
		wantPublishErr  bool
		wantRequestErr  bool
		requestIsNoPeer bool
	}{
		{
			name:            "no peer",
			transportErr:    ErrNoPeer,
			wantPublishErr:  false,
			wantRequestErr:  true,
			requestIsNoPeer: true,
		},
		{
			name:            "no peer, wrapped by the transport",
			transportErr:    fmt.Errorf("quic: %w", ErrNoPeer),
			wantPublishErr:  false,
			wantRequestErr:  true,
			requestIsNoPeer: true,
		},
		{
			name:           "a genuine publish failure",
			transportErr:   errors.New("stream reset by peer"),
			wantPublishErr: false,
			wantRequestErr: true,
		},
		{
			name:           "a partial delivery",
			transportErr:   fmt.Errorf("publish: %w", io.ErrUnexpectedEOF),
			wantPublishErr: false,
			wantRequestErr: true,
		},
		{
			name:           "the send succeeded",
			transportErr:   nil,
			wantPublishErr: false,
			wantRequestErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := NewEventBus[*anypb.Any](&stubTransport{err: tt.transportErr})
			t.Cleanup(func() {
				if err := bus.Close(); err != nil {
					t.Errorf("close bus: %v", err)
				}
			})

			ev := NewEvent[*anypb.Any]("system", "", "test.msg", "test", nil)

			perr := bus.Publish(ev)
			if (perr != nil) != tt.wantPublishErr {
				t.Errorf("Publish = %v, want error: %v (an announcement does not fail on the transport)", perr, tt.wantPublishErr)
			}

			rerr := bus.PublishRequest(ev)
			if (rerr != nil) != tt.wantRequestErr {
				t.Fatalf("PublishRequest = %v, want error: %v (a request must know its send did not happen)", rerr, tt.wantRequestErr)
			}
			if tt.requestIsNoPeer && !errors.Is(rerr, ErrNoPeer) {
				t.Errorf("PublishRequest = %v; the sentinel must survive so a caller can tell 'nobody there' from 'the send broke'", rerr)
			}
		})
	}
}

// A REQUEST STILL DISPATCHES LOCALLY. PublishRequest returns the
// transport error FIRST, which must not be mistaken for skipping local
// delivery - an in-process subscriber has to see the event either way, or
// a single-node daemon would stop working the moment it had no peer.
func TestPublishRequestStillDispatchesLocally(t *testing.T) {
	bus := NewEventBus[*anypb.Any](&stubTransport{err: ErrNoPeer})
	t.Cleanup(func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	})

	got := make(chan Event[*anypb.Any], 1)
	if err := bus.Subscribe("system", "", "test.msg", func(e Event[*anypb.Any]) { got <- e }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	ev := NewEvent[*anypb.Any]("system", "", "test.msg", "test", nil)
	if err := bus.PublishRequest(ev); !errors.Is(err, ErrNoPeer) {
		t.Fatalf("PublishRequest = %v, want ErrNoPeer", err)
	}

	select {
	case delivered := <-got:
		if delivered.ID != ev.ID {
			t.Errorf("delivered event %s, want %s", delivered.ID, ev.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("local subscriber never received the event; a failed remote send must not suppress local dispatch")
	}
}
