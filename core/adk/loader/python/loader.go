package python

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/goppydae/gapi/core/adk/meta"
)

type Agent struct {
	Meta *meta.AgentInfo
	Path string
}

func Load(path string) (*Agent, error) {
	cmd := exec.Command("python3", "adk/python/agent/runner.py", path, "--describe")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("agent describe failed: %v\n%s", err, out.String())
	}

	var info meta.AgentInfo
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		return nil, fmt.Errorf("describe output invalid: %w\n%s", err, out.String())
	}

	return &Agent{
		Meta: &info,
		Path: path,
	}, nil
}

func (a *Agent) Initialize() error         { return nil }
func (a *Agent) Start() error              { return nil }
func (a *Agent) Stop() error               { return nil }
func (a *Agent) Restart() error            { return nil }
func (a *Agent) Reload() error             { return nil }
func (a *Agent) Describe() *meta.AgentInfo { return a.Meta }
func (a *Agent) Info() *meta.AgentInfo     { return a.Meta }
