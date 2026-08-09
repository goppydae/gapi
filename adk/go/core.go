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
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goppydae/gapi/core/schemahash"
	"github.com/goppydae/gapi/internal/logattr"
	protopkg "github.com/goppydae/gapi/pkg/proto"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This package is designed to be bound to Python via gopy.
// It avoids channels in public signatures.

var (
	mu         sync.Mutex
	writeMu    sync.Mutex
	controlW   io.Writer
	schemaHash string
)

// envControlFD names the inherited descriptor the supervisor passes.
// Spelled as a literal for the same reason ADK_RUN_ID is: this package
// is the Python ADK's backend and imports nothing from the kernel.
const envControlFD = "ADK_CONTROL_FD"

// controlSchemaVersion is the frame contract's version.
const controlSchemaVersion = 1

// openControl opens the inherited control descriptor, once.
//
// StartQUIC used to live here, and it is GONE. Operator decision 37 puts
// the agent's control channel on a descriptor the supervisor passes at
// exec, so an agent neither dials nor needs an address - which is also
// why the hardcoded "127.0.0.1:14242" the runner fell back to has no
// successor. gopy binds every exported symbol in this package, so a
// symbol removed is a permanent cost removed.
func openControl() (io.Writer, error) {
	mu.Lock()
	defer mu.Unlock()

	if controlW != nil {
		return controlW, nil
	}
	raw, ok := os.LookupEnv(envControlFD)
	if !ok {
		return nil, fmt.Errorf("%s is unset, so this process was not started by a "+
			"supervisor that can hear it", envControlFD)
	}
	fd, err := strconv.Atoi(raw)
	if err != nil || fd < 0 {
		return nil, fmt.Errorf("%s=%q is not a descriptor number", envControlFD, raw)
	}
	controlW = os.NewFile(uintptr(fd), "control")
	return controlW, nil
}

// writeControl frames one message onto the channel.
func writeControl(msg *protopkg.AgentControl) {
	w, err := openControl()
	if err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelError,
			"control channel unavailable", logattr.Component("gapi-adk"), logattr.Err(err))
		return
	}

	writeMu.Lock()
	defer writeMu.Unlock()
	if _, err := protodelim.MarshalTo(w, msg); err != nil {
		slog.Default().LogAttrs(context.Background(), slog.LevelError,
			"control write failed", logattr.Component("gapi-adk"), logattr.Err(err))
	}
}

// Initialize sets up the agent identity.
func Initialize(name, version, typeStr string) {
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "initialized agent", logattr.Component("gapi-adk"), logattr.AgentID(name), logattr.Version(version), logattr.Type(typeStr))
}

// schemaHash defaults to the protobuf contract this binary was compiled
// against.
//
// COMPUTED AT INIT AND NOT BY A CALLER, which is the structural half of
// GAPI-DIV-127. The Python runner called SetSchemaHash and Go agents had
// no equivalent startup hook, so a Go agent reported the EMPTY STRING and
// any comparison was one the ecosystem's two agent kinds answered
// differently - the shape of gate that gets disabled rather than fixed.
//
// Adding a Go call site would have fixed that symptom and kept the
// mechanism that produced it: two languages, two opt-ins. Computing here
// removes the opt-in from both at once, because the Python ADK IS this
// package through gopy. There is one code path, so the two cannot
// disagree.
func init() {
	mu.Lock()
	defer mu.Unlock()
	schemaHash = schemahash.Contract()
}

// SchemaHash reports the protobuf contract this agent was compiled
// against. It is set before any agent code runs.
func SchemaHash() string {
	mu.Lock()
	defer mu.Unlock()
	return schemaHash
}

// SetSchemaHash overrides the computed contract hash.
//
// RETAINED ONLY AS A SEAM, and deliberately not called by either ADK.
// GAPI-DIV-127 closes on a DELIBERATE mismatch being detected, and a
// test that can only produce matching hashes passes when both sides
// compute nothing - so something has to be able to lie. A caller in
// shipping code would be reintroducing the defect: whatever it set would
// no longer be the contract the binary was built against.
func SetSchemaHash(hash string) {
	mu.Lock()
	defer mu.Unlock()
	schemaHash = hash
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "schema hash set", logattr.Component("gapi-adk"), logattr.Hash(hash))
}

// SendEvent reports one lifecycle transition.
//
// TYPED, and that is GAPI-DIV-087's whole content. It used to take a
// JSON STRING, unmarshal it into a map, pluck event/id/state/run_id by
// key and rebuild a LifecycleStatus - a value encoded, decoded and
// re-encoded before it left the process, with the event name extracted
// and then discarded. The caller now passes what it means.
//
// THE CALLER SETS THE STATE. The Python runner used to pass one on two
// of its eight notifications, so six transitions reached the supervisor
// with an empty state and were dropped on arrival.
func SendEvent(agentID, state, message string) {
	writeControl(&protopkg.AgentControl{
		SchemaVersion: controlSchemaVersion,
		Event: &protopkg.AgentControl_Status{
			Status: &protopkg.LifecycleStatus{
				AgentId:    agentID,
				State:      strings.ToUpper(strings.TrimSpace(state)),
				Message:    message,
				Time:       timestamppb.Now(),
				SchemaHash: schemaHash,
				RunId:      os.Getenv("ADK_RUN_ID"),
			},
		},
	})
}

// A supervisor-to-agent command channel used to live here as
// AwaitCommand and InjectCommand over an in-process mailbox. Nothing
// drove it: InjectCommand was its only producer, described in its own
// comment as a testing helper, and neither had a caller in either repo -
// while AwaitCommand's comment conceded that a real implementation
// "would read from a QUIC stream or IPC socket".
//
// It is removed rather than left in place because gopy binds every
// exported symbol in this package into the Python extension, so the
// surface here is a constraint rather than a convenience, and these two
// spent slots of it on a mechanism that did nothing. Worse, being
// Python-visible, an agent author could call AwaitCommand and block
// forever (GAPI-DIV-088).
//
// A command channel is the inverse of the event path in SendEvent and
// belongs on the same transport carrying the same schema. Design it;
// do not restore this.

// StartHeartbeat reports liveness on a fixed cadence.
//
// The JSON this used to build with fmt.Sprintf had NO ESCAPING, so an
// agent id containing a quote emitted malformed JSON every five seconds
// (GAPI-DIV-087). A typed message cannot be malformed by its own
// contents, which is the point: the defect class disappears rather than
// being fixed case by case.
func StartHeartbeat(agentID string) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			writeControl(&protopkg.AgentControl{
				SchemaVersion: controlSchemaVersion,
				Event: &protopkg.AgentControl_Heartbeat{
					Heartbeat: &protopkg.Heartbeat{
						AgentId: agentID,
						RunId:   os.Getenv("ADK_RUN_ID"),
					},
				},
			})
		}
	}()
}
