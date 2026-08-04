// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

// Simple service agent for cross-ADK testing
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
				"id":           "simple_service",
				"type":         "service",
				"version":      "1.0.0",
				"language":     "go",
				"description":  "A minimal service agent for testing",
				"capabilities": []string{"initialize", "start", "stop"},
			},
		}
		if err := json.NewEncoder(os.Stdout).Encode(metadata); err != nil {
			fmt.Fprintln(os.Stderr, "encode describe metadata:", err)
			os.Exit(1)
		}
		return
	}

	// Service logic
	fmt.Println("[simple_service] Initialized")
	fmt.Println("[simple_service] Started")

	// Run indefinitely until signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Service runs
		case <-sigChan:
			fmt.Println("[simple_service] Stopped")
			return
		}
	}
}
