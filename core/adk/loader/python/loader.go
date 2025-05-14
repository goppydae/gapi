package python

import (
	"fmt"
	"os"

	"github.com/go-python/gpython/py"
	"github.com/goppydae/gapi/core/adk/meta"
)

// Agent represents a parsed Python agent with its metadata and lifecycle access
type Agent struct {
	Meta    *meta.AgentInfo
	Globals py.StringDict
}

// Load reads and introspects a Python agent file using gpython
func Load(path string) (*Agent, error) {
	code, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read error: %w", err)
	}

	// Create a new isolated module environment
	mod := py.NewModule("__main__", "__main__")
	globals := mod.Globals

	// Execute the Python code
	_, err = py.Run(string(code), globals, "__main__")
	if err != nil {
		return nil, fmt.Errorf("python exec error: %w", err)
	}

	// Required constants
	required := []string{"ID", "NAME", "VERSION", "TYPE"}
	values := make(map[string]string)

	for _, key := range required {
		val, ok := globals[py.String(key)]
		if !ok {
			return nil, fmt.Errorf("missing required constant: %s", key)
		}
		values[key] = val.String()
	}

	// Optional constants
	description := ""
	if val, ok := globals[py.String("DESCRIPTION")]; ok {
		description = val.String()
	}

	interval := 0
	if val, ok := globals[py.String("INTERVAL")]; ok {
		if i, ok := py.AsInt(val); ok {
			interval = i
		}
	}

	enabled := false
	if val, ok := globals[py.String("ENABLED")]; ok {
		enabled = val.IsTrue()
	}

	// Detect available lifecycle functions
	lifecycle := []string{}
	for _, fn := range []string{"initialize", "start", "stop", "restart", "reload"} {
		if val, ok := globals[py.String(fn)]; ok && py.HasAttr(val, "__call__") {
			lifecycle = append(lifecycle, fn)
		}
	}

	info := &meta.AgentInfo{
		ID:          values["ID"],
		Name:        values["NAME"],
		Version:     values["VERSION"],
		Type:        values["TYPE"],
		Description: description,
		Interval:    interval,
		Enabled:     enabled,
		Implements:  lifecycle,
	}

	return &Agent{
		Meta:    info,
		Globals: globals,
	}, nil
}

// Call invokes a lifecycle function by name if it exists
func (a *Agent) Call(name string) error {
	fn, ok := a.Globals[py.String(name)]
	if !ok {
		return fmt.Errorf("function %s not found", name)
	}

	_, err := fn.Call(nil, nil)
	if err != nil {
		return fmt.Errorf("error calling %s: %w", name, err)
	}
	return nil
}

// Info returns the loaded metadata
func (a *Agent) Info() *meta.AgentInfo {
	return a.Meta
}
