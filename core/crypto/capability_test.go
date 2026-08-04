// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package crypto_test

import (
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/crypto"
	gapiv1 "github.com/goppydae/gapi/pkg/proto"
	"google.golang.org/protobuf/proto"
)

func testKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return pub, priv
}

func testResolver(keyID string, pub ed25519.PublicKey) crypto.KeyResolver {
	return func(id string) (ed25519.PublicKey, bool) {
		if id == keyID {
			return pub, true
		}
		return nil, false
	}
}

func mintToken(t *testing.T, priv ed25519.PrivateKey, mutate func(*gapiv1.CapabilityTokenPayload)) *gapiv1.CapabilityToken {
	t.Helper()
	payload := &gapiv1.CapabilityTokenPayload{
		TokenId:      []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SubjectUuid:  []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		Rights:       crypto.RightSignalTerm,
		IssuedAtMs:   time.Unix(1000, 0).UnixMilli(),
		ExpiresAtMs:  time.Unix(1120, 0).UnixMilli(),
		IssuerNodeId: "node-1",
		KeyId:        "issuer-key-1",
	}
	if mutate != nil {
		mutate(payload)
	}
	tok, err := crypto.SignCapabilityToken(payload, priv)
	if err != nil {
		t.Fatalf("SignCapabilityToken: %v", err)
	}
	return tok
}

func TestVerifyCapabilityToken_Valid(t *testing.T) {
	pub, priv := testKeypair(t)
	tok := mintToken(t, priv, nil)

	payload, err := crypto.VerifyCapabilityToken(tok, testResolver("issuer-key-1", pub), time.Unix(1050, 0), crypto.RightSignalTerm)
	if err != nil {
		t.Fatalf("VerifyCapabilityToken: %v", err)
	}
	if payload.IssuerNodeId != "node-1" {
		t.Errorf("payload issuer = %q, want node-1", payload.IssuerNodeId)
	}
}

func TestVerifyCapabilityToken_Expired(t *testing.T) {
	pub, priv := testKeypair(t)
	tok := mintToken(t, priv, nil)

	_, err := crypto.VerifyCapabilityToken(tok, testResolver("issuer-key-1", pub), time.Unix(1121, 0), crypto.RightSignalTerm)
	if !errors.Is(err, crypto.ErrTokenExpired) {
		t.Fatalf("expired token error = %v, want ErrTokenExpired", err)
	}
}

func TestVerifyCapabilityToken_BadSignature(t *testing.T) {
	pub, priv := testKeypair(t)
	tok := mintToken(t, priv, nil)
	// Tamper with the payload after signing: rights escalation attempt.
	var payload gapiv1.CapabilityTokenPayload
	if err := proto.Unmarshal(tok.Payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	payload.Rights = crypto.RightSignalTerm | crypto.RightSignalKill
	tampered, err := proto.Marshal(&payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tok.Payload = tampered

	_, err = crypto.VerifyCapabilityToken(tok, testResolver("issuer-key-1", pub), time.Unix(1050, 0), crypto.RightSignalTerm)
	if !errors.Is(err, crypto.ErrTokenSignature) {
		t.Fatalf("tampered token error = %v, want ErrTokenSignature", err)
	}
}

func TestVerifyCapabilityToken_InsufficientRights(t *testing.T) {
	pub, priv := testKeypair(t)
	tok := mintToken(t, priv, nil) // grants TERM only

	_, err := crypto.VerifyCapabilityToken(tok, testResolver("issuer-key-1", pub), time.Unix(1050, 0), crypto.RightSignalKill)
	if !errors.Is(err, crypto.ErrTokenRights) {
		t.Fatalf("under-privileged token error = %v, want ErrTokenRights", err)
	}
}

func TestVerifyCapabilityToken_UnknownKey(t *testing.T) {
	pub, priv := testKeypair(t)
	tok := mintToken(t, priv, func(p *gapiv1.CapabilityTokenPayload) {
		p.KeyId = "who-is-this"
	})

	_, err := crypto.VerifyCapabilityToken(tok, testResolver("issuer-key-1", pub), time.Unix(1050, 0), crypto.RightSignalTerm)
	if !errors.Is(err, crypto.ErrTokenUnknownKey) {
		t.Fatalf("unknown-key token error = %v, want ErrTokenUnknownKey", err)
	}
}

func TestVerifyCapabilityToken_Malformed(t *testing.T) {
	pub, _ := testKeypair(t)
	cases := map[string]*gapiv1.CapabilityToken{
		"nil token":     nil,
		"empty":         {},
		"garbage bytes": {Payload: []byte("not-proto"), Signature: make([]byte, 64)},
		"short sig":     {Payload: []byte{}, Signature: []byte{1, 2}},
	}
	for name, tok := range cases {
		if _, err := crypto.VerifyCapabilityToken(tok, testResolver("issuer-key-1", pub), time.Unix(1050, 0), 0); !errors.Is(err, crypto.ErrTokenMalformed) {
			t.Errorf("%s: error = %v, want ErrTokenMalformed", name, err)
		}
	}
}

func TestVerifyCapabilityToken_InvertedValidity(t *testing.T) {
	pub, priv := testKeypair(t)
	tok := mintToken(t, priv, func(p *gapiv1.CapabilityTokenPayload) {
		p.ExpiresAtMs = p.IssuedAtMs - 1 // expires before issued
	})
	_, err := crypto.VerifyCapabilityToken(tok, testResolver("issuer-key-1", pub), time.Unix(999, 0), crypto.RightSignalTerm)
	if !errors.Is(err, crypto.ErrTokenMalformed) {
		t.Fatalf("inverted-validity token error = %v, want ErrTokenMalformed", err)
	}
}

func TestRightForSignal(t *testing.T) {
	cases := []struct {
		signum int32
		want   uint64
		ok     bool
	}{
		{15, crypto.RightSignalTerm, true}, // SIGTERM
		{2, crypto.RightSignalTerm, true},  // SIGINT
		{9, crypto.RightSignalKill, true},  // SIGKILL
		{10, crypto.RightSignalUser, true}, // SIGUSR1
		{12, crypto.RightSignalUser, true}, // SIGUSR2
		{11, 0, false},                     // SIGSEGV: never grantable
		{0, 0, false},
		{-1, 0, false},
	}
	for _, c := range cases {
		got, err := crypto.RightForSignal(c.signum)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("RightForSignal(%d) = %#x, %v; want %#x", c.signum, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("RightForSignal(%d) accepted an ungrantable signal", c.signum)
		}
	}
}
