package timer

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/goppydae/gapi/internal/eventbus"
)

type TimerAgent struct {
	id        string
	scope     string
	topicRoot string
	script    string
	lang      string
	interval  time.Duration
	bus       *eventbus.EventBus

	mu     sync.Mutex
	ticker *time.Ticker
	stop   chan struct{}
	status string
}

func NewTimerAgent(id, scope, topicRoot, scriptPath, lang string, intervalSec int, bus *eventbus.EventBus) *TimerAgent {
	return &TimerAgent{
		id:        id,
		scope:     scope,
		topicRoot: topicRoot,
		script:    scriptPath,
		lang:      lang,
		interval:  time.Duration(intervalSec) * time.Second,
		bus:       bus,
		status:    "initialized",
	}
}

func (t *TimerAgent) ID() string    { return t.id }
func (t *TimerAgent) Scope() string { return t.scope }
func (t *TimerAgent) Type() string  { return "timer" }

func (t *TimerAgent) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.ticker != nil {
		return nil
	}

	t.status = "running"
	t.ticker = time.NewTicker(t.interval)
	t.stop = make(chan struct{})

	log.Printf("[%s] timer agent started (%s every %v)", t.id, filepath.Base(t.script), t.interval)

	go func() {
		for {
			select {
			case <-t.ticker.C:
				t.fire()
			case <-t.stop:
				return
			}
		}
	}()

	return nil
}

func (t *TimerAgent) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.ticker == nil {
		return nil
	}

	t.ticker.Stop()
	close(t.stop)
	t.ticker = nil
	t.stop = nil
	t.status = "stopped"

	log.Printf("[%s] timer agent stopped", t.id)
	return nil
}

func (t *TimerAgent) Restart() error { return t.Reload() }

func (t *TimerAgent) Reload() error {
	if err := t.Stop(); err != nil {
		return err
	}
	return t.Start()
}
func (t *TimerAgent) Describe() map[string]string {
	return map[string]string{
		"id":     t.id,
		"scope":  t.scope,
		"type":   "timer",
		"lang":   t.lang,
		"status": t.status,
	}
}

func (t *TimerAgent) fire() {
	cmd, err := t.buildCmd()
	if err != nil {
		log.Printf("[%s] buildCmd error: %v", t.id, err)
		return
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[%s] error during timer execution: %v", t.id, err)
		return
	}

	log.Printf("[%s] timer tick: %s", t.id, string(output))

	// Convert map to Struct
	structPayload, err := structpb.NewStruct(map[string]interface{}{
		"output": string(output),
	})
	if err != nil {
		log.Printf("[%s] struct conversion error: %v", t.id, err)
		return
	}

	// Wrap in Any
	anyPayload, err := anypb.New(structPayload)
	if err != nil {
		log.Printf("[%s] packing into Any failed: %v", t.id, err)
		return
	}

	t.bus.Publish(eventbus.Event{
		Scope:     t.scope,
		Topic:     t.topicRoot + "/tick",
		Payload:   anyPayload,
		Source:    t.id,
		Broadcast: false,
	})
}

func (t *TimerAgent) buildCmd() (*exec.Cmd, error) {
	switch t.lang {
	case "py":
		return exec.Command("python3", t.script), nil
	case "sh":
		return exec.Command("sh", t.script), nil
	case "go":
		return exec.Command(t.script), nil
	default:
		return nil, fmt.Errorf("[%s] unknown or unsupported language: %s", t.id, t.lang) // assume binary
	}
}
