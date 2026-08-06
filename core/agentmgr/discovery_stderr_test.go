// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/eventbus"
)

// TestDiscovery_ReportsDiagnosticsFromASuccessfulDescribe is
// GAPI-DIV-094's gate.
//
// EVERY FAILURE PATH IN pythonDescribe PUTS STDERR IN ITS ERROR; THE
// SUCCESS PATH READ THE BUFFER AND DROPPED IT. So the runner's
// diagnostics were legible only when something else had already gone
// wrong - and discovery then registered the agent, reported "discovery
// complete", and looked healthy.
//
// The fixture writes at module scope, which the runner redirects to
// stderr while importing (load_module wraps the import in
// redirect_stdout(sys.stderr)). That is the ordinary case rather than a
// contrived one: an agent whose module prints anything at import time
// produces exactly this, and until now none of it was ever seen.
//
// GAPI-DIV-086 took the stub warning off this path in production by
// setting ADK_REJECT_DUMMY, which is why this test does not use the stub
// to demonstrate the loss. The entry stands because the NEXT diagnostic
// will not be about the stub.
func TestDiscovery_ReportsDiagnosticsFromASuccessfulDescribe(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process execution test in short mode")
	}

	dir := t.TempDir()
	chatty := filepath.Join(dir, "chatty.py.service")
	body := "ID = \"chatty\"\n" +
		"TYPE = \"service\"\n" +
		"print(\"import-time chatter discovery used to discard\")\n" +
		"def start():\n    pass\n"
	if err := os.WriteFile(chatty, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	sink := captureDiscoveryLogs(t)

	// A REAL BUS, unlike the sibling tests in discovery_silence_test.go.
	// Those pass nil because every one of their fixtures FAILS to
	// describe, so discovery never registers anything. This one succeeds
	// by design, and registration builds a lifecycle.Controller that
	// subscribes - which panics on a nil bus, since Subscribe has no
	// nil guard where Publish does.
	bus := eventbus.NewInprocBus[*anypb.Any]()
	am := NewAgentManager(bus, nil, "../../adk/python/agent/runner.py", false, nil)
	agents, err := am.discoverFromSinglePath(dir, config.PathTypeSystem)
	if err != nil {
		t.Fatalf("discoverFromSinglePath: %v", err)
	}
	// The premise: this agent describes SUCCESSFULLY. If it did not, the
	// error path would carry the stderr and this test would be asserting
	// something else entirely.
	if len(agents) != 1 {
		t.Fatalf("fixture did not describe successfully: discovered %d agents", len(agents))
	}

	if !mentions(sink.records(), slog.LevelDebug, "chatty.py.service", "import-time chatter") {
		t.Error("a successful describe wrote diagnostics and discovery discarded them; " +
			"the agent registers and the node looks healthy")
	}
}
