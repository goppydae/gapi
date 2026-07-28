package ident_test

import (
	"testing"

	"github.com/goppydae/gapi/internal/ident"
)

func TestNewV7_IsValidVersion7(t *testing.T) {
	b := ident.NewV7()
	if len(b) != 16 {
		t.Fatalf("NewV7 length = %d, want 16", len(b))
	}
	if version := b[6] >> 4; version != 7 {
		t.Fatalf("UUID version = %d, want 7", version)
	}
}

func TestNewV7String_CanonicalAndOrdered(t *testing.T) {
	prev := ident.NewV7String()
	if len(prev) != 36 {
		t.Fatalf("canonical form length = %d, want 36", len(prev))
	}
	// UUIDv7 is time-ordered: string form sorts by mint order too
	// (the property that makes ids usable as trace foreign keys).
	for i := 0; i < 100; i++ {
		next := ident.NewV7String()
		if next <= prev {
			t.Fatalf("UUIDv7 order regressed at iteration %d: %s <= %s", i, next, prev)
		}
		prev = next
	}
}
