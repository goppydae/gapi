package crypto

import (
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

// #29: SavePrivate must write PKCS#8 ("PRIVATE KEY") and its output must be
// loadable by the shared PEMNS parser (and vice versa), so the two persistence
// paths are interchangeable.
func TestED25519_PKCS8Interop(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "key.pem")

	kp, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := kp.SavePrivate(p); err != nil {
		t.Fatalf("SavePrivate: %v", err)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatalf("expected PKCS#8 \"PRIVATE KEY\" block, got %v", block)
	}

	// PEMNS must be able to parse what SavePrivate wrote.
	if _, err := PEM.ParsePrivateKey(data); err != nil {
		t.Fatalf("PEMNS.ParsePrivateKey cannot read SavePrivate output: %v", err)
	}

	// LoadPrivate round-trips.
	loaded, err := LoadPrivate(p)
	if err != nil {
		t.Fatalf("LoadPrivate: %v", err)
	}
	if !loaded.Private.Equal(kp.Private) {
		t.Fatal("round-tripped private key does not match original")
	}

	// Reverse direction: a key encoded by PEMNS is loadable by LoadPrivate.
	enc, err := PEM.EncodePrivateKey(kp.Private)
	if err != nil {
		t.Fatalf("PEMNS.EncodePrivateKey: %v", err)
	}
	p2 := filepath.Join(dir, "pemns.pem")
	if err := os.WriteFile(p2, enc, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded2, err := LoadPrivate(p2)
	if err != nil {
		t.Fatalf("LoadPrivate cannot read PEMNS output: %v", err)
	}
	if !loaded2.Private.Equal(kp.Private) {
		t.Fatal("PEMNS-encoded key mismatch after LoadPrivate")
	}
}

// #29: legacy raw "ED25519 PRIVATE KEY" files must still load for backward
// compatibility.
func TestLoadPrivate_LegacyRawFormat(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "legacy.pem")

	kp, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	legacy := pem.EncodeToMemory(&pem.Block{Type: "ED25519 PRIVATE KEY", Bytes: kp.Private})
	if err := os.WriteFile(p, legacy, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	loaded, err := LoadPrivate(p)
	if err != nil {
		t.Fatalf("LoadPrivate legacy: %v", err)
	}
	if !loaded.Private.Equal(kp.Private) {
		t.Fatal("legacy-loaded private key does not match original")
	}
}
