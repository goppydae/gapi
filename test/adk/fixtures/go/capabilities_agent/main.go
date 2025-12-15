// Capabilities agent with capability detection
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

func main() {
	flag.Parse()

	if *describe {
		metadata := map[string]interface{}{
			"describe": map[string]interface{}{
				"id":          "capabilities_agent",
				"type":        "service",
				"version":     "1.0.0",
				"language":    "go",
				"description": "Agent demonstrating capability detection",
				"capabilities": []string{
					"initialize",
					"start",
					"stop",
					"reload",
					"custom_action", // Custom capability
				},
			},
		}
		json.NewEncoder(os.Stdout).Encode(metadata)
		return
	}

	// Service logic
	initialize()
	start()
}

func initialize() {
	fmt.Println("[capabilities_agent] Initialized")
}

func start() {
	fmt.Println("[capabilities_agent] Started")

	// Demonstrate custom capability
	performAction()

	// Run indefinitely
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Service runs
		case <-sigChan:
			stop()
			return
		}
	}
}

func stop() {
	fmt.Println("[capabilities_agent] Stopped")
}

func reload() {
	fmt.Println("[capabilities_agent] Reloaded")
}

// Custom capability
func performAction() {
	fmt.Println("Performing custom action")
}
