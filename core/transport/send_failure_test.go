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
	"log/slog"
	"testing"
	"time"

	quic "github.com/quic-go/quic-go"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
)

// A PUBLISH THAT FAILS TO SEND MUST SAY SO.
//
// PublishRemote USED TO return nil once the peer set was non-empty, with
// the actual send in a goroutine whose every failure mode was a bare
// `return` - no error, no log, nothing. So "Publish returned nil" meant
// "a goroutine was spawned", not "the bytes went out", and a lost
// request was indistinguishable from one the daemon answered slowly.
//
// That ambiguity is not hypothetical: it is why five occurrences of the
// test/adk lifecycle failure were attributed to three different causes
// across as many pull requests. The system logs every transmission and
// logged no reception, so no log could locate a loss.
//
// This asserts the send path LOGS its failure. That the failure is also
// RETURNED is asserted in publish_join_test.go; the two halves are kept
// apart because the log is what located this defect and the return is
// what fixes it, and a log that stopped being emitted would otherwise
// pass unnoticed behind the error check.
//
// THE PEER SET IS BUILT DIRECTLY rather than through NewQUICClient,
// because handleConn removes a dead connection when AcceptStream errors.
// Dialling and then closing would race the eviction: sometimes the
// publish finds an empty set and returns ErrNoPeer, which is a DIFFERENT
// path and would make this test pass for the wrong reason roughly half
// the time. Owning the map removes the race instead of tolerating it.
func TestPublishLogsSendFailure(t *testing.T) {
	conn := dialThenClose(t)

	records := captureLogs(t)

	q := &QUIC{peers: map[*quic.Conn]struct{}{conn: {}}}

	payload, err := anypb.New(&anypb.Any{})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	ev := eventbus.NewEvent("system", "", "lost-topic", "test", payload)

	// THIS ASSERTION WAS INVERTED, AND THE OLD ONE WAS THE DEFECT WRITTEN
	// DOWN AS A PROPERTY. It required nil here, reasoning that a
	// non-empty peer set means the send is asynchronous - true of the
	// code as it stood, and exactly what made a lost request
	// unreportable. The fan-out is joined now, so a peer that cannot be
	// opened is a returned error. TestPublishRemoteReportsDeadPeer is
	// what this half became.
	if perr := q.PublishRemote(context.Background(), ev); perr == nil {
		t.Fatal("PublishRemote returned nil for an already-closed peer; a send that did not happen must be reported")
	}

	rec := awaitFailureRecord(t, records, ev.ID)
	if rec.Level < slog.LevelWarn {
		t.Errorf("send failure logged at %v; a dropped publish must be at least WARN", rec.Level)
	}
}

// A RECEIVED EVENT MUST BE LOGGED, because a send log alone cannot
// locate a loss.
//
// eventbus.Publish records every transmission. The receive path recorded
// only its three error cases and never a success, so "the request was
// published" and "the request arrived" were never both observable. A
// timeout could then mean the frame was dropped in flight OR that it
// landed and the answer was slow, and no log distinguished them.
//
// The assertion is on the SENDER'S event id appearing in a receive
// record, because the id is what joins the two logs. A receipt line that
// did not carry it would prove a frame arrived without saying which.
func TestReceiveLogsArrival(t *testing.T) {
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

	// The peer set is populated by handleConn in a goroutine, so a
	// publish issued the instant after New can find it empty and return
	// ErrNoPeer. Waiting for membership tests the receive path rather
	// than that race.
	awaitPeer(t, client)

	records := captureLogs(t)

	payload, err := anypb.New(&anypb.Any{})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	ev := eventbus.NewEvent("system", "", "arrival-topic", "test", payload)

	if perr := client.PublishRemote(context.Background(), ev); perr != nil {
		t.Fatalf("publish: %v", perr)
	}

	rec := awaitTracedRecord(t, records, ev.ID, "receive")
	if !recordHasValue(rec, "module", "transport") {
		t.Errorf("receive record for %s is not from the transport; arrival must be recorded where the frame arrives, not by a later handler", ev.ID)
	}
}

