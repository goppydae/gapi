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
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	quic "github.com/quic-go/quic-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/internal/logattr"
	protopkg "github.com/goppydae/gapi/pkg/proto"
)

// ErrEnvelopeTooLarge reports an envelope whose byte length cannot be
// expressed in the four-byte frame prefix. It is OURS, not the network's
// - nothing was written and nothing failed on the wire - so it is a
// sentinel here rather than a wrapped transport error.
var ErrEnvelopeTooLarge = errors.New("transport: envelope too large to frame")

// sendStage names the step of a single peer's send that failed. It is a
// value, not a formatted string, so a caller can ask WHICH step went
// wrong without parsing a message - the distinction matters, because an
// open failure means the peer is gone while a write failure means it
// died mid-frame.
type sendStage string

const (
	stageOpen     sendStage = "open"
	stageMarshal  sendStage = "marshal"
	stageFrame    sendStage = "frame"
	stageWrite    sendStage = "write"
	stageCloseErr sendStage = "close"
)

// PeerSendError reports one peer's failed send.
type PeerSendError struct {
	Stage sendStage
	Peer  string
	Err   error
}

func (e *PeerSendError) Error() string {
	return fmt.Sprintf("publish to %s failed at %s: %v", e.Peer, e.Stage, e.Err)
}

func (e *PeerSendError) Unwrap() error { return e.Err }

// PublishIncomplete reports a remote publish that did not confirm
// delivery to every peer.
//
// FAILED AND UNCONFIRMED ARE DIFFERENT FACTS AND ARE COUNTED
// SEPARATELY. A failed peer reached a named error; an unconfirmed one
// had not finished when the confirmation window closed, and its send may
// still complete afterwards. Collapsing the two would make the error say
// "this did not arrive" where the honest claim is "this is not known to
// have arrived".
type PublishIncomplete struct {
	// Peers is how many peers the publish addressed.
	Peers int
	// Failed is how many reported a named error.
	Failed int
	// Unconfirmed is how many had not answered when the window closed.
	Unconfirmed int
	// Errs holds one PeerSendError per failed peer.
	Errs []error
}

func (e *PublishIncomplete) Error() string {
	return fmt.Sprintf("publish confirmed for %d of %d peers (%d failed, %d unconfirmed)",
		e.Peers-e.Failed-e.Unconfirmed, e.Peers, e.Failed, e.Unconfirmed)
}

// Unwrap returns the per-peer errors so errors.Is and errors.As reach
// them. Only FAILED peers appear here; an unconfirmed peer has no error
// to report, which is precisely what makes it unconfirmed.
func (e *PublishIncomplete) Unwrap() []error { return e.Errs }

// PublishRemote sends one event to every attached peer and reports
// whether the bytes went out.
//
// A nil RETURN NOW ASSERTS DELIVERY TO THE WIRE, AND IT PREVIOUSLY
// ASSERTED NOTHING. This function used to spawn a goroutine per peer and
// return nil immediately, so nil meant "some work was scheduled" - and
// measured on gapi #136, sometimes not even that: ten failing gapictl
// clients logged `event=dial` and NOT ONE reached the send goroutine's
// first line. The client gave up on its 2s deadline and exited before
// the runtime scheduled the goroutine, so OpenStreamSync never ran,
// nothing was written, and no error path was reached. That is why all
// seven of the send path's error paths reported zero while requests
// demonstrably vanished: nothing was failing, nothing was happening.
//
// WHAT nil MEANS, STATED PRECISELY, BECAUSE THE WHOLE DEFECT WAS AN
// OVERCLAIMED RETURN VALUE: every peer's frame was marshalled, written
// and its write side closed. It does NOT mean any peer acknowledged or
// handled the event - this layer cannot know that, and a return value
// that implied it would repeat the mistake one level up.
//
// THE FAN-OUT IS UNCHANGED; ONLY THE WAITING IS NEW. The sends still run
// concurrently, one goroutine per peer, so no peer's send is sequenced
// behind another's timeout and a publish reaching some peers and not
// others still does not depend on their order. What the old comment
// defended - one unresponsive peer must not block every publisher - is
// preserved by the concurrency plus the bounded window below, not by
// refusing to wait at all.
func (q *QUIC) PublishRemote(ctx context.Context, e eventbus.Event[*anypb.Any]) error {
	// Snapshot under the mutex. Sending while holding it would let one
	// unresponsive peer block every other publisher.
	q.mu.Lock()
	peers := make([]*quic.Conn, 0, len(q.peers))
	for c := range q.peers {
		peers = append(peers, c)
	}
	q.mu.Unlock()

	if len(peers) == 0 {
		// An EMPTINESS check now, where it was a nil check; GAPI-DIV-095's
		// reasoning is unchanged. NOT io.ErrUnexpectedEOF, which asserts
		// that a read ended before it should have. Nothing was read and
		// nothing went wrong: there is simply nobody to send to.
		return eventbus.ErrNoPeer
	}

	// BUFFERED TO len(peers), which is what makes abandoning the wait
	// safe. A goroutine that finishes after the window closed still has
	// somewhere to put its result and exits; an unbuffered channel would
	// leak every one of them.
	results := make(chan error, len(peers))
	for _, conn := range peers {
		go func(c *quic.Conn) { results <- q.publishTo(ctx, c, e) }(conn)
	}

	// THE WINDOW BOUNDS THE WAIT, NOT THE SEND. When it closes the
	// goroutines are deliberately NOT cancelled - each keeps its own
	// QUICStreamTimeout and logs its own outcome - so no send gets less
	// time to complete than it has today. Only the caller's patience is
	// bounded, and today that patience is zero.
	window := time.NewTimer(config.PublishConfirmTimeout)
	defer window.Stop()

	var errs []error
	answered := 0
collect:
	for range peers {
		select {
		case perr := <-results:
			answered++
			if perr != nil {
				errs = append(errs, perr)
			}
		case <-window.C:
			break collect
		}
	}

	if len(errs) == 0 && answered == len(peers) {
		return nil
	}
	return &PublishIncomplete{
		Peers:       len(peers),
		Failed:      len(errs),
		Unconfirmed: len(peers) - answered,
		Errs:        errs,
	}
}

