package agentmgr

import (
	"testing"

	"github.com/goppydae/gapi/internal/lifecycle"
)

// MockAgent to satisfy Agent interface
type MockAgent struct {
	id   string
	deps []string
	caps []string
}

func (m *MockAgent) ID() string                        { return m.id }
func (m *MockAgent) Type() string                      { return "service" }
func (m *MockAgent) Lang() string                      { return "mock" }
func (m *MockAgent) Dependencies() []string            { return m.deps }
func (m *MockAgent) Controller() *lifecycle.Controller { return nil }
func (m *MockAgent) Describe() map[string]string {
	return map[string]string{"capabilities": ""} // Simplified
}

func TestTopologicalSort(t *testing.T) {
	tests := []struct {
		name   string
		agents map[string]Agent
		want   []string // One valid order (simplified check for now) or subset check?
		// Topological sort isn't unique, so checking exact match isn't great.
		// However, the implementation uses deterministic order if input map iteration is handled.
		// NOTE: Go map iteration is random. The sort output might vary for independent nodes.
		// But dependent nodes must be ordered correctly.
		wantErr bool
	}{
		{
			name: "Simple Chain A->B",
			agents: map[string]Agent{
				"A": &MockAgent{id: "A", deps: []string{"B"}},
				"B": &MockAgent{id: "B", deps: []string{}},
			},
			// B must come before A
			// Expected: [B, A]
			want: []string{"B", "A"},
		},
		{
			name: "Diamond",
			agents: map[string]Agent{
				"A": &MockAgent{id: "A", deps: []string{"B", "C"}},
				"B": &MockAgent{id: "B", deps: []string{"D"}},
				"C": &MockAgent{id: "C", deps: []string{"D"}},
				"D": &MockAgent{id: "D", deps: []string{}},
			},
			// D first. B and C in any order. A last.
			// Valid: D, B, C, A or D, C, B, A
			// We will check order validity in code.
		},
		{
			name: "Cycle A->B->A",
			agents: map[string]Agent{
				"A": &MockAgent{id: "A", deps: []string{"B"}},
				"B": &MockAgent{id: "B", deps: []string{"A"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TopologicalSort(tt.agents)
			if (err != nil) != tt.wantErr {
				t.Errorf("TopologicalSort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Verify order validity
			seen := make(map[string]bool)
			for _, id := range got {
				seen[id] = true
				agent := tt.agents[id]
				for _, dep := range agent.Dependencies() {
					if _, exists := tt.agents[dep]; exists && !seen[dep] {
						t.Errorf("Order violation: %s depends on %s, but %s not seen yet. Order: %v", id, dep, dep, got)
					}
				}
			}

			if len(got) != len(tt.agents) {
				t.Errorf("Missing agents in output. Got %d, want %d", len(got), len(tt.agents))
			}
		})
	}
}
