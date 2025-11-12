package agentmgr

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/internal/agents/service"
	"github.com/goppydae/gapi/internal/agents/timer"
	"github.com/goppydae/gapi/internal/eventbus"
	"github.com/goppydae/gapi/internal/lifecycle"
)

type AgentManager struct {
	mu     sync.RWMutex
	agents map[string]lifecycle.Agent
	bus    *eventbus.EventBus[*anypb.Any]
}

func NewAgentManager(bus *eventbus.EventBus[*anypb.Any]) *AgentManager {
	return &AgentManager{
		agents: make(map[string]lifecycle.Agent),
		bus:    bus,
	}
}

// RegisterFromFile parses the filename and dispatches to the appropriate agent constructor
func (am *AgentManager) RegisterFromFile(filePath string) error {
	parts := strings.Split(filepath.Base(filePath), ".")

	if len(parts) < 3 {
		return fmt.Errorf("invalid filename: expected format <name>.<lang>.<type>, got %s", filePath)
	}

	typ := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))

	var agent lifecycle.Agent

	switch typ {
	case "timer":
		var err error
		agent, err = timer.NewTimerAgent(filePath)
		if err != nil {
			return err
		}
	case "service":
		var err error
		agent, err = service.NewServiceAgent(filePath)
		if err != nil {
			return err
		}
	default:
		return errors.New("unsupported agent type: " + typ)
	}

	return am.RegisterAgent(agent)
}

func (am *AgentManager) RegisterAgent(a lifecycle.Agent) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	id := a.ID()
	am.agents[id] = a
	log.Printf("Agent registered: [%s] type=%s", id, a.Type())

	scope := a.Scope()
	root := id

	sub := func(topic string, fn func(eventbus.Event[*anypb.Any])) {
		if err := am.bus.Subscribe(scope, topic, fn); err != nil {
			log.Printf("subscribe %s [%s]: %v", topic, id, err)
		}
	}

	sub(root+"/control.start", func(e eventbus.Event[*anypb.Any]) {
		if err := a.Start(); err != nil {
			log.Printf("start[%s]: %v", id, err)
		}
	})
	sub(root+"/control.stop", func(e eventbus.Event[*anypb.Any]) {
		if err := a.Stop(); err != nil {
			log.Printf("stop[%s]: %v", id, err)
		}
	})

	// restart adapter: prefer Restart() if present, else Stop()+Start()
	type restarter interface{ Restart() error }
	sub(root+"/control.restart", func(e eventbus.Event[*anypb.Any]) {
		if r, ok := any(a).(restarter); ok {
			if err := r.Restart(); err != nil {
				log.Printf("restart[%s]: %v", id, err)
			}
			return
		}
		if err := a.Stop(); err != nil {
			log.Printf("restart/stop[%s]: %v", id, err)
			return
		}
		if err := a.Start(); err != nil {
			log.Printf("restart/start[%s]: %v", id, err)
		}
	})

	sub(root+"/control.reload", func(e eventbus.Event[*anypb.Any]) {
		if err := a.Reload(); err != nil {
			log.Printf("reload[%s]: %v", id, err)
		}
	})

	return nil
}

func (am *AgentManager) StopAll() {
	// snapshot & release lock before slow ops
	am.mu.RLock()
	snapshot := make(map[string]lifecycle.Agent, len(am.agents))
	for id, a := range am.agents {
		snapshot[id] = a
	}
	am.mu.RUnlock()

	var wg sync.WaitGroup
	for id, a := range snapshot {
		wg.Add(1)
		go func(id string, a lifecycle.Agent) {
			defer wg.Done()
			if err := a.Stop(); err != nil {
				log.Printf("Error stopping agent [%s]: %v", id, err)
			}
			if closer, ok := a.(interface{ Close() error }); ok {
				if err := closer.Close(); err != nil {
					log.Printf("Error closing transport for agent [%s]: %v", id, err)
				}
			}
		}(id, a)
	}
	wg.Wait()
	log.Println("All agents stopped.")
}

func (am *AgentManager) Describe() []map[string]string {
	am.mu.RLock()
	defer am.mu.RUnlock()

	out := make([]map[string]string, 0, len(am.agents))
	for _, a := range am.agents {
		if a == nil {
			continue
		}
		info := a.Describe()
		if info == nil || info.ID == "" {
			continue
		}
		out = append(out, map[string]string{
			"id":          info.ID,
			"name":        info.Name,
			"version":     info.Version,
			"type":        info.Type,
			"description": info.Description,
			"enabled":     fmt.Sprintf("%v", info.Enabled),
			"interval":    fmt.Sprintf("%d", info.Interval),
		})
	}
	return out
}

func (am *AgentManager) Get(id string) lifecycle.Agent {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.agents[id]
}

func (am *AgentManager) DiscoverFromPath(path string) ([]map[string]string, error) {
	var walkErr error

	// 1) Walk and register files (same behavior as before, just no filename->ID lookup)
	walkErr = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		// keep your existing filter(s); if you want to be stricter/looser, adjust here
		if strings.Count(d.Name(), ".") < 2 {
			return nil
		}
		if err := am.RegisterFromFile(p); err != nil {
			log.Printf("Discovery skipped: %v", err)
			return nil // keep walking
		}
		return nil
	})
	// if walkErr != nil we’ll still try to report whatever got registered

	// 2) Snapshot registered agents and build the output from their self-reported info
	am.mu.RLock()
	defer am.mu.RUnlock()

	out := make([]map[string]string, 0, len(am.agents))
	for _, agent := range am.agents {
		if agent == nil {
			continue
		}
		info := agent.Describe()
		if info == nil || info.ID == "" {
			continue // ignore malformed registrations
		}
		out = append(out, map[string]string{
			"id":          info.ID,
			"name":        info.Name,
			"version":     info.Version,
			"type":        info.Type,
			"description": info.Description,
			"enabled":     fmt.Sprintf("%v", info.Enabled),
			"interval":    fmt.Sprintf("%d", info.Interval),
		})
	}

	return out, walkErr
}
