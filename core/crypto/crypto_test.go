// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package crypto

import (
	"bytes"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestED25519(t *testing.T) {
	tmpDir := t.TempDir()
	privPath := filepath.Join(tmpDir, "key.pem")
	pubPath := filepath.Join(tmpDir, "key.pub.hex")

	// 1. Generate Key
	kp, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	// Save keys
	if err := kp.SavePrivate(privPath); err != nil {
		t.Fatalf("SavePrivate failed: %v", err)
	}
	if err := kp.SavePublic(pubPath); err != nil {
		t.Fatalf("SavePublic failed: %v", err)
	}

	// Verify files exist
	if _, err := os.Stat(privPath); err != nil {
		t.Errorf("Private key not created")
	}
	if _, err := os.Stat(pubPath); err != nil {
		t.Errorf("Public key not created")
	}

	// 2. Sign
	data := []byte("hello world")
	sig := kp.Sign(data)

	sigPath := filepath.Join(tmpDir, "data.sig")
	if err := os.WriteFile(sigPath, []byte(hex.EncodeToString(sig)), 0644); err != nil {
		t.Fatalf("Failed to write signature: %v", err)
	}

	// 3. Verify
	// Load public key
	pubKey, err := LoadPublic(pubPath)
	if err != nil {
		t.Fatalf("LoadPublic failed: %v", err)
	}

	// Verify valid signature (raw bytes)
	valid := Verify(pubKey, data, sig)
	if !valid {
		t.Error("Verify returned false for valid signature")
	}

	// Verify invalid signature
	invalidValid := Verify(pubKey, data, []byte("deadbeef"))
	if invalidValid {
		t.Error("Verify returned true for invalid signature")
	}

	// Verify modified data
	modifiedData := []byte("hello world modified")
	modifiedValid := Verify(pubKey, modifiedData, sig)
	if modifiedValid {
		t.Error("Verify returned true for modified data")
	}
}

// TestSavePrivateIsOwnerOnly asserts the stored mode bits, which are
// the same whoever asks - so this deliberately does NOT skip under
// root. An earlier version did, borrowing a guard from a test that
// chmods 000 and attempts a read, where root genuinely bypasses the
// check. Here it only made the test vanish in a root CI container while
// still reporting green.
func TestSavePrivateIsOwnerOnly(t *testing.T) {
	kp, err := GenerateKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "id.pem")
	if err := kp.SavePrivate(path); err != nil {
		t.Fatalf("SavePrivate: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("private key mode is %#o; group and other bits must be clear", perm)
	}
}

// TestSavePrivateTightensAnExistingLooseFile covers regenerating over a
// path that already holds a world-readable file. O_CREATE's mode does
// nothing when the file exists, so without the explicit chmod the key
// bytes land in a 0644 file and are only protected afterwards - if at
// all.
// It asserts mode bits, so like the test above it does not skip under
// root.
func TestSavePrivateTightensAnExistingLooseFile(t *testing.T) {
	kp, err := GenerateKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "id.pem")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatalf("plant loose file: %v", err)
	}
	if err := kp.SavePrivate(path); err != nil {
		t.Fatalf("SavePrivate over an existing file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("private key mode is %#o after overwriting a loose file", perm)
	}
	// The stale bytes must be gone, not merely overwritten in place: the
	// key has to load back identically.
	loaded, err := LoadPrivate(path)
	if err != nil {
		t.Fatalf("LoadPrivate after overwrite: %v", err)
	}
	if !loaded.Private.Equal(kp.Private) {
		t.Fatal("key written over an existing file does not round-trip")
	}
}

// TestSavePrivateReplacesRatherThanWritesThrough is the gate for
// GAPI-DIV-045. It plants a loose 0644 file, holds an open descriptor on
// it, and saves over the path: if the save truncated and wrote through
// the existing file, the held descriptor would see the key bytes, and
// the key would have been world-readable for the length of the write. A
// mode assertion taken afterwards cannot see that window - it passes
// against the broken code - so the descriptor is the assertion.
//
// Like the mode tests above this does not skip under root: it reads
// content through a descriptor it already owns, which root does not
// change.
func TestSavePrivateReplacesRatherThanWritesThrough(t *testing.T) {
	kp, err := GenerateKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "id.pem")
	const stale = "stale placeholder contents\n"
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatalf("plant loose file: %v", err)
	}
	held, err := os.Open(path)
	if err != nil {
		t.Fatalf("hold open: %v", err)
	}
	defer func() {
		if err := held.Close(); err != nil {
			t.Errorf("close held descriptor: %v", err)
		}
	}()

	if err := kp.SavePrivate(path); err != nil {
		t.Fatalf("SavePrivate over an existing file: %v", err)
	}

	got, err := io.ReadAll(held)
	if err != nil {
		t.Fatalf("read held descriptor: %v", err)
	}
	if string(got) != stale {
		t.Fatalf("the 0644 file the key replaced was written through, not replaced: held descriptor reads %q, want %q", got, stale)
	}
	assertOwnerOnly(t, path)
	loaded, err := LoadPrivate(path)
	if err != nil {
		t.Fatalf("LoadPrivate after replace: %v", err)
	}
	if !loaded.Private.Equal(kp.Private) {
		t.Fatal("key written over an existing file does not round-trip")
	}
}

