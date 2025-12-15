// Lifecycle agent demonstrating full lifecycle support
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var describe = flag.Bool("describe", false, "Print agent metadata")

type State struct {
	initialized bool
	running     bool
}

var state = State{}

func main() {
	flag.Parse()

	if *describe {
		metadata := map[string]interface{}{
			"describe": map[string]interface{}{
				"id":           "lifecycle_agent",
				"type":         "service",
				"version":      "1.0.0",
				"language":     "go",
				"description":  "Agent demonstrating full lifecycle support",
				"capabilities": []string{"initialize", "start", "stop", "reload"},
			},
		}
		json.NewEncoder(os.Stdout).Encode(metadata)
		return
	}

	// Initialize
	initialize()

	// Start
	start()
}

func initialize() {
	state.initialized = true
	fmt.Println("[lifecycle_agent] Initialized")
}

func start() {
	if !state.initialized {
		fmt.Fprintln(os.Stderr, "Cannot start before initialization")
		os.Exit(1)
	}

	state.running = true
	fmt.Println("[lifecycle_agent] Started")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for state.running {
		select {
		case <-ticker.C:
			// Service runs
		case sig := <-sigChan:
			if sig == syscall.SIGHUP {
				reload()
			} else {
				stop()
			}
		}
	}
}

func stop() {
	state.running = false
	fmt.Println("[lifecycle_agent] Stopped")
}

func reload() {
	fmt.Println("[lifecycle_agent] Reloaded")
}
