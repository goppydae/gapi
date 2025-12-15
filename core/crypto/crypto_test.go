package crypto

import (
	"bytes"
	"encoding/hex"
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
