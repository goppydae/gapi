package agentmgr

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/crypto"
)

// writeExecutable creates a fake agent binary with the given permissions.
func writeExecutable(t *testing.T, dir, name string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), perm); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	// WriteFile is umask-filtered; force the exact permissions under test.
	if err := os.Chmod(path, perm); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
	return path
}

// signBinary writes the .b3 hash file and a .sig over its bytes, matching
// the real signing convention: a hex ed25519 signature over the canonical
// BLAKE3 digest, with the .b3 sidecar written the way the build writes it.
func signBinary(t *testing.T, kp *crypto.KeyPair, binPath string) {
	t.Helper()
	hash, err := crypto.HashFile(binPath)
	if err != nil {
		t.Fatalf("hash %s: %v", binPath, err)
	}

	// The sidecar carries a trailing newline, because that is what
	// Magefile.go writes.
	if err := os.WriteFile(binPath+".b3", []byte(hash+"\n"), 0o644); err != nil {
		t.Fatalf("write .b3: %v", err)
	}

	// The signature covers the CANONICAL digest - no newline - because
	// that is what 'gapictl crypto sign' covers (pkg/cli/security.go).
	//
	// This helper previously signed the sidecar bytes INCLUDING the
	// newline, which is what the verifier happened to check. That made
	// this test mirror the implementation rather than the contract: it
	// stayed green while no signature the real CLI produced could ever
	// verify, and production mode could not start any signed agent
	// (GAPI-DIV-032). Signing what the CLI signs is the whole point of
	// the helper.
	sig := kp.Sign([]byte(hash))
	if err := os.WriteFile(binPath+".sig", []byte(hex.EncodeToString(sig)), 0o644); err != nil {
		t.Fatalf("write .sig: %v", err)
	}
}

func devManager() *AgentManager {
	return NewAgentManager(nil, nil, "", false, nil)
}

func TestSafeToExecute_RejectsWorldWritableFile(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "agent", 0o757)

	err := devManager().safeToExecute(bin)
	if err == nil || !strings.Contains(err.Error(), "world-writable file") {
		t.Fatalf("err = %v, want world-writable file rejection", err)
	}
}

func TestSafeToExecute_RejectsWorldWritableDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "agents")
	if err := os.Mkdir(sub, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o777); err != nil {
		t.Fatal(err)
	}
	bin := writeExecutable(t, sub, "agent", 0o755)

	err := devManager().safeToExecute(bin)
	if err == nil || !strings.Contains(err.Error(), "world-writable directory") {
		t.Fatalf("err = %v, want world-writable directory rejection", err)
	}
}

func TestSafeToExecute_AcceptsCleanBinaryInDevMode(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "agent", 0o755)

	if err := devManager().safeToExecute(bin); err != nil {
		t.Fatalf("dev-mode clean binary rejected: %v", err)
	}
}

func TestSafeToExecute_ProductionRequiresKey(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "agent", 0o755)

	am := NewAgentManager(nil, nil, "", true, nil)
	err := am.safeToExecute(bin)
	if err == nil || !strings.Contains(err.Error(), "verification public key") {
		t.Fatalf("err = %v, want missing-key rejection", err)
	}
}

func TestSafeToExecute_ProductionRejectsUnsigned(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "agent", 0o755)

	kp, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	am := NewAgentManager(nil, nil, "", true, kp.Public)

	if err := am.safeToExecute(bin); err == nil {
		t.Fatal("unsigned binary accepted in production mode")
	}
}

func TestSafeToExecute_ProductionAcceptsValidSignature(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "agent", 0o755)

	kp, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signBinary(t, kp, bin)
	am := NewAgentManager(nil, nil, "", true, kp.Public)

	if err := am.safeToExecute(bin); err != nil {
		t.Fatalf("validly signed binary rejected: %v", err)
	}
}

func TestSafeToExecute_ProductionRejectsTamperedBinary(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "agent", 0o755)

	kp, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signBinary(t, kp, bin)
	// Tamper after signing.
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho evil\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	am := NewAgentManager(nil, nil, "", true, kp.Public)

	if err := am.safeToExecute(bin); err == nil {
		t.Fatal("tampered binary accepted in production mode")
	}
}

func TestSafeToExecute_ProductionRejectsWrongKey(t *testing.T) {
	dir := t.TempDir()
	bin := writeExecutable(t, dir, "agent", 0o755)

	signer, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signBinary(t, signer, bin)

	other, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	am := NewAgentManager(nil, nil, "", true, other.Public)

	if err := am.safeToExecute(bin); err == nil {
		t.Fatal("binary signed by a different key accepted in production mode")
	}
}
