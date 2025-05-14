package agentreg

import (
	"fmt"
	"sync"

	"github.com/goppydae/gapi/core/store"
	"github.com/goppydae/gapi/internal/db/graphdb"
)

type AgentDescription struct {
	ID           string   `json:"id"`
	Path         string   `json:"path"`
	Type         string   `json:"type"`
	Language     string   `json:"language"`
	Version      string   `json:"version"`
	Hash         string   `json:"hash"`
	Capabilities []string `json:"capabilities"`
	Requires     []string `json:"requires"`
	Wants        []string `json:"wants"`
	Tags         []string `json:"tags"`
}

type AgentRegistry struct {
	store  store.HybridStore
	nodeMu sync.Mutex
}

const agentsBucket = "agents"

func NewAgentRegistry(s store.HybridStore) (*AgentRegistry, error) {
	r := &AgentRegistry{
		store: s,
	}
	return r, nil
}

func (r *AgentRegistry) Register(agent *AgentDescription) error {
	id := agent.ID
	return r.store.Set(agentsBucket, id, agent)
}

func (r *AgentRegistry) Lookup(id string) (*AgentDescription, error) {
	var agent AgentDescription
	err := r.store.Get(agentsBucket, id, &agent)
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func (r *AgentRegistry) List() ([]*AgentDescription, error) {
	keys, err := r.store.Keys(agentsBucket)
	if err != nil {
		return nil, err
	}

	var agents []*AgentDescription
	for _, k := range keys {
		agent, err := r.Lookup(k)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

func (r *AgentRegistry) GetDependencies(id string) ([]string, error) {
	agent, err := r.Lookup(id)
	if err != nil {
		return nil, err
	}
	return append(agent.Requires, agent.Wants...), nil
}

func (r *AgentRegistry) TopologicalSort() ([]string, error) {
	agents, err := r.List()
	if err != nil {
		return nil, err
	}

	nodeIndex := make(map[string]struct{})
	for _, a := range agents {
		r.store.AddNode(graphdb.Node{ID: a.ID})
		nodeIndex[a.ID] = struct{}{}
	}

	for _, a := range agents {
		for _, dep := range append(a.Requires, a.Wants...) {
			if _, ok := nodeIndex[dep]; ok {
				r.store.AddEdge(graphdb.Edge{From: a.ID, To: dep, Kind: "dependency"})
			}
		}
	}

	// Assume store.graph.ShortestPath is enough for now; topo.Sort is unavailable
	// So we return a naive linearization by counting 0-dependency agents first
	var sorted []string
	visited := make(map[string]bool)

	for len(sorted) < len(agents) {
		didWork := false
		for _, a := range agents {
			if visited[a.ID] {
				continue
			}
			deps, _ := r.GetDependencies(a.ID)
			ready := true
			for _, d := range deps {
				if !visited[d] {
					ready = false
					break
				}
			}
			if ready {
				visited[a.ID] = true
				sorted = append(sorted, a.ID)
				didWork = true
			}
		}
		if !didWork {
			return nil, fmt.Errorf("cycle detected or missing dependencies")
		}
	}

	return sorted, nil
}

func (r *AgentRegistry) Close() error {
	return r.store.Close()
}
