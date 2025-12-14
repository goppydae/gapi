package adk

import (
	"fmt"
	"log"
	"sync"
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
)

func init() {
	cmdMailbox.cond = sync.NewCond(&mu)
}

// Initialize sets up the agent identity.
func Initialize(name, version, typeStr string) {
	log.Printf("[GAPI-ADK] Initialized agent: %s v%s (%s)", name, version, typeStr)
}

// SendEvent sends a raw JSON event string to the supervisor.
func SendEvent(jsonStr string) {
	mu.Lock()
	defer mu.Unlock()
	// Direct stdout for now, as gapid reads this stream
	fmt.Println(jsonStr)
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
