package graphdb

import (
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

type Node struct {
	ID     string            `json:"id"`
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels"`
}

type Edge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Weight int    `json:"weight"`
}

type Path struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Kind      string   `json:"kind"`
	Nodes     []string `json:"nodes"`
	Cost      int      `json:"cost"`
	Timestamp int64    `json:"timestamp"`
	TTL       int64    `json:"ttl"`
}

type Graph struct {
	db       *bbolt.DB
	mutex    sync.Mutex
	handlers map[string]func(map[string]string)
}

func NewGraph(path string) (*Graph, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}

	g := &Graph{
		db:       db,
		handlers: make(map[string]func(map[string]string)),
	}

	err = g.db.Update(func(tx *bbolt.Tx) error {
		for _, name := range []string{"nodes", "edges", "paths"} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})

	return g, err
}

// Close cleanly shuts down the underlying DB
func (g *Graph) Close() error {
	return g.db.Close()
}

// UpsertAgent stores an agent node into the graph with type "agent"
func (g *Graph) UpsertAgent(scope, id string, props map[string]string) error {
	node := Node{
		ID:     fmt.Sprintf("%s/%s", scope, id),
		Type:   "agent",
		Labels: props,
	}
	return g.AddNode(node)
}

func (g *Graph) AddNode(n Node) error {
	return g.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("nodes"))
		data, err := json.Marshal(n)
		if err != nil {
			return err
		}
		return b.Put([]byte(n.ID), data)
	})
}

func (g *Graph) AddEdge(e Edge) error {
	return g.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("edges"))
		key := fmt.Sprintf("%s|%s|%s", e.From, e.Kind, e.To)
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		return b.Put([]byte(key), data)
	})
}

func (g *Graph) Neighbors(id string, kind string) ([]Edge, error) {
	var result []Edge
	err := g.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("edges"))
		cursor := b.Cursor()
		prefix := []byte(fmt.Sprintf("%s|%s|", id, kind))
		for k, v := cursor.Seek(prefix); k != nil && string(k[:len(prefix)]) == string(prefix); k, v = cursor.Next() {
			var edge Edge
			if err := json.Unmarshal(v, &edge); err != nil {
				return err
			}
			result = append(result, edge)
		}
		return nil
	})
	return result, err
}

func (g *Graph) RegisterEventHandler(eventType string, handler func(map[string]string)) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.handlers[eventType] = handler
}

func (g *Graph) EmitEvent(eventType string, data map[string]string) {
	g.mutex.Lock()
	handler, exists := g.handlers[eventType]
	g.mutex.Unlock()
	if exists {
		go handler(data)
	}
}

func (g *Graph) ShortestPath(start, end, kind string, ttl int64) ([]string, int, error) {
	type Item struct {
		Node     string
		Cost     int
		Path     []string
		Priority int
		Index    int
	}

	pq := make(PriorityQueue, 0)
	heap.Init(&pq)
	heap.Push(&pq, &Item{Node: start, Cost: 0, Path: []string{start}, Priority: 0})

	visited := map[string]bool{}

	for pq.Len() > 0 {
		current := heap.Pop(&pq).(*Item)
		if visited[current.Node] {
			continue
		}
		visited[current.Node] = true

		if current.Node == end {
			_ = g.db.Update(func(tx *bbolt.Tx) error {
				p := Path{
					From:      start,
					To:        end,
					Kind:      kind,
					Nodes:     current.Path,
					Cost:      current.Cost,
					Timestamp: time.Now().Unix(),
					TTL:       ttl,
				}
				data, err := json.Marshal(p)
				if err != nil {
					return err
				}
				b := tx.Bucket([]byte("paths"))
				key := fmt.Sprintf("%s|%s|%s", start, kind, end)
				return b.Put([]byte(key), data)
			})
			return current.Path, current.Cost, nil
		}

		neighbors, err := g.Neighbors(current.Node, kind)
		if err != nil {
			return nil, 0, err
		}

		for _, edge := range neighbors {
			if !visited[edge.To] {
				newCost := current.Cost + edge.Weight
				newPath := append([]string{}, current.Path...)
				newPath = append(newPath, edge.To)
				heap.Push(&pq, &Item{Node: edge.To, Cost: newCost, Path: newPath, Priority: newCost})
			}
		}
	}
	return nil, 0, errors.New("no path found")
}

func (g *Graph) GetStoredPath(start, end, kind string) (*Path, error) {
	var path Path
	err := g.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("paths"))
		key := fmt.Sprintf("%s|%s|%s", start, kind, end)
		data := b.Get([]byte(key))
		if data == nil {
			return errors.New("no stored path")
		}
		if err := json.Unmarshal(data, &path); err != nil {
			return err
		}
		now := time.Now().Unix()
		if now-path.Timestamp > path.TTL {
			return errors.New("path expired")
		}
		return nil
	})
	return &path, err
}

// Initialize auto-refresh event logic
func (g *Graph) InitAutoRefreshHandler(ttl int64) {
	g.RegisterEventHandler("graph.path.recompute", func(data map[string]string) {
		start := data["start"]
		end := data["end"]
		kind := data["kind"]
		if start == "" || end == "" || kind == "" {
			log.Println("Invalid path recompute event data")
			return
		}
		path, cost, err := g.ShortestPath(start, end, kind, ttl)
		if err != nil {
			log.Printf("Path recompute failed: %v", err)
		} else {
			log.Printf("Recomputed path from %s to %s [%s]: %v (cost: %d)", start, end, kind, path, cost)
		}
	})
}

type PriorityQueue []*Item

type Item struct {
	Node     string
	Cost     int
	Path     []string
	Priority int
	Index    int
}

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].Priority < pq[j].Priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.Index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}
