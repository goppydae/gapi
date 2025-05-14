package agentmgr

import (
	"errors"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/goppydae/gapi/internal/agents/service"
	"github.com/goppydae/gapi/internal/agents/timer"
	"github.com/goppydae/gapi/internal/eventbus"
	"github.com/goppydae/gapi/internal/lifecycle"
)

type AgentManager struct {
	mu     sync.RWMutex
	agents map[string]lifecycle.Agent
	bus    *eventbus.EventBus
}

func NewAgentManager(bus *eventbus.EventBus) *AgentManager {
	return &AgentManager{
		agents: make(map[string]lifecycle.Agent),
		bus:    bus,
	}
}

// RegisterFromFile parses the filename and dispatches to the appropriate agent constructor
func (am *AgentManager) RegisterFromFile(filePath string) error {
	base := filepath.Base(filePath)
	parts := strings.Split(base, ".")

	if len(parts) < 3 {
		return errors.New("filename must follow format <name>.<lang>.<type>")
	}

	id := parts[0]
	lang := parts[1]
	typ := parts[2]
	scope := "system"     // default for now
	topicRoot := "/" + id // default topic

	var agent lifecycle.Agent

	switch typ {
	case "timer":
		agent = timer.NewTimerAgent(id, scope, topicRoot, filePath, lang, 10, am.bus)
	case "service":
		agent = service.NewServiceAgent(id, scope, topicRoot, filePath, lang, am.bus)
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

	_ = am.bus.Subscribe(scope, root+"/control.start", func(e eventbus.Event) {
		if err := a.Start(); err != nil {
			log.Printf("Error starting agent [%s]: %v", id, err)
		}
	})
	_ = am.bus.Subscribe(scope, root+"/control.stop", func(e eventbus.Event) {
		if err := a.Stop(); err != nil {
			log.Printf("Error stopping agent [%s]: %v", id, err)
		}
	})
	_ = am.bus.Subscribe(scope, root+"/control.restart", func(e eventbus.Event) {
		if err := a.Restart(); err != nil {
			log.Printf("Error restarting agent [%s]: %v", id, err)
		}
	})
	_ = am.bus.Subscribe(scope, root+"/control.reload", func(e eventbus.Event) {
		if err := a.Reload(); err != nil {
			log.Printf("Error reloading agent [%s]: %v", id, err)
		}
	})

	return nil
}

func (am *AgentManager) StopAll() {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var wg sync.WaitGroup
	for id, a := range am.agents {
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

	var out []map[string]string
	for _, a := range am.agents {
		out = append(out, a.Describe())
	}
	return out
}

func (am *AgentManager) Get(id string) lifecycle.Agent {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.agents[id]
}

func (am *AgentManager) DiscoverFromPath(path string) ([]map[string]string, error) {
	var out []map[string]string

	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.Count(d.Name(), ".") < 2 {
			return nil
		}
		err = am.RegisterFromFile(p)
		if err != nil {
			log.Printf("Discovery skipped: %v", err)
			return nil
		}
		id := strings.Split(d.Name(), ".")[0]
		agent := am.Get(id)
		if agent != nil {
			out = append(out, agent.Describe())
		}
		return nil
	})

	return out, err
}
