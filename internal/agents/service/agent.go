package service

import (
	"fmt"

	"github.com/goppydae/gapi/core/adk"
	"github.com/goppydae/gapi/core/adk/loader"
	"github.com/goppydae/gapi/core/adk/meta"
)

// ServiceAgent wraps a Python agent for service-type behavior.
type ServiceAgent struct {
	adk.Agent
	path string
}

// NewServiceAgent creates a new service agent from a Python file.
func NewServiceAgent(path string) (*ServiceAgent, error) {
	agent, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load service agent: %w", err)
	}
	return &ServiceAgent{
		Agent: agent,
		path:  path,
	}, nil
}

func (s *ServiceAgent) ID() string {
	return s.Agent.Info().ID
}

func (s *ServiceAgent) Type() string {
	return s.Agent.Info().Type
}

func (s *ServiceAgent) Scope() string {
	return "system" // or read from file metadata if needed
}

func (s *ServiceAgent) Describe() *meta.AgentInfo {
	return s.Info()
}

// Optionally override lifecycle methods

func (s *ServiceAgent) Start() error {
	fmt.Println("[service] starting:", s.path)
	return s.Agent.Start()
}

func (s *ServiceAgent) Stop() error {
	fmt.Println("[service] stopping:", s.path)
	return s.Agent.Stop()
}

func (s *ServiceAgent) Restart() error {
	fmt.Println("[service] restarting:", s.path)
	if hooks, ok := s.Agent.(adk.OptionalHooks); ok {
		return hooks.Restart()
	}
	return nil // not an error if not implemented
}

func (s *ServiceAgent) Reload() error {
	if hooks, ok := s.Agent.(adk.OptionalHooks); ok {
		return hooks.Reload()
	}
	return nil // Not implemented, silently skip
}
