// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk

import (
	"fmt"
	"sync"
	"time"
)

// ChannelManager manages a string channel for native communication between Go and Python.
// It is designed to be exposed via gopy.
type ChannelManager struct {
	ch   chan string
	mu   sync.Mutex
	done bool
}

// NewChannelManager creates a new manager.
func NewChannelManager() *ChannelManager {
	return &ChannelManager{
		// Buffer size could be configurable, but 100 is reasonable for now.
		ch: make(chan string, 100),
	}
}

// Send sends a message to the channel.
// It is safe to call from multiple goroutines (or Python threads).
func (m *ChannelManager) Send(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.done {
		m.ch <- msg
	}
}

// Receive blocks until a message is received or timeout.
// Returns the message and an error.
// If the channel is closed, returns error "channel closed".
// If timeout occurs, returns error "timeout".
func (m *ChannelManager) Receive(timeoutSeconds int) (string, error) {
	select {
	case msg, ok := <-m.ch:
		if !ok {
			return "", fmt.Errorf("channel closed")
		}
		return msg, nil
	case <-time.After(time.Duration(timeoutSeconds) * time.Second):
		return "", fmt.Errorf("timeout")
	}
}

// Close closes the channel.
func (m *ChannelManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.done {
		close(m.ch)
		m.done = true
	}
}
