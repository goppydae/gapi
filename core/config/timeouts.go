// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package config

import "time"

// QUIC Transport Timeouts
var (
	// QUICStreamTimeout is the max time for a single QUIC stream operation.
	// Rationale: Covers network latency + agent marshaling/unmarshaling (< 10s typical).
	QUICStreamTimeout = 10 * time.Second

	// QUICIdleTimeout is the connection idle timeout before automatic closure.
	// Rationale: Balances connection reuse vs resource cleanup (1 minute idle acceptable).
	QUICIdleTimeout = 60 * time.Second

	// PublishConfirmTimeout is how long PublishRemote waits for its
	// per-peer sends to confirm before reporting the publish
	// unconfirmed.
	//
	// IT IS CHOSEN, NOT DERIVED, and saying so is the point - nothing
	// here measured how long a healthy stream open and two writes take,
	// so there is no table to derive it from and labelling it derived
	// would be the cargo-culted constant GAPI-DIV-107 exists to
	// complain about.
	//
	// The two facts that chose it: it must be BELOW
	// ClientPendingTimeout, or a client's reply deadline expires before
	// its own publish admits it never sent, which is exactly the
	// symptom this bound exists to convert into an error; and it must
	// be well ABOVE a healthy send, which on a live connection is a
	// stream open and two buffered writes.
	//
	// IT BOUNDS THE WAIT AND NOT THE SEND. A send that outlives this
	// window keeps its QUICStreamTimeout and finishes or fails on its
	// own, so no send gets less time than it had before the window
	// existed - only the caller stops pretending it knows the outcome.
	PublishConfirmTimeout = 1 * time.Second
)

// Client Lifecycle Timeouts
var (
	// ClientPendingTimeout is the max wait for an agent to enter PENDING state.
	// Rationale: Fast-fail if supervisor doesn't acknowledge command quickly.
	ClientPendingTimeout = 2 * time.Second

	// ClientTerminalTimeout is the max wait for an agent to reach terminal state (RUNNING/STOPPED/FAILED).
	// Rationale: Covers Python startup, health checks, socket binding (< 20s for well-behaved agents).
	ClientTerminalTimeout = 20 * time.Second
)

// Supervisor Timeouts
var (
	// SupervisorStartDeadline is the max time for an agent to report ready after start command.
	// Rationale: Aligns with ClientTerminalTimeout to prevent supervisor-client mismatch.
	SupervisorStartDeadline = 20 * time.Second

	// SupervisorShutdownTimeout is the graceful shutdown timeout before force-kill.
	// Rationale: Allows agents to flush logs, close connections (5s is reasonable grace period).
	SupervisorShutdownTimeout = 5 * time.Second
)

// Test Timeouts (more generous for CI environments)
var (
	// TestAgentStartTimeout is the max time to wait for agent start in tests.
	// Rationale: CI environments may be slow; 2-minute buffer covers edge cases.
	TestAgentStartTimeout = 120 * time.Second

	// TestAgentStopTimeout is the max time to wait for agent stop in tests.
	// Rationale: Covers graceful shutdown + Python interpreter cleanup.
	TestAgentStopTimeout = 60 * time.Second
)
