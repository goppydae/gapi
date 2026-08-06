// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package transport_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/transport"
)

// TestUnroutableRemoteEventIsRejectedNotDropped is the second half of
// GAPI-DIV-100's gate.
//
// Publish refuses an event whose scope is not one of {system, user,
// admin} (core/eventbus ValidateEvent). The remote path never reached
// that refusal: NewEventBus wires the transport's OnRemoteEvent straight
// into dispatch, so the ONE ingress that accepts bytes from another
// process was the one ingress with no validation, and an unroutable
// event produced neither an error to the publisher nor a line in the
// daemon's log.
//
// The assertion is on the log because that is the observable: an ingress
// callback has no error to return to anyone. Silence is what made the
// original defect cost a day, so silence is what this test forbids.
func TestUnroutableRemoteEventIsRejectedNotDropped(t *testing.T) {
	logbuf := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logbuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cert, err := transport.GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	server, err := transport.NewQUICServer("127.0.0.1:0", cert)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	t.Cleanup(func() {
		if cerr := server.Close(); cerr != nil {
			t.Errorf("close server: %v", cerr)
		}
	})

	bus := eventbus.NewEventBus[*anypb.Any](server)
	t.Cleanup(func() {
		if cerr := bus.Close(); cerr != nil {
			t.Errorf("close bus: %v", cerr)
		}
	})

	client, err := transport.NewQUICClient(server.Addr(), nil, transport.TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	t.Cleanup(func() {
		if cerr := client.Close(); cerr != nil {
			t.Errorf("close client: %v", cerr)
		}
	})
	time.Sleep(100 * time.Millisecond)

	// Scope unset, and a topic whose first segment is not a valid scope.
	// The receiver splits on the first '/', manufacturing scope="nope",
	// which ValidateEvent rejects.
	unroutable := eventbus.Event[*anypb.Any]{ID: "unroutable-1", Topic: "nope/whatever"}
	if perr := client.PublishRemote(context.Background(), unroutable); perr != nil {
		t.Fatalf("publish: %v", perr)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logbuf.String(), "unroutable-1") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("an unroutable remote event was dropped without a word; log was:\n%s", logbuf.String())
}

// lockedBuffer serialises the handler goroutine's writes against the
// test goroutine's reads.
//
// slog writes from whichever goroutine logs - here, the QUIC accept
// loop - while the assertion polls from the test goroutine. A bare
// bytes.Buffer is a data race that -race reports and a plain run does
// not, which is the kind of test that passes until it matters.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
