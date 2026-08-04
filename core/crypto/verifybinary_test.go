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
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/crypto"
)

// The round trip these tests cover is the one that was broken:
// 'gapictl crypto sign' produced signatures that VerifySignedBinary
// could never accept, because sign covered the canonical digest and
// verify covered the raw .b3 file bytes - which Magefile writes with a
// trailing newline. Production mode gates agent start on this, so the
// effect was that no signed agent could start (GAPI-DIV-032).
//
// These tests deliberately reproduce the SIDECAR FORMAT the build
// writes rather than round-tripping through one helper. A test that
// signs and verifies via a single function would have passed against
// the broken code.

// signLikeCLI reproduces what pkg/cli/security.go does: hash the file
// and sign the canonical digest string.
func signLikeCLI(t *testing.T, kp *crypto.KeyPair, path string) []byte {
	t.Helper()
	digest, err := crypto.HashFile(path)
	if err != nil {
		t.Fatalf("hashing %s: %v", path, err)
	}
	return kp.Sign([]byte(digest))
}

// writeSidecars reproduces what Magefile.go does: the .b3 carries the
// digest WITH a trailing newline, the .sig carries hex.
func writeSidecars(t *testing.T, path string, sig []byte) {
	t.Helper()
	digest, err := crypto.HashFile(path)
	if err != nil {
		t.Fatalf("hashing %s: %v", path, err)
	}
	if err := os.WriteFile(path+".b3", []byte(digest+"\n"), 0o600); err != nil {
		t.Fatalf("writing .b3: %v", err)
	}
	if err := os.WriteFile(path+".sig", []byte(hex.EncodeToString(sig)), 0o600); err != nil {
		t.Fatalf("writing .sig: %v", err)
	}
}

func fixture(t *testing.T) (string, *crypto.KeyPair) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.bin")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho agent\n"), 0o700); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	kp, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	return path, kp
}

// The headline case: sign the way the CLI does, write the sidecars the
// way the build does, and verify the way discovery does.
func TestSignedBinaryVerifiesAcrossTheRealPaths(t *testing.T) {
	path, kp := fixture(t)
	writeSidecars(t, path, signLikeCLI(t, kp, path))

	if err := crypto.VerifySignedBinary(path, kp.Public); err != nil {
		t.Fatalf("a signature made the way the CLI makes them did not verify: %v", err)
	}
}

// The trailing newline in the sidecar must not participate in the
// signed message. This is the exact byte that broke it.
func TestSidecarTrailingNewlineDoesNotAffectVerification(t *testing.T) {
	path, kp := fixture(t)
	sig := signLikeCLI(t, kp, path)

	digest, err := crypto.HashFile(path)
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if err := os.WriteFile(path+".sig", []byte(hex.EncodeToString(sig)), 0o600); err != nil {
		t.Fatalf("writing .sig: %v", err)
	}

	// Same signature, same binary, sidecar written both ways.
	for _, form := range []struct {
		name string
		body string
	}{
		{"with trailing newline", digest + "\n"},
		{"without trailing newline", digest},
	} {
		if err := os.WriteFile(path+".b3", []byte(form.body), 0o600); err != nil {
			t.Fatalf("writing .b3 %s: %v", form.name, err)
		}
		if err := crypto.VerifySignedBinary(path, kp.Public); err != nil {
			t.Errorf("%s: %v", form.name, err)
		}
	}
}

// A modified binary must fail, and fail on the digest rather than
// limping to the signature check.
func TestTamperedBinaryFailsDigest(t *testing.T) {
	path, kp := fixture(t)
	writeSidecars(t, path, signLikeCLI(t, kp, path))

	if err := os.WriteFile(path, []byte("#!/bin/sh\necho pwned\n"), 0o700); err != nil {
		t.Fatalf("tampering: %v", err)
	}

	err := crypto.VerifySignedBinary(path, kp.Public)
	if err == nil {
		t.Fatal("a modified binary verified")
	}
	if !strings.Contains(err.Error(), "digest mismatch") {
		t.Errorf("want a digest mismatch, got: %v", err)
	}
}

// A signature from the wrong key must fail even when the digest is
// consistent - otherwise the signature check is decoration.
func TestWrongKeyFails(t *testing.T) {
	path, kp := fixture(t)
	writeSidecars(t, path, signLikeCLI(t, kp, path))

	other, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generating second key: %v", err)
	}
	if err := crypto.VerifySignedBinary(path, other.Public); err == nil {
		t.Fatal("a signature verified against an unrelated public key")
	}
}

// A signature over the SIDECAR BYTES rather than the digest must be
// rejected. This pins the contract: had verify kept accepting those,
// the CLI and the verifier could silently diverge again.
func TestSignatureOverSidecarBytesIsRejected(t *testing.T) {
	path, kp := fixture(t)
	digest, err := crypto.HashFile(path)
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if err := os.WriteFile(path+".b3", []byte(digest+"\n"), 0o600); err != nil {
		t.Fatalf("writing .b3: %v", err)
	}
	// Sign the file bytes, newline included - the old, wrong message.
	wrong := kp.Sign([]byte(digest + "\n"))
	if err := os.WriteFile(path+".sig", []byte(hex.EncodeToString(wrong)), 0o600); err != nil {
		t.Fatalf("writing .sig: %v", err)
	}

	if err := crypto.VerifySignedBinary(path, kp.Public); err == nil {
		t.Fatal("a signature over the sidecar bytes was accepted; the contract is ambiguous again")
	}
}

func TestRejectsMalformedPublicKey(t *testing.T) {
	path, kp := fixture(t)
	writeSidecars(t, path, signLikeCLI(t, kp, path))

	if err := crypto.VerifySignedBinary(path, ed25519.PublicKey{1, 2, 3}); err == nil {
		t.Fatal("a 3-byte public key was accepted")
	}
}
