package timer

import (
	"fmt"

	"github.com/goppydae/gapi/core/adk"
	"github.com/goppydae/gapi/core/adk/loader"
	"github.com/goppydae/gapi/core/adk/meta"
)

// TimerAgent wraps a Python agent for timer-triggered behavior.
type TimerAgent struct {
	adk.Agent
	path string
}

// NewTimerAgent creates a new timer agent from a Python file.
func NewTimerAgent(path string) (*TimerAgent, error) {
	agent, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load timer agent: %w", err)
	}
	return &TimerAgent{
		Agent: agent,
		path:  path,
	}, nil
}

func (t *TimerAgent) ID() string {
	return t.Agent.Info().ID
}

func (t *TimerAgent) Type() string {
	return t.Agent.Info().Type
}

func (t *TimerAgent) Scope() string {
	return "system"
}

func (t *TimerAgent) Describe() *meta.AgentInfo {
	return t.Agent.Info()
}

// Optionally override lifecycle methods

func (t *TimerAgent) Start() error {
	fmt.Println("[timer] starting:", t.path)
	return t.Agent.Start()
}

func (t *TimerAgent) Stop() error {
	fmt.Println("[timer] stopping:", t.path)
	return t.Agent.Stop()
}

func (t *TimerAgent) Restart() error {
	fmt.Println("[timer] restarting:", t.path)
	if hooks, ok := t.Agent.(adk.OptionalHooks); ok {
		return hooks.Restart()
	}
	return nil // not an error if not implemented
}

func (t *TimerAgent) Reload() error {
	if hooks, ok := t.Agent.(adk.OptionalHooks); ok {
		return hooks.Reload()
	}
	return nil
}
