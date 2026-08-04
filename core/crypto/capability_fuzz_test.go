// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package crypto_test

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/crypto"
	gapiv1 "github.com/goppydae/gapi/pkg/proto"
	"google.golang.org/protobuf/proto"
)

// Capability-token verification is the kernel's authorization boundary.
// VerifyCapabilityToken takes a *gapiv1.CapabilityToken, but nothing on
// the wire is a struct: the hostile input is BYTES, so this target
// fuzzes []byte through proto.Unmarshal into the token and then through
// verification.
//
// Everything the verifier consults is pinned: one key, derived from a
// fixed ed25519 seed, one resolver that knows only that key, and one
// fixed "now". A crasher therefore replays from its seed alone.

const fuzzKeyID = "fuzz-key"

var (
	fuzzSeed = bytes.Repeat([]byte{0x2a}, ed25519.SeedSize)
	fuzzPriv = ed25519.NewKeyFromSeed(fuzzSeed)
	fuzzPub  = fuzzPriv.Public().(ed25519.PublicKey)
	fuzzNow  = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
)

func fuzzResolve(keyID string) (ed25519.PublicKey, bool) {
	if keyID != fuzzKeyID {
		return nil, false
	}
	return fuzzPub, true
}

// mustToken builds and marshals a token for the seed corpus. mutate runs
// on the wire token before marshalling, so a seed can carry a tampered
// signature or a truncated field.
func mustToken(f *testing.F, p *gapiv1.CapabilityTokenPayload, mutate func(*gapiv1.CapabilityToken)) []byte {
	f.Helper()
	tok, err := crypto.SignCapabilityToken(p, fuzzPriv)
	if err != nil {
		f.Fatalf("seed: sign: %v", err)
	}
	if mutate != nil {
		mutate(tok)
	}
	raw, err := proto.Marshal(tok)
	if err != nil {
		f.Fatalf("seed: marshal: %v", err)
	}
	return raw
}

func fuzzPayload() *gapiv1.CapabilityTokenPayload {
	return &gapiv1.CapabilityTokenPayload{
		TokenId:      []byte("fuzz-token-id"),
		SubjectUuid:  []byte("fuzz-subject"),
		Rights:       crypto.RightSignalTerm | crypto.RightSignalKill,
		IssuedAtMs:   fuzzNow.Add(-time.Minute).UnixMilli(),
		ExpiresAtMs:  fuzzNow.Add(time.Minute).UnixMilli(),
		IssuerNodeId: "fuzz-node",
		KeyId:        fuzzKeyID,
	}
}

