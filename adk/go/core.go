// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/transport"
	"github.com/goppydae/gapi/internal/logattr"
	"github.com/goppydae/gapi/internal/safeio"
	protopkg "github.com/goppydae/gapi/pkg/proto"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/types/known/anypb"
)

// This package is designed to be bound to Python via gopy.
// It avoids channels in public signatures.

var (
	mu sync.Mutex
	// Simple command mailbox
	cmdMailbox struct {
		cond *sync.Cond
		cmd  string // e.g. "START", "STOP"
	}
	quicClient *transport.QUIC
	schemaHash string
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

	// Connect to local GAPI server
	// TODO: Support secure connections for remote agents
	tlsCfg := transport.TLSConfig{InsecureSkipVerify: true}
	agent, err := transport.NewQUICClient(addr, nil, tlsCfg)
	if err != nil {
		return fmt.Errorf("failed to start QUIC client: %w", err)
	}
	quicClient = agent
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "connected to supervisor", logattr.Component("gapi-adk"), logattr.Addr(addr))
	return nil
}

// Initialize sets up the agent identity.
func Initialize(name, version, typeStr string) {
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "initialized agent", logattr.Component("gapi-adk"), logattr.AgentID(name), logattr.Version(version), logattr.Type(typeStr))
}

// SetSchemaHash sets the schema hash for the agent.
func SetSchemaHash(hash string) {
	mu.Lock()
	defer mu.Unlock()
	schemaHash = hash
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "schema hash set", logattr.Component("gapi-adk"), logattr.Hash(hash))
}

// ComputeSchemaHash reads a file and returns its BLAKE3 hash as a hex string.
// Returns empty string on error to simplify binding logic.
func ComputeSchemaHash(path string) string {
	data, err := safeio.ReadFile(path)
	if err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelError, "failed to read file for hashing", logattr.Component("gapi-adk"), logattr.Path(path), logattr.Err(err))
		return ""
	}
	hash := blake3.Sum256(data)
	return fmt.Sprintf("%x", hash)
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
		slog.Default().LogAttrs(context.Background(), slog.LevelError, "sendevent json marshal failed", logattr.Component("gapi-adk"), logattr.Err(err))
		return
	}

	event, _ := data["event"].(string)
	_ = event // Topic logic uses fixed string for now
	id, _ := data["id"].(string)
	stateRaw, _ := data["state"].(string)
	runID, _ := data["run_id"].(string)

	// Map to LifecycleStatus. run_id travels as the structural field,
	// never inside the message text (R16).
	stat := &protopkg.LifecycleStatus{
		AgentId:    id,
		State:      strings.ToUpper(stateRaw),
		Message:    jsonStr,
		SchemaHash: schemaHash,
		RunId:      runID,
	}

	anyStat, err := anypb.New(stat)
	if err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelError, "proto marshal failed", logattr.Component("gapi-adk"), logattr.Err(err))
		return
	}

	ev := eventbus.Event[*anypb.Any]{
		ID:      id,
		Topic:   eventbus.TopicAgentLifecycleStatus,
		Payload: anyStat,
		Source:  "agent:" + id,
	}

	if err := quicClient.PublishRemote(context.Background(), ev); err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelError, "quic publish failed", logattr.Component("gapi-adk"), logattr.Err(err))
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
