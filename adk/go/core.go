package adk

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/goppydae/gapi/internal/eventbus"
	protopkg "github.com/goppydae/gapi/internal/proto"
	"github.com/goppydae/gapi/internal/transport"
	"google.golang.org/protobuf/types/known/anypb"
)

// This package is designed to be bound to Python via gopy.
// It avoids channels in public signatures.

var (
	mu           sync.Mutex
	statusReport struct {
		State string
		Msg   string
	}
	// Simple command mailbox
	cmdMailbox struct {
		cond *sync.Cond
		cmd  string // e.g. "START", "STOP"
	}
	quicClient *transport.QUIC
)

func init() {
	cmdMailbox.cond = sync.NewCond(&mu)
}

// StartQUIC initializes the QUIC connection to the supervisor.
func StartQUIC(addr string) error {
	mu.Lock()
	defer mu.Unlock()

	if quicClient != nil {
		return nil
	}

	qc, err := transport.NewQUICClient(addr, nil)
	if err != nil {
		return fmt.Errorf("failed to start QUIC client: %w", err)
	}
	quicClient = qc
	log.Printf("[GAPI-ADK] Connected to supervisor at %s (QUIC)", addr)
	return nil
}

// Initialize sets up the agent identity.
func Initialize(name, version, typeStr string) {
	log.Printf("[GAPI-ADK] Initialized agent: %s v%s (%s)", name, version, typeStr)
}

// SendEvent sends a generic event to the supervisor via QUIC.
func SendEvent(jsonStr string) {
	mu.Lock()
	defer mu.Unlock()

	// If no QUIC client, fall back to stdout for debugging/bootstrap
	if quicClient == nil {
		fmt.Println(jsonStr)
		return
	}

	// Parse JSON to extract topic/id/type
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		log.Printf("[GAPI-ADK] SendEvent JSON error: %v", err)
		return
	}

	event, _ := data["event"].(string)
	_ = event // Topic logic uses fixed string for now
	id, _ := data["id"].(string)
	// state, _ := data["state"].(string)

	// Map to LifecycleStatus
	// Create a minimal LifecycleStatus matching proto definition
	stat := &protopkg.LifecycleStatus{
		AgentId: id,
		State:   "unknown", // Default to unknown or extract from data
		Message: jsonStr,
	}

	anyStat, err := anypb.New(stat)
	if err != nil {
		log.Printf("[GAPI-ADK] Proto marshal error: %v", err)
		return
	}

	ev := eventbus.Event[*anypb.Any]{
		ID:      id,
		Topic:   "agent/lifecycle.status",
		Payload: anyStat,
		Source:  "agent:" + id,
	}

	if err := quicClient.PublishRemote(ev); err != nil {
		log.Printf("[GAPI-ADK] QUIC publish error: %v", err)
		// Fallback
		fmt.Println(jsonStr)
	}
}

// AwaitCommand blocks until a command is received from the supervisor.
// Returns the command string (e.g. "start", "stop").
// In a real implementation, this would read from a QUIC stream or IPC socket.
func AwaitCommand() string {
	cmdMailbox.cond.L.Lock()
	defer cmdMailbox.cond.L.Unlock()

	// Wait for command (simulated)
	for cmdMailbox.cmd == "" {
		cmdMailbox.cond.Wait()
	}
	c := cmdMailbox.cmd
	cmdMailbox.cmd = "" // clear after read
	return c
}

// InjectCommand is a helper for testing/simulation to push a command into the mailbox.
func InjectCommand(cmd string) {
	cmdMailbox.cond.L.Lock()
	defer cmdMailbox.cond.L.Unlock()
	cmdMailbox.cmd = cmd
	cmdMailbox.cond.Signal()
}

// StartHeartbeat starts a background goroutine that sends heartbeat events.
func StartHeartbeat(id, typeStr string) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			jsonStr := fmt.Sprintf(`{"event":"heartbeat","id":"%s","type":"%s"}`, id, typeStr)
			SendEvent(jsonStr)
		}
	}()
}