// FuzzVerifyCapabilityToken asserts:
//
//   - totality: arbitrary bytes decoded as a CapabilityToken never panic
//     the verifier, and it returns a payload or an error, never both and
//     never neither;
//   - errors are data: every rejection matches one of the five exported
//     sentinels. An untyped error escaping here would leave callers
//     branching on message strings;
//   - an acceptance is sound. The fuzzer cannot forge ed25519, so any
//     accepted token must be one the fixed key really signed, still
//     inside its validity window at the fixed now, and carrying the
//     rights that were demanded. The signature must verify over the
//     LITERAL payload bytes - protobuf is not canonical, so a verifier
//     that re-serialized before checking would be forgeable;
//   - determinism: the same bytes verify to the same verdict twice.
func FuzzVerifyCapabilityToken(f *testing.F) {
	// A genuinely valid token: this is what makes the accept path live.
	f.Add(mustToken(f, fuzzPayload(), nil))

	// Tampered signature: one flipped bit.
	f.Add(mustToken(f, fuzzPayload(), func(tok *gapiv1.CapabilityToken) {
		tok.Signature[0] ^= 0x01
	}))

	// Tampered payload after signing.
	f.Add(mustToken(f, fuzzPayload(), func(tok *gapiv1.CapabilityToken) {
		tok.Payload = append(tok.Payload, 0x00)
	}))

	// Truncated signature: wrong length, must fail structurally.
	f.Add(mustToken(f, fuzzPayload(), func(tok *gapiv1.CapabilityToken) {
		tok.Signature = tok.Signature[:32]
	}))

	// Expired.
	expired := fuzzPayload()
	expired.IssuedAtMs = fuzzNow.Add(-2 * time.Hour).UnixMilli()
	expired.ExpiresAtMs = fuzzNow.Add(-time.Hour).UnixMilli()
	f.Add(mustToken(f, expired, nil))

	// Boundary: expires exactly at now (verification is >=, so rejected).
	atBoundary := fuzzPayload()
	atBoundary.ExpiresAtMs = fuzzNow.UnixMilli()
	f.Add(mustToken(f, atBoundary, nil))

	// Inverted window: expires_at before issued_at.
	inverted := fuzzPayload()
	inverted.IssuedAtMs = fuzzNow.Add(time.Hour).UnixMilli()
	inverted.ExpiresAtMs = fuzzNow.Add(-time.Hour).UnixMilli()
	f.Add(mustToken(f, inverted, nil))

	// No rights at all.
	norights := fuzzPayload()
	norights.Rights = 0
	f.Add(mustToken(f, norights, nil))

	// Unknown signing key.
	unknown := fuzzPayload()
	unknown.KeyId = "not-the-fuzz-key"
	f.Add(mustToken(f, unknown, nil))

	// Extreme timestamps.
	extreme := fuzzPayload()
	extreme.IssuedAtMs = -9223372036854775808
	extreme.ExpiresAtMs = 9223372036854775807
	f.Add(mustToken(f, extreme, nil))

	// Structurally empty and structurally junk.
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0x0a, 0x00, 0x12, 0x00})       // empty payload, empty signature
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff}) // not a protobuf at all

	f.Fuzz(func(t *testing.T, raw []byte) {
		var tok gapiv1.CapabilityToken
		if err := proto.Unmarshal(raw, &tok); err != nil {
			// Not a token on the wire; the transport rejects it before
			// the verifier ever sees it.
			return
		}

		payload, err := crypto.VerifyCapabilityToken(&tok, fuzzResolve, fuzzNow, crypto.RightSignalTerm)

		if (payload == nil) == (err == nil) {
			t.Fatalf("payload=%v err=%v: exactly one must be non-nil", payload, err)
		}

		if err != nil {
			if !errors.Is(err, crypto.ErrTokenMalformed) &&
				!errors.Is(err, crypto.ErrTokenUnknownKey) &&
				!errors.Is(err, crypto.ErrTokenSignature) &&
				!errors.Is(err, crypto.ErrTokenExpired) &&
				!errors.Is(err, crypto.ErrTokenRights) {
				t.Fatalf("untyped rejection %q: callers cannot branch on it", err)
			}
			// Determinism: the same bytes must fail the same way twice.
			if _, err2 := crypto.VerifyCapabilityToken(&tok, fuzzResolve, fuzzNow, crypto.RightSignalTerm); err2 == nil {
				t.Fatalf("non-deterministic verdict for %q: failed then succeeded", err)
			}
			return
		}

		if payload.KeyId != fuzzKeyID {
			t.Fatalf("accepted a token whose key id %q is not the only known key", payload.KeyId)
		}
		if !ed25519.Verify(fuzzPub, tok.Payload, tok.Signature) {
			t.Fatal("accepted a token whose signature does not verify over the literal payload bytes")
		}
		if payload.ExpiresAtMs <= payload.IssuedAtMs {
			t.Fatalf("accepted an inverted validity window: issued %d expires %d", payload.IssuedAtMs, payload.ExpiresAtMs)
		}
		if fuzzNow.UnixMilli() >= payload.ExpiresAtMs {
			t.Fatalf("accepted an expired token: now %d expires %d", fuzzNow.UnixMilli(), payload.ExpiresAtMs)
		}
		if payload.Rights&crypto.RightSignalTerm == 0 {
			t.Fatalf("accepted a token missing the required right: rights %#x", payload.Rights)
		}

		if _, err2 := crypto.VerifyCapabilityToken(&tok, fuzzResolve, fuzzNow, crypto.RightSignalTerm); err2 != nil {
			t.Fatalf("non-deterministic verdict: succeeded then failed: %v", err2)
		}
	})
}
