package toposort

import (
	"errors"
	"testing"
)

func indexOf(order []string, id string) int {
	for i, v := range order {
		if v == id {
			return i
		}
	}
	return -1
}

// requireBefore fails unless a precedes b in order.
func requireBefore(t *testing.T, order []string, a, b string) {
	t.Helper()
	ia, ib := indexOf(order, a), indexOf(order, b)
	if ia == -1 || ib == -1 {
		t.Fatalf("order %v missing %s or %s", order, a, b)
	}
	if ia >= ib {
		t.Fatalf("order %v: want %s before %s", order, a, b)
	}
}

func TestSort_HardChainAndDiamond(t *testing.T) {
	hard := map[string][]string{
		"db":     nil,
		"cache":  {"db"},
		"api":    {"db", "cache"},
		"worker": {"db"},
	}
	order, err := Sort(hard, nil)
	if err != nil {
		t.Fatalf("Sort: %v", err)
	}
	if len(order) != 4 {
		t.Fatalf("order %v, want 4 nodes", order)
	}
	requireBefore(t, order, "db", "cache")
	requireBefore(t, order, "cache", "api")
	requireBefore(t, order, "db", "worker")
}

func TestSort_SoftEdgeOrders(t *testing.T) {
	hard := map[string][]string{"a": nil, "b": nil}
	soft := map[string][]string{"b": {"a"}} // b wants a -> a first
	order, err := Sort(hard, soft)
	if err != nil {
		t.Fatalf("Sort: %v", err)
	}
	requireBefore(t, order, "a", "b")
}

func TestSort_SoftCycleNeverBlocks(t *testing.T) {
	hard := map[string][]string{"a": nil, "b": nil}
	soft := map[string][]string{"a": {"b"}, "b": {"a"}}
	order, err := Sort(hard, soft)
	if err != nil {
		t.Fatalf("soft cycle must not error, got %v", err)
	}
	if len(order) != 2 {
		t.Fatalf("order %v, want both nodes emitted", order)
	}
}

func TestSort_MixedCycleDropsSoftEdge(t *testing.T) {
	// a hard-requires b; b soft-wants a. The soft edge is the one dropped:
	// hard ordering (b before a) must hold.
	hard := map[string][]string{"a": {"b"}, "b": nil}
	soft := map[string][]string{"b": {"a"}}
	order, err := Sort(hard, soft)
	if err != nil {
		t.Fatalf("mixed cycle must not error, got %v", err)
	}
	requireBefore(t, order, "b", "a")
}

func TestSort_HardCycleErrors(t *testing.T) {
	hard := map[string][]string{"a": {"b"}, "b": {"a"}}
	_, err := Sort(hard, nil)
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("hard cycle: err = %v, want ErrCycle", err)
	}
}

func TestSort_UnknownSoftDepIgnored(t *testing.T) {
	hard := map[string][]string{"a": nil}
	soft := map[string][]string{"a": {"ghost"}}
	order, err := Sort(hard, soft)
	if err != nil {
		t.Fatalf("unknown soft dep must be ignored, got %v", err)
	}
	if len(order) != 1 || order[0] != "a" {
		t.Fatalf("order %v, want [a]", order)
	}
}

func TestSort_UnknownHardDepIgnored(t *testing.T) {
	// Matches the existing agentreg semantics: hard deps naming agents
	// outside the set are skipped, not errors (external services).
	hard := map[string][]string{"a": {"external-service"}}
	order, err := Sort(hard, nil)
	if err != nil {
		t.Fatalf("unknown hard dep must be ignored, got %v", err)
	}
	if len(order) != 1 || order[0] != "a" {
		t.Fatalf("order %v, want [a]", order)
	}
}

func TestSort_Deterministic(t *testing.T) {
	hard := map[string][]string{"c": nil, "a": nil, "b": nil}
	first, err := Sort(hard, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		again, err := Sort(hard, nil)
		if err != nil {
			t.Fatal(err)
		}
		for j := range first {
			if first[j] != again[j] {
				t.Fatalf("non-deterministic order: %v vs %v", first, again)
			}
		}
	}
}