// publishTo sends one event to one peer and reports what happened.
//
// EVERY EXIT IS LOGGED AND EVERY EXIT IS RETURNED. The logging came
// first, on its own, because a lost send had to become VISIBLE before it
// could be located - each failure path here used to be a bare `return`
// with no error and no log. Returning them is the second half: a log
// tells a reader what happened, a return tells the CALLER, and only the
// caller can decline to report success.
func (q *QUIC) publishTo(ctx context.Context, conn *quic.Conn, e eventbus.Event[*anypb.Any]) error {
	traceSendStart(e)

	peer := conn.RemoteAddr().String()

	timeoutCtx, cancel := context.WithTimeout(ctx, config.QUICStreamTimeout)
	defer cancel()
	s, err := conn.OpenStreamSync(timeoutCtx)
	if err != nil {
		// The event id is the join key. Without it a reader can see
		// that A publish failed but not WHICH request vanished, and
		// correlating a lost request to a client's timeout is the
		// entire diagnostic task.
		slog.Default().LogAttrs(ctx, slog.LevelWarn, "publish stream open failed",
			logattr.Module("transport"), logattr.EventID(e.ID),
			logattr.Topic(e.Topic), logattr.Err(err))
		return &PeerSendError{Stage: stageOpen, Peer: peer, Err: err}
	}

	// The write side is closed on EVERY path, and its error is returned
	// on the success path only. A close error can mean the frame did not
	// flush, so it must not be swallowed - but on a path that already
	// failed, the first error is the one worth reporting.
	closed := false
	defer func() {
		if closed {
			return
		}
		if cerr := s.Close(); cerr != nil {
			slog.Default().LogAttrs(ctx, slog.LevelWarn, "close publish stream failed",
				logattr.Module("transport"), logattr.EventID(e.ID), logattr.Err(cerr))
		}
	}()

	// Every routing value the Event declares is written to the field
	// that declares it (GAPI-DIV-102). Namespace and tags were
	// declared on the Envelope and written by nobody, so they were
	// dropped on every publish in the system's life.
	//
	// Event.Broadcast is deliberately absent, and it is now absent
	// from the Event too (GAPI-DIV-106). It was a flag with no
	// receiver: this transport addressed ONE peer, so "broadcast"
	// and "publish" were the same operation and the two bus arms
	// called the same code. With a peer set, a remote publish IS to
	// every peer, so the flag has nothing left to select and the
	// distinction it encoded never existed on the wire.
	env := &protopkg.Envelope{
		Id:        e.ID,
		Scope:     e.Scope,
		Namespace: e.Namespace,
		Topic:     e.Topic,
		Source:    e.Source,
		Type:      "event",
		Payload:   e.Payload,
		Tags:      e.Tags,
	}

	data, err := proto.Marshal(env)
	if err != nil {
		slog.Default().LogAttrs(ctx, slog.LevelError, "marshal envelope failed",
			logattr.Module("transport"), logattr.EventID(e.ID), logattr.Err(err))
		return &PeerSendError{Stage: stageMarshal, Peer: peer, Err: err}
	}

	// Length prefix. The receiver caps messages far below this, but
	// the conversion itself must be provably in range.
	dataLen := len(data)
	if dataLen > math.MaxUint32 {
		slog.Default().LogAttrs(ctx, slog.LevelError, "envelope too large to frame",
			logattr.Module("transport"), logattr.EventID(e.ID), logattr.Bytes(dataLen))
		return &PeerSendError{Stage: stageFrame, Peer: peer, Err: ErrEnvelopeTooLarge}
	}
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(dataLen))
	if _, err := s.Write(lenBuf); err != nil {
		slog.Default().LogAttrs(ctx, slog.LevelWarn, "publish length prefix failed",
			logattr.Module("transport"), logattr.EventID(e.ID),
			logattr.Topic(e.Topic), logattr.Err(err))
		return &PeerSendError{Stage: stageWrite, Peer: peer, Err: err}
	}
	if _, err := s.Write(data); err != nil {
		slog.Default().LogAttrs(ctx, slog.LevelWarn, "publish payload write failed",
			logattr.Module("transport"), logattr.EventID(e.ID),
			logattr.Topic(e.Topic), logattr.Err(err))
		return &PeerSendError{Stage: stageWrite, Peer: peer, Err: err}
	}

	// CLOSED HERE, NOT IN THE DEFER, because its error is part of the
	// answer. Close sends the FIN that tells the peer the frame is
	// complete; a close that fails may mean the payload never flushed,
	// and reporting nil after it would be the same overclaim this whole
	// change exists to remove.
	closed = true
	if cerr := s.Close(); cerr != nil {
		slog.Default().LogAttrs(ctx, slog.LevelWarn, "close publish stream failed",
			logattr.Module("transport"), logattr.EventID(e.ID), logattr.Err(cerr))
		return &PeerSendError{Stage: stageCloseErr, Peer: peer, Err: cerr}
	}
	return nil
}
