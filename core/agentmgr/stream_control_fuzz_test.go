// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"bytes"
	"strings"
	"testing"
)

// The agent stdout decoder is the kernel's most exposed parser: it eats
// whatever a supervised process writes to its stdout, line by line, and
// tries to read each line as a JSON control message.
//
// GAPI-DIV-042 cites go_agent.go:581 and python_agent.go:610; those line
// numbers have drifted. The decode now lives inline in
// (*GoAgent).streamControl and (*PythonAgent).streamControl. Nothing is
// extracted here: streamControl already takes an io.Reader, so it is the
// smallest in-package function that does the decoding and it is
// reachable with no process, no pipe and no exec.
//
// The bus is left nil on purpose. publishLog/publishStatus/publishHeart-
// beat all return early on a nil bus, so the target stays deterministic:
// no event-bus goroutines (dispatch fans out with `go`), no global
// metrics, no log flood. A nil bus is a real configuration - NewGoAgent
// accepts one - so this also covers the nil-bus path itself.
//
// Invariants:
//
//   - totality: arbitrary bytes on a supervised process's stdout never
//     panic the supervisor and always terminate the reader loop;
//   - containment: the decoder is READ-ONLY with respect to supervisor
//     bookkeeping. Nothing a child process prints may mutate the fields
//     that track that process - identity, cmd, pipes, controller,
//     adopted pid, enabled bit. A child that could flip those by writing
//     to stdout would be able to talk the supervisor out of supervising
//     it.

// controlSeeds are shared by both runner targets: the wire protocol is
// the same and the two decoders are near-identical copies.
var controlSeeds = [][]byte{
	[]byte(`{"event":"ready"}`),                                     // valid control message
	[]byte(`{"event":"error","error":"boom"}`),                      // valid, carries a payload
	[]byte("{\"event\":\"starting\"}\n{\"event\":\"heartbeat\"}\n"), // multiple lines
	[]byte(""),                                               // empty stream
	[]byte("\n\n   \n\t\n"),                                  // blank lines only
	[]byte("plain text, not json at all\n"),                  // opaque log line
	[]byte(`{"event":123}`),                                  // known-nasty: event is not a string
	[]byte(`{"event":null,"error":{"a":[1,2]}}`),             // nested, null-typed
	[]byte(`null`),                                           // valid JSON, decodes to a nil map
	[]byte(`[1,2,3]`),                                        // valid JSON, wrong shape
	[]byte(`{"event":"ready"`),                               // truncated object
	[]byte("{\"event\":\"ready\"}\x00trailing"),              // embedded NUL
	[]byte("{\"event\":\"\xff\xfe invalid utf8\"}"),          // invalid UTF-8 in a string
	[]byte(strings.Repeat(`{"event":"heartbeat"}`+"\n", 64)), // burst
	[]byte(`{"event":"ready","event":"stopped"}`),            // duplicate keys
}

// runnerState is the supervisor bookkeeping the decoder must not touch.
type runnerState struct {
	id, typ, path, hostname string
	enabled                 bool
	adoptedPid              int
	hasCmd, hasCtrl         bool
	hasStdout, hasStderr    bool
}

func FuzzGoAgentStreamControl(f *testing.F) {
	for _, s := range controlSeeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		a := &GoAgent{
			id:       "fuzz-agent",
			typ:      "service",
			path:     "/nonexistent/fuzz",
			hostname: "fuzz-host",
			enabled:  true,
		}
		before := goAgentState(a)

		a.streamControl(bytes.NewReader(data))

		if after := goAgentState(a); after != before {
			t.Fatalf("stdout decoder mutated supervisor state: %+v -> %+v", before, after)
		}
	})
}

func FuzzPythonAgentStreamControl(f *testing.F) {
	for _, s := range controlSeeds {
		f.Add(s)
	}
	// The Python decoder recognizes one extra event the Go one does not.
	f.Add([]byte(`{"event":"reloaded"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		a := &PythonAgent{
			id:       "fuzz-agent",
			typ:      "service",
			path:     "/nonexistent/fuzz.py",
			hostname: "fuzz-host",
			enabled:  true,
		}
		before := pythonAgentState(a)

		a.streamControl(bytes.NewReader(data))

		if after := pythonAgentState(a); after != before {
			t.Fatalf("stdout decoder mutated supervisor state: %+v -> %+v", before, after)
		}
	})
}

func goAgentState(a *GoAgent) runnerState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return runnerState{
		id:         a.id,
		typ:        a.typ,
		path:       a.path,
		hostname:   a.hostname,
		enabled:    a.enabled,
		adoptedPid: a.adoptedPid,
		hasCmd:     a.cmd != nil,
		hasCtrl:    a.ctrl != nil,
		hasStdout:  a.stdout != nil,
		hasStderr:  a.stderr != nil,
	}
}

func pythonAgentState(a *PythonAgent) runnerState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return runnerState{
		id:        a.id,
		typ:       a.typ,
		path:      a.path,
		hostname:  a.hostname,
		enabled:   a.enabled,
		hasCmd:    a.cmd != nil,
		hasCtrl:   a.ctrl != nil,
		hasStdout: a.stdout != nil,
		hasStderr: a.stderr != nil,
	}
}
