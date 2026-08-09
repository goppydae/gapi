// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agent

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"

	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/goppydae/gapi/core/schemahash"
	gapiv1 "github.com/goppydae/gapi/pkg/proto"
)

// controlSchemaVersion is the frame contract's version. A supervisor
// that does not know it refuses the frame and says so, rather than
// reading fields that may have moved.
const controlSchemaVersion = 1

// envControlFD names the inherited descriptor the supervisor passes.
//
// Spelled here as a literal rather than imported from core/agentmgr:
// this package imports nothing from the kernel by design, and ADK_RUN_ID
// is already read the same way. The supervisor's side declares it in
// agent_env.go, which is where the contract is documented.
const envControlFD = "ADK_CONTROL_FD"

// Lifecycle states, spelled as the supervisor's state machine spells
// them. THE AGENT SETS THE STATE, which is the substance of
// GAPI-DIV-087: an event name that the supervisor had to translate into
// a state is how six of eight transitions came to carry no state at all
// and be dropped on arrival.
const (
	statePending = "PENDING"
	stateRunning = "RUNNING"
	stateStopped = "STOPPED"
	stateFailed  = "FAILED"
)

// control writes typed lifecycle frames to the supervisor.
//
// OPERATOR DECISION 37: the channel is an inherited file descriptor, not
// stdout and not a transport the agent dials. Two consequences, both
// load-bearing. Stdout carries log lines and only log lines, so no
// stream has to guess which of the two it is holding - which is what
// decision 33 was really about, and what the deleted stdout fence
// existed to paper over. And the descriptor exists BEFORE the process
// starts, so there is no pre-connect window and no event that has
// nowhere to go: GAPI-DIV-087's buffer-and-flush clause has no referent
// here and was deliberately not built.
//
// OPERATOR DECISION 38: the frames are protobuf. The channel crosses a
// process boundary, and the cooperative-ecosystem manifesto's FFI
// exception is named precisely so that such a boundary does not get a
// second one.
type control struct {
	mu    sync.Mutex
	w     io.Writer
	id    string
	runID string
}

// newControl opens the inherited control descriptor.
//
// A missing or unusable descriptor is an ERROR, not a fallback to
// stdout. A fallback is how the supervisor came to read protocol off a
// stream that also carries arbitrary program output, and an agent that
// cannot report its state has not started successfully - it has started
// invisibly, which is worse.
func newControl(id string) (*control, error) {
	raw, ok := os.LookupEnv(envControlFD)
	if !ok {
		return nil, fmt.Errorf(
			"adk/agent: %s is unset, so this process was not started by a supervisor "+
				"that can hear it", envControlFD)
	}
	fd, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("adk/agent: %s=%q is not a descriptor number: %w", envControlFD, raw, err)
	}
	if fd < 0 {
		return nil, fmt.Errorf("adk/agent: %s=%d is not a descriptor number", envControlFD, fd)
	}

	return &control{
		w:     os.NewFile(uintptr(fd), "control"),
		id:    id,
		runID: os.Getenv("ADK_RUN_ID"),
	}, nil
}

// newControlTo sinks frames into w. Tests only.
func newControlTo(w io.Writer, id string) *control {
	return &control{w: w, id: id, runID: os.Getenv("ADK_RUN_ID")}
}

// status reports a lifecycle transition the agent chose to announce.
func (c *control) status(state, message string) {
	c.write(&gapiv1.AgentControl{
		SchemaVersion: controlSchemaVersion,
		Event: &gapiv1.AgentControl_Status{
			Status: &gapiv1.LifecycleStatus{
				AgentId: c.id,
				State:   state,
				Message: message,
				Time:    timestamppb.Now(),
				RunId:   c.runID,
				// The contract this binary was compiled against
				// (GAPI-DIV-127). The Python path sets it too; a field one
				// language populates and the other does not is the state
				// that entry was filed against, and it had reappeared here
				// one message below where it was first found.
				SchemaHash: schemahash.Contract(),
			},
		},
	})
}

// write frames one message onto the channel.
//
// protodelim, not a hand-rolled length prefix: the stream needs message
// boundaries, and a framing written twice - once here and once in the
// reader - is a framing that can disagree with itself.
//
// A failed write is not recoverable here, but it must not be silent. An
// agent stuck in PENDING because its report was lost looks exactly like
// an agent that never reached ready, and the two want different
// remedies.
func (c *control) write(msg *gapiv1.AgentControl) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := protodelim.MarshalTo(c.w, msg); err != nil {
		fmt.Fprintf(os.Stderr, "adk/agent: control write failed: %v\n", err)
	}
}
