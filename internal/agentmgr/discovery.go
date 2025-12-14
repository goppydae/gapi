package agentmgr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/internal/eventbus"
	"github.com/goppydae/gapi/internal/lifecycle"
)

type Discovered struct {
	ID           string
	Type         string
	Lang         string
	Path         string
	Requires     []string
	Wants        []string
	WantedBy     []string
	RequiredBy   []string
	ListenStream string
}

type pyDescribe struct {
	Describe struct {
		ID           string   `json:"id"`
		Type         string   `json:"type"`
		Requires     []string `json:"requires"`
		Wants        []string `json:"wants"`
		WantedBy     []string `json:"wanted_by"`
		RequiredBy   []string `json:"required_by"`
		Enabled      bool     `json:"enabled"`
		ListenStream string   `json:"listen_stream"`
		CPULimit     string   `json:"cpu_limit"`
		MemoryLimit  string   `json:"memory_limit"`
	} `json:"describe"`
}

type Agent interface {
	ID() string
	Type() string
	Lang() string
	Dependencies() []string
	Controller() *lifecycle.Controller
	Describe() map[string]string
}

type AgentManager struct {
	bus    *eventbus.EventBus[*anypb.Any] // ← T is *anypb.Any
	lbus   *lifecycle.TypedBus
	pyRun  string // path to adk runner
	agents map[string]Agent
}

func NewAgentManager(bus *eventbus.EventBus[*anypb.Any], lbus *lifecycle.TypedBus, pyRunnerPath string) *AgentManager {
	return &AgentManager{
		bus: bus, lbus: lbus, pyRun: pyRunnerPath, agents: map[string]Agent{},
	}
}

func (am *AgentManager) Register(a Agent)    { am.agents[a.ID()] = a }
func (am *AgentManager) Get(id string) Agent { return am.agents[id] }
func (am *AgentManager) All() map[string]Agent {
	out := make(map[string]Agent, len(am.agents))
	for k, v := range am.agents {
		out[k] = v
	}
	return out
}

func (am *AgentManager) DiscoverFromPath(root string) ([]map[string]string, error) {
	var out []map[string]string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(d.Name(), ".service") && !strings.HasSuffix(d.Name(), ".timer") && !strings.HasSuffix(d.Name(), ".socket") {
			return nil
		}
		if !strings.Contains(d.Name(), ".py.") {
			return nil
		}

		desc, err := am.pythonDescribe(p)
		if err != nil {
			println(err.Error())
			return nil
		}
		meta := Discovered{
			ID: desc.Describe.ID, Type: strings.ToLower(desc.Describe.Type),
			Lang: "python", Path: p,
			Requires:     append([]string(nil), desc.Describe.Requires...),
			Wants:        append([]string(nil), desc.Describe.Wants...),
			WantedBy:     append([]string(nil), desc.Describe.WantedBy...),
			RequiredBy:   append([]string(nil), desc.Describe.RequiredBy...),
			ListenStream: desc.Describe.ListenStream,
		}
		a := NewPythonAgent(
			meta.ID, meta.Type, meta.Path, am.pyRun,
			meta.Requires, meta.Wants, meta.WantedBy, meta.RequiredBy,
			desc.Describe.ListenStream,
			desc.Describe.CPULimit,
			desc.Describe.MemoryLimit,
			am.bus,
			depView{am}, // Kept original as `am.depView` does not exist
		)
		am.Register(a)
		out = append(out, a.Describe())
		return nil
	})
	return out, nil
}

func (am *AgentManager) pythonDescribe(modulePath string) (*pyDescribe, error) {
	runnerAbs, err := filepath.Abs(am.pyRun)
	if err != nil {
		return nil, fmt.Errorf("describe: abs runner: %w", err)
	}
	modAbs, err := filepath.Abs(modulePath)
	if err != nil {
		return nil, fmt.Errorf("describe: abs module: %w", err)
	}
	if _, err := os.Stat(runnerAbs); err != nil {
		return nil, fmt.Errorf("describe: runner not found: %s (%w)", runnerAbs, err)
	}
	if _, err := os.Stat(modAbs); err != nil {
		return nil, fmt.Errorf("describe: module not found: %s (%w)", modAbs, err)
	}

	pythonBin := "python"
	if _, err := exec.LookPath(pythonBin); err != nil {
		if _, err3 := exec.LookPath("python3"); err3 == nil {
			pythonBin = "python3"
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, pythonBin, runnerAbs, "--module", modAbs, "--describe")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	cmdline := fmt.Sprintf("%s %s --module %s --describe", pythonBin, runnerAbs, modAbs)

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("describe: runner failed (code=%d): %s\ncmd: %s",
				exitErr.ExitCode(), bytes.TrimSpace(stderr.Bytes()), cmdline)
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("describe: timeout: %v\nstderr: %s\ncmd: %s",
				ctx.Err(), bytes.TrimSpace(stderr.Bytes()), cmdline)
		}
		return nil, fmt.Errorf("describe: exec error: %v\nstderr: %s\ncmd: %s",
			err, bytes.TrimSpace(stderr.Bytes()), cmdline)
	}

	out := bytes.TrimSpace(stdout.Bytes())
	out = bytes.TrimPrefix(out, []byte{0xEF, 0xBB, 0xBF})
	if len(out) == 0 {
		return nil, fmt.Errorf("describe: empty stdout; stderr: %s\ncmd: %s",
			bytes.TrimSpace(stderr.Bytes()), cmdline)
	}

	var d pyDescribe
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, fmt.Errorf("describe: invalid JSON: %v\nstdout: %q\nstderr: %s\ncmd: %s",
			err, string(out), bytes.TrimSpace(stderr.Bytes()), cmdline)
	}
	return &d, nil
}

// DependencyResolver for lifecycle
type depView struct{ am *AgentManager }

func (d depView) DepsOf(id string) []string {
	a := d.am.Get(id)
	if a == nil {
		return nil
	}
	return a.Dependencies()
}
func (d depView) IsRunning(id string) bool {
	a := d.am.Get(id)
	if a == nil {
		return false
	}
	return a.Controller().State() == lifecycle.StateRunning
}

func (d depView) EnsureStarted(id string) error {
	a := d.am.Get(id)
	if a == nil {
		return fmt.Errorf("dependency %q not found", id)
	}
	// Recursive call via controller
	return a.Controller().Apply(lifecycle.ActionStart)
}
