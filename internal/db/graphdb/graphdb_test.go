package graphdb

import (
	"fmt"
	"sync"
	"testing"
)

func newTestGraph(t *testing.T) *Graph {
	t.Helper()
	g, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

// #10: exported operations rely on bbolt transactions uniformly; these are
// the package's first tests - round-trip, kind filtering, pathfinding, and
// a concurrent read/write pass under -race.
func TestGraph_AddAndNeighbors(t *testing.T) {
	g := newTestGraph(t)

	for _, id := range []string{"a", "b", "c"} {
		if err := g.AddNode(Node{ID: id, Type: "agent"}); err != nil {
			t.Fatalf("AddNode(%s): %v", id, err)
		}
	}
	if err := g.AddEdge(Edge{From: "a", To: "b", Kind: "requires", Weight: 1}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}
	if err := g.AddEdge(Edge{From: "a", To: "c", Kind: "wants", Weight: 1}); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	requires, err := g.Neighbors("a", "requires")
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(requires) != 1 || requires[0].To != "b" {
		t.Fatalf("Neighbors(a, requires) = %v, want [a->b]", requires)
	}

	wants, err := g.Neighbors("a", "wants")
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(wants) != 1 || wants[0].To != "c" {
		t.Fatalf("Neighbors(a, wants) = %v, want [a->c]", wants)
	}

	none, err := g.Neighbors("b", "requires")
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("Neighbors(b, requires) = %v, want empty", none)
	}
}

func TestGraph_ShortestPath(t *testing.T) {
	g := newTestGraph(t)

	for _, id := range []string{"a", "b", "c", "d"} {
		if err := g.AddNode(Node{ID: id, Type: "agent"}); err != nil {
			t.Fatal(err)
		}
	}
	// a->b->d (cost 2) beats a->c->d (cost 5).
	edges := []Edge{
		{From: "a", To: "b", Kind: "requires", Weight: 1},
		{From: "b", To: "d", Kind: "requires", Weight: 1},
		{From: "a", To: "c", Kind: "requires", Weight: 2},
		{From: "c", To: "d", Kind: "requires", Weight: 3},
	}
	for _, e := range edges {
		if err := g.AddEdge(e); err != nil {
			t.Fatal(err)
		}
	}

	nodes, cost, err := g.ShortestPath("a", "d", "requires", 60)
	if err != nil {
		t.Fatalf("ShortestPath: %v", err)
	}
	if cost != 2 || len(nodes) != 3 || nodes[0] != "a" || nodes[1] != "b" || nodes[2] != "d" {
		t.Fatalf("ShortestPath = %v cost %d, want [a b d] cost 2", nodes, cost)
	}

	if _, _, err := g.ShortestPath("d", "a", "requires", 60); err == nil {
		t.Fatal("ShortestPath(d, a) should fail: no path")
	}
}

func TestGraph_ConcurrentReadWrite(t *testing.T) {
	g := newTestGraph(t)

	if err := g.AddNode(Node{ID: "hub", Type: "agent"}); err != nil {
		t.Fatal(err)
	}

	const n = 12
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("n%02d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := g.AddNode(Node{ID: id, Type: "agent"}); err != nil {
				t.Errorf("AddNode(%s): %v", id, err)
				return
			}
			if err := g.AddEdge(Edge{From: "hub", To: id, Kind: "requires", Weight: 1}); err != nil {
				t.Errorf("AddEdge(hub->%s): %v", id, err)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := g.Neighbors("hub", "requires"); err != nil {
				t.Errorf("Neighbors: %v", err)
			}
		}()
	}
	wg.Wait()

	final, err := g.Neighbors("hub", "requires")
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(final) != n {
		t.Fatalf("hub has %d requires edges, want %d", len(final), n)
	}
}
