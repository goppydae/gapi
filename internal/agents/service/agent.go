package service

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/goppydae/gapi/internal/eventbus"
)

type ServiceAgent struct {
	id        string
	scope     string
	topicRoot string
	script    string
	lang      string
	cmd       *exec.Cmd
	status    string
	bus       *eventbus.EventBus
	mu        sync.Mutex
}

func NewServiceAgent(id, scope, topicRoot, scriptPath, lang string, bus *eventbus.EventBus) *ServiceAgent {
	return &ServiceAgent{
		id:        id,
		scope:     scope,
		topicRoot: topicRoot,
		script:    scriptPath,
		lang:      lang,
		status:    "initialized",
		bus:       bus,
	}
}

func (s *ServiceAgent) ID() string    { return s.id }
func (s *ServiceAgent) Scope() string { return s.scope }
func (s *ServiceAgent) Type() string  { return "service" }

func (s *ServiceAgent) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		return nil
	}

	cmd, err := s.buildCmd()
	if err != nil {
		log.Printf("[%s] buildCmd error: %v", s.id, err)
		return err
	}

	s.cmd = cmd
	if err := s.cmd.Start(); err != nil {
		log.Printf("[%s] failed to start service: %v", s.id, err)
		return err
	}

	s.status = "running"
	log.Printf("[%s] service agent started (%s)", s.id, filepath.Base(s.script))
	return nil
}

func (s *ServiceAgent) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	err := s.cmd.Process.Kill()
	s.cmd = nil
	s.status = "stopped"
	log.Printf("[%s] service agent stopped", s.id)
	return err
}

func (s *ServiceAgent) Restart() error { return s.Reload() }

func (s *ServiceAgent) Reload() error {
	if err := s.Stop(); err != nil {
		return err
	}
	return s.Start()
}

func (s *ServiceAgent) Describe() map[string]string {
	return map[string]string{
		"id":     s.id,
		"scope":  s.scope,
		"type":   "service",
		"lang":   s.lang,
		"status": s.status,
	}
}

func (s *ServiceAgent) buildCmd() (*exec.Cmd, error) {
	switch s.lang {
	case "py":
		return exec.Command("python3", s.script), nil
	case "sh":
		return exec.Command("sh", s.script), nil
	case "go":
		return exec.Command(s.script), nil
	default:
		return nil, fmt.Errorf("[%s] unknown or unsupported language: %s", s.id, s.lang)
	}
}
