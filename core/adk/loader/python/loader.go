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

// errDescribeOnly is returned by the lifecycle methods below. This loader only
// introspects a Python agent (via --describe); the actual subprocess lifecycle
// is driven by core/agentmgr's PythonAgent/runner. Previously these methods
// silently returned nil, so callers routing lifecycle through the loader saw
// success with no effect. Returning an explicit error surfaces the foot-gun.
var errDescribeOnly = fmt.Errorf("python loader is describe-only; drive lifecycle via core/agentmgr")

func (a *Agent) Initialize() error         { return errDescribeOnly }
func (a *Agent) Start() error              { return errDescribeOnly }
func (a *Agent) Stop() error               { return errDescribeOnly }
func (a *Agent) Restart() error            { return errDescribeOnly }
func (a *Agent) Reload() error             { return errDescribeOnly }
func (a *Agent) Describe() *meta.AgentInfo { return a.Meta }
func (a *Agent) Info() *meta.AgentInfo     { return a.Meta }
