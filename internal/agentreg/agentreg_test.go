package agentreg

import (
	"fmt"
	"sync"
	"testing"

	"github.com/goppydae/gapi/core/store"
)

func newTestRegistry(t *testing.T) *AgentRegistry {
	t.Helper()
	raw, err := store.Open(store.Hybrid)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	hs, ok := raw.(store.HybridStore)
	if !ok {
		t.Fatalf("store is not a HybridStore")
	}
	r, err := NewAgentRegistry(hs, nil)
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func indexOf(order []string, id string) int {
	for i, v := range order {
		if v == id {
			return i
		}
	}
	return -1
}

// #5: TopologicalSort must place hard dependencies before their dependents, even
// when multiple chains share a node.
func TestTopologicalSort_Order(t *testing.T) {
	r := newTestRegistry(t)

	// db -> cache -> api, and db -> worker (db shared by cache and worker)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	must(r.Register(&AgentDescription{ID: "db", Type: "service"}))
	must(r.Register(&AgentDescription{ID: "cache", Type: "service", Requires: []string{"db"}}))
	must(r.Register(&AgentDescription{ID: "api", Type: "service", Requires: []string{"cache"}}))
	must(r.Register(&AgentDescription{ID: "worker", Type: "service", Requires: []string{"db"}}))

	order, err := r.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(order) != 4 {
		t.Fatalf("expected 4 agents in order, got %v", order)
	}
	if indexOf(order, "db") > indexOf(order, "cache") {
		t.Errorf("db must come before cache: %v", order)
	}
	if indexOf(order, "cache") > indexOf(order, "api") {
		t.Errorf("cache must come before api: %v", order)
	}
	if indexOf(order, "db") > indexOf(order, "worker") {
		t.Errorf("db must come before worker: %v", order)
	}
}

// #2: after TopologicalSort calls syncGraph, dependency queries by kind must
// still return edges (previously syncGraph rewrote them as "dependency" and
// GetDependencies found nothing).
func TestGetDependencies_SurvivesSync(t *testing.T) {
	r := newTestRegistry(t)

	if err := r.Register(&AgentDescription{ID: "db", Type: "service"}); err != nil {
		t.Fatalf("register db: %v", err)
	}
	if err := r.Register(&AgentDescription{
		ID: "api", Type: "service", Requires: []string{"db"}, Wants: []string{"cache"},
	}); err != nil {
		t.Fatalf("register api: %v", err)
	}

	// Trigger syncGraph.
	if _, err := r.TopologicalSort(); err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}

	deps, err := r.GetDependencies("api")
	if err != nil {
		t.Fatalf("GetDependencies: %v", err)
	}
	found := map[string]bool{}
	for _, d := range deps {
		found[d] = true
	}
	if !found["db"] {
		t.Errorf("expected hard dep 'db' in %v after sync", deps)
	}
	if !found["cache"] {
		t.Errorf("expected soft dep 'cache' in %v after sync", deps)
	}
}

// #8: concurrent Register calls must be serialized by nodeMu; the -race run
// is the real assertion, the List check is the functional one.
func TestRegister_ConcurrentRegistrations(t *testing.T) {
	r := newTestRegistry(t)

	const n = 16
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("agent-%02d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- r.Register(&AgentDescription{
				ID:       id,
				Type:     "service",
				Requires: []string{"agent-00"},
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Register: %v", err)
		}
	}

	agents, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != n {
		t.Fatalf("registered %d agents, want %d", len(agents), n)
	}
}
