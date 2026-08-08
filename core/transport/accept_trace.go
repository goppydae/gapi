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
	"net"

	quic "github.com/quic-go/quic-go"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/internal/logattr"
)

// tracePort is THE join key, and the full address is not.
//
// A client's LocalAddr reports the wildcard it bound - "[::]:55291" -
// while the daemon reports the same connection as "127.0.0.1:55291".
// The ports agree and the strings do not, so joining on the address
// silently matches nothing. Found by running it: the first version of
// this tracing logged addresses on both sides and the grep that was
// supposed to demonstrate the join returned empty.
//
// The port alone is sufficient here because every gapictl invocation is
// a fresh process holding a fresh ephemeral port for its lifetime. It
// is NOT unique forever - the kernel reuses ports - so a join across a
// long window must still check ordering. Within one test it is exact.
func tracePort(addr net.Addr) slog.Attr {
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return slog.String("port", "unknown")
	}
	return slog.String("port", port)
}

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
// ANSWERED ON THE FIRST RUN THAT CARRIED IT: 29 connections accepted,
// 26 streams accepted, 26 envelopes received. The read path is exact -
// every accepted stream produced an envelope - and THREE connections
// were accepted and never yielded a stream at all. The loss is inside
// an established connection, which is none of the three places five
// occurrences of this defect were attributed to.
//
// The join key is the PORT, not the address - see tracePort.
//
// THIS IS DELIBERATELY NOT A FIX. A retry on the lifecycle publish
// would probably mask the symptom, and GAPI-DIV-122 narrowed retries to
// idempotent READS on purpose. Guessing a fix before the loss is
// located is what produced three different attributions of this defect
// across three pull requests.
//
// It lives in its own file because core/transport/quic.go is within a
// couple of lines of its 500-line ceiling. Splitting on the seam that
// is actually about something - arrival tracing - beats trimming prose
// out of the file that explains why the peer set is a set.
func traceAccept(conn *quic.Conn) {
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "structured event",
		logattr.Module("transport"), logattr.Event("accept_conn"),
		logattr.Addr(conn.RemoteAddr().String()), tracePort(conn.RemoteAddr()))
}

// traceSendStart records that a publish goroutine actually RAN.
//
// It tests one hypothesis and it is worth stating so the next reader can
// discard it cleanly if the log refutes it: PublishRemote spawns a
// goroutine per peer and returns nil immediately, so "Publish returned
// nil" may mean not merely that the bytes have not gone out yet, but
// that the send NEVER STARTED. If the client gives up and exits before
// that goroutine is scheduled, OpenStreamSync never runs, no frame is
// ever written, and every observation we have is explained:
//
//	no error logged        the goroutine reached no error path
//	accept_conn present    the dial happened synchronously in New
//	accept_stream absent   nothing was ever sent
//	rate tracks load       goroutine scheduling, not peer count
//
// THE PREDICTION IS EXACT: for a request whose port has accept_conn and
// no accept_stream, this line is ABSENT. If it is PRESENT, the goroutine
// ran and the loss is downstream of it - the hypothesis dies and the
// next place to look is quic-go's stream lifecycle.
//
// A CAVEAT THAT MAY YET KILL IT: this explains the 2s
// ClientPendingTimeout failures well, and explains the 60s poll-loop
// failures badly, since a goroutine has an implausibly long time to be
// scheduled in 60 seconds. Either those are a different path or the
// hypothesis is incomplete. Do not read a confirmation on the short
// cases as covering the long ones.
func traceSendStart(e eventbus.Event[*anypb.Any]) {
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "structured event",
		logattr.Module("transport"), logattr.Event("send_start"),
		logattr.EventID(e.ID), logattr.Topic(e.Topic))
}

// traceDial records the local endpoint a client dialled from.
//
// IT COMPLETES A JOIN THAT WAS PREVIOUSLY A TIMESTAMP COMPARISON. On
// gapi #136 a lost request was matched to an orphaned connection by
// noticing that the client published at 01:28:30 and a connection
// accepted at 01:28:30.126 never yielded a stream. That is a correct
// inference and a fragile one: it needs sub-second log alignment across
// two processes, and it fails outright once two clients overlap.
//
// The ADDRESS here is not the daemon's address for the same connection
// - a client binds a wildcard and reports "[::]:55291" where the daemon
// reports "127.0.0.1:55291" - so both sides also emit a `port` field
// and THAT is what joins. The full address is kept because it says
// which interface was bound, which the port does not.
//
// A lost request is then one grep: a port the client dialled from that
// has no accept_stream against it.
func traceDial(conn *quic.Conn) {
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "structured event",
		logattr.Module("transport"), logattr.Event("dial"),
		logattr.Addr(conn.LocalAddr().String()), tracePort(conn.LocalAddr()))
}

// traceAcceptStream records that a peer-opened stream reached the
// accept loop. It is logged BEFORE the handler is dispatched, for the
// same reason the receive record is: the claim is "this arrived", not
// "a handler ran", and folding handler latency into an arrival record
// makes the arrival unusable as a boundary.
func traceAcceptStream(conn *quic.Conn, s *quic.Stream) {
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "structured event",
		logattr.Module("transport"), logattr.Event("accept_stream"),
		logattr.Addr(conn.RemoteAddr().String()), tracePort(conn.RemoteAddr()),
		// Int64, not Uint64. quic.StreamID IS an int64, so widening it
		// through uint64 is a sign conversion gosec correctly flags as
		// G115 - and waiving that would trade a real overflow class for
		// a log field's cosmetics. Stream ids are non-negative in
		// practice; printing one as signed costs nothing.
		slog.Int64("stream_id", int64(s.StreamID())))
}
