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

	quic "github.com/quic-go/quic-go"

	"github.com/goppydae/gapi/internal/logattr"
)

// Accept tracing exists to LOCATE a loss that is already known to
// happen and is not yet known to happen ANYWHERE in particular.
//
// Measured on gapi #136: a control request is published with no error
// on any of the send path's three failure modes, on a connection that
// is demonstrably alive - the client goes on receiving heartbeats
// throughout - and its event id never appears in a receive record,
// while thirteen sibling requests on the same topic arrive normally.
//
// So the frame leaves the client and does not reach handleStream. That
// bounds the loss between a completed write and the daemon accepting
// the stream, and it does not narrow it further. TWO CANDIDATES REMAIN
// AND THESE TWO LINES SEPARATE THEM IN ONE RUN:
//
//	connection never accepted  -> the loss is in dial/accept
//	connection accepted,
//	stream never accepted      -> the loss is inside the connection
//
// The remote address is the join key rather than a counter, because
// every gapictl invocation is a fresh process and therefore a fresh
// ephemeral port. That makes the address unique per client for the
// window that matters, and it is a value the CLIENT can also print, so
// the two processes' logs join without a shared identifier having to be
// invented and threaded through.
//
// THIS IS DELIBERATELY NOT A FIX. A retry on the lifecycle publish
// would probably mask the symptom, and GAPI-DIV-122 narrowed retries to
// idempotent READS on purpose. Guessing a fix before the loss is
// located is what produced three different attributions of this defect
// across three pull requests.
//
// It lives in its own file because core/transport/quic.go is at 496 of
// its 500-line ceiling. Splitting on the seam that is actually about
// something - arrival tracing - beats trimming prose out of the file
// that explains why the peer set is a set.
func traceAccept(conn *quic.Conn) {
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "structured event",
		logattr.Module("transport"), logattr.Event("accept_conn"),
		logattr.Addr(conn.RemoteAddr().String()))
}

// traceAcceptStream records that a peer-opened stream reached the
// accept loop. It is logged BEFORE the handler is dispatched, for the
// same reason the receive record is: the claim is "this arrived", not
// "a handler ran", and folding handler latency into an arrival record
// makes the arrival unusable as a boundary.
func traceAcceptStream(conn *quic.Conn, s *quic.Stream) {
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "structured event",
		logattr.Module("transport"), logattr.Event("accept_stream"),
		logattr.Addr(conn.RemoteAddr().String()),
		// Int64, not Uint64. quic.StreamID IS an int64, so widening it
		// through uint64 is a sign conversion gosec correctly flags as
		// G115 - and waiving that would trade a real overflow class for
		// a log field's cosmetics. Stream ids are non-negative in
		// practice; printing one as signed costs nothing.
		slog.Int64("stream_id", int64(s.StreamID())))
}
