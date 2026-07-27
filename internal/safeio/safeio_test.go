package safeio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_EmptyRejected(t *testing.T) {
	if _, err := Resolve(""); err == nil {
		t.Fatal("Resolve(\"\") should fail")
	}
}

func TestReadFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("payload"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("ReadFile = %q, want %q", got, "payload")
	}
}

func TestCreate_ThenOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	f, err := Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.WriteString("x"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	g, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestReadFileUnder_ConfinedOK(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b.txt")
	if err := os.MkdirAll(filepath.Dir(sub), 0750); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(sub, []byte("ok"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := ReadFileUnder(root, sub)
	if err != nil {
		t.Fatalf("ReadFileUnder: %v", err)
	}
	if string(got) != "ok" {
		t.Errorf("ReadFileUnder = %q, want %q", got, "ok")
	}
}

func TestResolveUnder_EscapeRejected(t *testing.T) {
	root := t.TempDir()

	cases := []string{
		filepath.Join(root, "..", "escape.txt"),
		filepath.Join(root, "a", "..", "..", "escape.txt"),
		"/etc/passwd",
	}
	for _, path := range cases {
		if _, err := ResolveUnder(root, path); err == nil {
			t.Errorf("ResolveUnder(%q, %q) should fail", root, path)
		} else if !strings.Contains(err.Error(), "escapes root") {
			t.Errorf("ResolveUnder(%q, %q) error = %v, want escape error", root, path, err)
		}
	}
}

func TestResolveUnder_SiblingPrefixRejected(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "a")
	sibling := filepath.Join(base, "ab", "f.txt")
	if err := os.MkdirAll(root, 0750); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// "ab" shares the string prefix "a" but is not under root "a".
	if _, err := ResolveUnder(root, sibling); err == nil {
		t.Errorf("ResolveUnder(%q, %q) should fail", root, sibling)
	}
}

func TestResolveUnder_RootItselfAllowed(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveUnder(root, root); err != nil {
		t.Errorf("ResolveUnder(root, root) = %v, want nil", err)
	}
}
