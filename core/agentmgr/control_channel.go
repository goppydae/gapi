// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"google.golang.org/protobuf/encoding/protodelim"

	"github.com/goppydae/gapi/internal/logattr"
	gapiv1 "github.com/goppydae/gapi/pkg/proto"
)

// controlSchemaVersion is the frame contract this supervisor knows.
//
// Checked rather than assumed: a version field no consumer reads
// documents nothing and prevents nothing, which is the standard
// AgentDescriptor's schema_version is already held to.
const controlSchemaVersion = 1

// controlPipe is the supervisor's end of an agent's control channel.
//
// The agent gets the write end as an inherited descriptor and the
// supervisor keeps the read end. Both are closed by the supervisor: the
// write end immediately after exec, or the reader never sees EOF when
// the child dies and the reading goroutine leaks for the daemon's life.
type controlPipe struct {
	r *os.File
	w *os.File
}

func newControlPipe() (*controlPipe, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create control pipe: %w", err)
	}
	return &controlPipe{r: r, w: w}, nil
}

// statusPublisher is what a control reader needs from an agent, and all
// it needs: the two publications an agent-originated frame can produce.
//
// An interface rather than a concrete agent because GoAgent and
// PythonAgent differ in nothing that matters here - the frames are
// identical, which is the parity decision 37 buys by construction
// instead of by test.
type statusPublisher interface {
	publishStatusWithRunID(state, message, runID string)
	publishHeartbeat()
}

// readControl consumes an agent's typed lifecycle frames until EOF.
//
// GAPI-DIV-099: this REPLACES reading state off the child's stdout. The
// supervisor used to scan stdout for JSON and switch on an "event" key,
// which meant one stream carried both a protocol and arbitrary program
// output and which one a line WAS depended on whether it happened to
// parse. Nothing about that guess survives here: a frame is a frame, and
// stdout is now only ever logs.
//
// THE TOPIC DERIVES FROM THE ARM, which is GAPI-DIV-087's clause with
// its owner changed by decision 37 - the supervisor publishes now, so
// the supervisor is what chooses the topic.
// NO CONTEXT PARAMETER, deliberately. The reader ends when the pipe
// reaches EOF, which happens when the child exits and the supervisor's
// copy of the write end is closed - so its lifetime is the run's
// lifetime, exactly. The start context would be the wrong one: it bounds
// the START OPERATION and is done the moment Start returns
// (GAPI-DIV-028), which would cut the channel while the agent runs.
func readControl(r io.Reader, a statusPublisher, id string, log *slog.Logger) {
	br := bufio.NewReader(r)

	for {
		var frame gapiv1.AgentControl
		if err := protodelim.UnmarshalFrom(br, &frame); err != nil {
			// EOF is the ordinary end of a run: the child exited and its
			// write end closed. Anything else is a malformed stream, and
			// it is reported rather than swallowed - a control channel
			// that goes quiet for an unreadable reason is exactly the
			// silence GAPI-DIV-100 cost a day to.
			if !errors.Is(err, io.EOF) {
				log.Error("agent control stream failed",
					logattr.AgentID(id), logattr.Err(err))
			}
			return
		}

		if frame.GetSchemaVersion() != controlSchemaVersion {
			log.Error("refusing agent control frame of unknown schema version",
				logattr.AgentID(id))
			continue
		}

		switch ev := frame.GetEvent().(type) {
		case *gapiv1.AgentControl_Status:
			st := ev.Status
			if st.GetState() == "" {
				// An agent that announces a transition without naming the
				// state has announced nothing. Saying so beats publishing
				// a status the state machine will drop on arrival, which
				// is what the JSON path did for six of its eight events.
				log.Error("agent status frame carries no state", logattr.AgentID(id))
				continue
			}
			a.publishStatusWithRunID(st.GetState(), st.GetMessage(), st.GetRunId())

		case *gapiv1.AgentControl_Heartbeat:
			a.publishHeartbeat()

		default:
			// A frame whose arm this build does not know. The schema
			// version matched, so this is a producer bug rather than a
			// version skew, and it is worth a line either way.
			log.Warn("agent control frame carries no known event", logattr.AgentID(id))
		}
	}
}
