// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

// Orphanmaker is the PID-1 e2e fixture agent: on start it double-forks
// so a grandchild is orphaned onto the container's init (gapid), which
// must reap it - the kernel obligation the harness asserts.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

var (
	describe   = flag.Bool("describe", false, "Print agent metadata")
	start      = flag.Bool("start", false, "Run the agent (ADK launch contract)")
	middle     = flag.Bool("middle", false, "internal: the disappearing parent")
	grandchild = flag.Bool("grandchild", false, "internal: the orphan")
)

func main() {
	flag.Parse()

	switch {
	case *describe:
		metadata := map[string]interface{}{
			"describe": map[string]interface{}{
				"id":           "orphanmaker",
				"type":         "service",
				"version":      "1.0.0",
				"language":     "go",
				"description":  "pid1 e2e fixture: orphans a grandchild onto init",
				"capabilities": []string{"start", "stop"},
			},
		}
		if err := json.NewEncoder(os.Stdout).Encode(metadata); err != nil {
			fmt.Fprintln(os.Stderr, "encode describe metadata:", err)
			os.Exit(1)
		}

	case *middle:
		// Spawn the grandchild and die immediately: the grandchild's
		// parent is gone, so it reparents to the subreaper (init).
		self, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "resolve self:", err)
			os.Exit(1)
		}
		cmd := exec.Command(self, "--grandchild")
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "spawn grandchild:", err)
			os.Exit(1)
		}
		os.Exit(0)

	case *grandchild:
		// Give reparenting a beat, then exit with a recognizable status
		// the reaper's log line will carry (7 -> wait_status 1792).
		time.Sleep(300 * time.Millisecond)
		os.Exit(7)

	case *start:
		fmt.Println("[orphanmaker] started")
		self, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "resolve self:", err)
			os.Exit(1)
		}
		mid := exec.Command(self, "--middle")
		if err := mid.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "run middle:", err)
			os.Exit(1)
		}
		// Service semantics: stay alive until stopped.
		for {
			time.Sleep(time.Hour)
		}

	default:
		fmt.Fprintln(os.Stderr, "orphanmaker: no mode flag given")
		os.Exit(2)
	}
}