func awaitPeer(t *testing.T, q *QUIC) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		n := len(q.peers)
		q.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("client peer set still empty after 5s")
}

// dialThenClose returns a connection that is already closed, so
// OpenStreamSync fails immediately and deterministically.
func dialThenClose(t *testing.T) *quic.Conn {
	t.Helper()

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
	if cerr := conn.CloseWithError(0, "closed by test"); cerr != nil {
		t.Fatalf("close conn: %v", cerr)
	}
	return conn
}

// captureLogs redirects the default logger for the duration of one test
// and returns a channel of records. Buffered well past what one publish
// emits, so an unrelated log line cannot block the handler.
func captureLogs(t *testing.T) chan slog.Record {
	t.Helper()

	records := make(chan slog.Record, 64)
	prev := slog.Default()
	slog.SetDefault(slog.New(&channelHandler{records: records}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return records
}

// awaitRecord waits for a record carrying the given event id. The send
// is asynchronous, so a bare channel read with no deadline would hang
// the suite rather than fail it when the defect is present.
// MATCHING ON THE EVENT ID ALONE WAS NOT ENOUGH, AND THE PROBE PROVED IT
// BY BREAKING BOTH CALLERS.
//
// One helper used to return the first record carrying the id, on the
// reasonable-sounding assumption that a publish emits one line about
// itself. Adding traceSendStart - which records that a send goroutine
// RAN, at INFO, with the event id - made that first record the probe's
// own, so TestPublishLogsSendFailure read INFO where it demanded WARN
// and TestReceiveLogsArrival read a send line where it demanded an
// arrival. Both failed on the branch that added the probe, and neither
// failure was about the thing it asserted.
//
// So the two waits are separate now and each names what it will accept.
// An instrument that changes what the harness measures is the same
// family of defect as a probe that cannot fail: the fix is to make the
// selection explicit rather than to trust an ordering.

// awaitTracedRecord waits for a structured trace record - one carrying an
// `event` field with the given value - that names the event id. The
// deadline exists because the send is concurrent: a bare channel read
// would hang the suite rather than fail it when the defect is present.
func awaitTracedRecord(t *testing.T, records chan slog.Record, eventID, event string) slog.Record {
	t.Helper()

	return awaitMatchingRecord(t, records, eventID,
		func(rec slog.Record) bool { return recordHasValue(rec, "event", event) },
		"trace record event="+event)
}

// awaitFailureRecord waits for a record that names the event id and is
// NOT a trace record. Failure lines carry no `event` field - they are
// prose about something going wrong, not a point on the wire - and that
// absence is what distinguishes them from the traces interleaved with
// them.
func awaitFailureRecord(t *testing.T, records chan slog.Record, eventID string) slog.Record {
	t.Helper()

	return awaitMatchingRecord(t, records, eventID,
		func(rec slog.Record) bool { return !recordHasKey(rec, "event") },
		"failure record")
}

func awaitMatchingRecord(t *testing.T, records chan slog.Record, eventID string, match func(slog.Record) bool, want string) slog.Record {
	t.Helper()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case rec := <-records:
			if recordHasValue(rec, "event_id", eventID) && match(rec) {
				return rec
			}
		case <-deadline:
			t.Fatalf("no %s naming event %s within 5s", want, eventID)
		}
	}
}

func recordHasKey(rec slog.Record, key string) bool {
	found := false
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = true
			return false
		}
		return true
	})
	return found
}

func recordHasValue(rec slog.Record, key, want string) bool {
	found := false
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == key && a.Value.String() == want {
			found = true
			return false
		}
		return true
	})
	return found
}

type channelHandler struct {
	records chan slog.Record
}

func (h *channelHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *channelHandler) Handle(_ context.Context, rec slog.Record) error {
	select {
	case h.records <- rec:
	default:
	}
	return nil
}

func (h *channelHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *channelHandler) WithGroup(string) slog.Handler { return h }
