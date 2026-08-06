// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"time"

	adk "github.com/goppydae/gapi/adk/go"
)

func main() {
	fmt.Println("Event emitter starting...")
	adk.Initialize("event_emitter", "1.0.0", "service")

	// Simple loop to keep alive and emit events if needed
	go func() {
		for {
			time.Sleep(1 * time.Second)
			adk.SendEvent("event_emitter", "RUNNING", "ping")
		}
	}()

	// Wait for shutdown
	select {}
}
