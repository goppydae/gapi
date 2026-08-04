// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agent

import (
	"os"
	"sync"
)

// The control channel is the process's ORIGINAL stdout, captured once
// before the fence redirects os.Stdout at the author's code.
//
// Capturing it at first use rather than in an init() keeps the ordering
// explicit: fenceStdout must not be able to run before the original has
// been taken, and a package-level initializer would make that a fact
// about import order instead of about this file.
var (
	ctlOnce sync.Once
	ctlFile *os.File
)

func controlOut() *os.File {
	ctlOnce.Do(func() { ctlFile = os.Stdout })
	return ctlFile
}

// fenceStdout points os.Stdout at stderr and returns a function restoring
// it.
//
// stdout is the control channel: the supervisor's streamControl scans the
// child's stdout line by line and reads each JSON object's "event" key
// (core/agentmgr/go_agent.go). A stray fmt.Println in agent code either
// parses as an event the supervisor acts on, or lands as a log line that
// quietly displaces the protocol. The Python runner fences this the same
// way - redirect_stdout(sys.stderr) around the author's start - so both
// languages give an author the same guarantee.
//
// SCOPE, deliberately bounded: this moves the os.Stdout VARIABLE, which
// covers everything an author writes through the os and fmt packages. It
// does not move file descriptor 1, so cgo code writing to the descriptor
// directly still reaches the control channel. Doing that portably needs
// dup2, which is spelled differently on the platforms this repo's flake
// declares, and no agent in the tree uses cgo. An exec'd child is NOT a
// gap: os/exec connects a nil Stdout to /dev/null rather than inheriting
// the parent's descriptor, and a child given cmd.Stdout = os.Stdout gets
// the fenced value.
func fenceStdout() (func(), error) {
	// Take the control handle BEFORE anything moves, so events keep
	// going to the real stdout for the life of the process.
	controlOut()

	prev := os.Stdout
	os.Stdout = os.Stderr

	restored := false
	return func() {
		if restored {
			return
		}
		restored = true
		os.Stdout = prev
	}, nil
}