// TestWriteAgeIdentityReplacesRatherThanWritesThrough is the same gate
// for the age identity, which is equally a secret. os.WriteFile's perm
// argument applied only on create and there was no compensating chmod at
// all, so the overwrite case was strictly worse here.
func TestWriteAgeIdentityReplacesRatherThanWritesThrough(t *testing.T) {
	id, err := GenerateAgeIdentity()
	if err != nil {
		t.Fatalf("GenerateAgeIdentity: %v", err)
	}
	path := filepath.Join(t.TempDir(), "age.key")
	const stale = "stale placeholder contents\n"
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatalf("plant loose file: %v", err)
	}
	held, err := os.Open(path)
	if err != nil {
		t.Fatalf("hold open: %v", err)
	}
	defer func() {
		if err := held.Close(); err != nil {
			t.Errorf("close held descriptor: %v", err)
		}
	}()

	if err := WriteAgeIdentity(path, id); err != nil {
		t.Fatalf("WriteAgeIdentity over an existing file: %v", err)
	}

	got, err := io.ReadAll(held)
	if err != nil {
		t.Fatalf("read held descriptor: %v", err)
	}
	if string(got) != stale {
		t.Fatalf("the 0644 file the identity replaced was written through, not replaced: held descriptor reads %q, want %q", got, stale)
	}
	assertOwnerOnly(t, path)
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read identity: %v", err)
	}
	if !strings.Contains(string(written), id.String()) {
		t.Fatal("identity written over an existing file does not contain the private key")
	}
}

// assertOwnerOnly fails unless path is readable by its owner alone.
func assertOwnerOnly(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("mode is %#o, want %#o", perm, 0o600)
	}
}

func TestAGE(t *testing.T) {
	// 1. Generate Identity
	id, err := GenerateAgeIdentity()
	if err != nil {
		t.Fatalf("GenerateAgeIdentity failed: %v", err)
	}
	if id == nil {
		t.Fatal("Identity is nil")
	}

	// Write identity to file to test WriteAgeIdentity
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "age.key")
	if err := WriteAgeIdentity(idPath, id); err != nil {
		t.Fatalf("WriteAgeIdentity failed: %v", err)
	}

	// Extract public key recipient
	content, _ := os.ReadFile(idPath)
	lines := strings.Split(string(content), "\n")
	var recipient string
	for _, line := range lines {
		if strings.Contains(line, "public key: ") {
			parts := strings.Split(line, ": ")
			if len(parts) > 1 {
				recipient = strings.TrimSpace(parts[1])
			}
		}
	}
	if recipient == "" {
		t.Fatal("Could not extract public key recipient from generated file")
	}

	// 2. Encrypt
	secret := []byte("my secret data")
	encrypted, err := EncryptAge([]string{recipient}, secret)
	if err != nil {
		t.Fatalf("EncryptAge failed: %v", err)
	}

	// 3. Decrypt
	decrypted, err := DecryptAge(idPath, encrypted)
	if err != nil {
		t.Fatalf("DecryptAge failed: %v", err)
	}

	if !bytes.Equal(decrypted, secret) {
		t.Errorf("Decrypted data mismatch. Got %s, want %s", decrypted, secret)
	}
}

func TestHashing(t *testing.T) {
	// 1. Compute Hash
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	hash, err := HashFile(filePath)
	if err != nil {
		t.Fatalf("HashFile failed: %v", err)
	}
	if len(hash) == 0 {
		t.Error("Hash is empty")
	}

	// Verify determinism
	hash2, _ := HashFile(filePath)
	if hash != hash2 {
		t.Error("Hash is not deterministic")
	}
}
