// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package eventbus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
)

// stubTransport returns a fixed error from every remote publish. It is the
// only way to test the bus's DISCRIMINATION between "nobody to send to"
// and "the send failed": a real QUIC transport reports the second from a
// fire-and-forget goroutine, so it never reaches Publish's return value.
type stubTransport struct {
	err error
}

func (t *stubTransport) PublishRemote(context.Context, Event[*anypb.Any]) error { return t.err }
func (t *stubTransport) OnRemoteEvent(func(Event[*anypb.Any]))                  {}
func (t *stubTransport) Close() error                                           { return nil }

// levelRecorder counts records by level. slog handlers are called from the
// publishing goroutine here, but the mutex costs nothing and removes the
// question.
type levelRecorder struct {
	mu      sync.Mutex
	byLevel map[slog.Level][]string
}

func newLevelRecorder() *levelRecorder {
	return &levelRecorder{byLevel: make(map[slog.Level][]string)}
}

func (r *levelRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *levelRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byLevel[rec.Level] = append(r.byLevel[rec.Level], rec.Message)
	return nil
}

func (r *levelRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *levelRecorder) WithGroup(string) slog.Handler      { return r }

func (r *levelRecorder) messages(l slog.Level) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.byLevel[l]...)
}

// captureLogs redirects the default logger for the duration of the test.
// The bus reads slog.Default() at publish time rather than holding a
// logger, so this is the only interception point available.
func captureLogs(t *testing.T) *levelRecorder {
	t.Helper()
	rec := newLevelRecorder()
	prev := slog.Default()
	slog.SetDefault(slog.New(rec))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return rec
}

func publishOnce(t *testing.T, terr error) *levelRecorder {
	t.Helper()
	rec := captureLogs(t)
	bus := NewEventBus[*anypb.Any](&stubTransport{err: terr})
	t.Cleanup(func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	})
	if err := bus.Publish(NewEvent[*anypb.Any]("system", "", "test.msg", "test", nil)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return rec
}

// The bus's whole job here is discrimination, so the cases are one table:
// a no-peer publish must be quiet and a genuine failure must be loud, and
// GAPI-DIV-095 is only closed if BOTH hold. Either one alone is a fix that
// inverts the defect - silence for everything, or noise for everything.
//
// The wrapped case is not a variation for its own sake. A transport that
// adds context to the sentinel is the normal Go idiom, and matching with
// == instead of errors.Is would reintroduce the noise the moment anyone
// wrote fmt.Errorf("...: %w", ErrNoPeer).
func TestPublishDiscriminatesNoPeerFromFailure(t *testing.T) {
	tests := []struct {
		name         string
		transportErr error
		wantError    bool
	}{
		{
			name:         "no peer",
			transportErr: ErrNoPeer,
			wantError:    false,
		},
		{
			name:         "no peer, wrapped by the transport",
			transportErr: fmt.Errorf("quic: %w", ErrNoPeer),
			wantError:    false,
		},
		{
			name:         "a genuine publish failure",
			transportErr: errors.New("stream reset by peer"),
			wantError:    true,
		},
		{
			name:         "a genuine failure that wraps something else",
			transportErr: fmt.Errorf("publish: %w", io.ErrUnexpectedEOF),
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No t.Parallel: the bus reads slog.Default(), which
			// captureLogs replaces process-wide.
			rec := publishOnce(t, tt.transportErr)
			got := rec.messages(slog.LevelError)

			if tt.wantError && !slices.Contains(got, "transport publish failed") {
				t.Fatalf("expected an ERROR for %v, got %v", tt.transportErr, got)
			}
			if !tt.wantError && len(got) != 0 {
				t.Fatalf("expected no ERROR for %v, got %v", tt.transportErr, got)
			}
		})
	}
}
