package agentreg

import (
	"fmt"
	"strings"
	"sync"

	"github.com/goppydae/gapi/core/store"
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
	if agent == nil || strings.TrimSpace(agent.ID) == "" || strings.TrimSpace(agent.Type) == "" {
		return fmt.Errorf("invalid agent: %+v", agent)
	}
	return r.store.Set(agentsBucket, agent.ID, agent)
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

	// Build adjacency using ONLY hard deps (Requires); Wants to remain soft.
	reqs := make(map[string][]string, len(agents))
	exists := make(map[string]struct{}, len(agents))
	for _, a := range agents {
		exists[a.ID] = struct{}{}
		reqs[a.ID] = append([]string(nil), a.Requires...)
	}

	// Kahn's algorithm (in-memory, no store side effects)
	inDeg := map[string]int{}
	for id := range exists {
		inDeg[id] = 0
	}
	for id, deps := range reqs {
		_ = id
		for _, d := range deps {
			if _, ok := exists[d]; ok {
				inDeg[id]++
			}
		}
	}

	queue := make([]string, 0, len(agents))
	for id, deg := range inDeg {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		// pop last (stack-ish is fine)
		n := len(queue) - 1
		id := queue[n]
		queue = queue[:n]
		order = append(order, id)

		for depOwner, deps := range reqs {
			for _, d := range deps {
				if d == id {
					inDeg[depOwner]--
					if inDeg[depOwner] == 0 {
						queue = append(queue, depOwner)
					}
				}
			}
		}
	}

	if len(order) != len(agents) {
		return nil, fmt.Errorf("cycle detected or missing hard dependency")
	}

	// Sync now we know the DAG is valid.
	r.syncGraph(agents)

	return order, nil
}

func (r *AgentRegistry) Close() error {
	return r.store.Close()
}
