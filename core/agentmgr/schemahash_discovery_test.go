// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"testing"

	"github.com/goppydae/gapi/core/eventbus"
	"github.com/goppydae/gapi/core/schema"
	"google.golang.org/protobuf/types/known/anypb"
)

// TestEveryRunnerCarriesTheSchemaHash pins the hand-off between the
// decoded envelope and the map setupAgents reads.
//
// EVERY AGENT KIND, in the shape TestAllRunnersDefaultToEnabled uses for
// the same reason. The three Describe() implementations are separate
// copies of one shape, so a field added to the struct and forgotten in
// one of them reaches the registry for two kinds and not the third - and
// a detector covering two thirds of a fleet reads as coverage while the
// gap is invisible (GAPI-DIV-127).
//
// The setter is on the Agent interface rather than an optional
// capability for the same reason: a future agent kind that omits it
// fails to compile, instead of reporting an empty hash forever.
func TestEveryRunnerCarriesTheSchemaHash(t *testing.T) {
	const want = "abc123"

	bus := eventbus.NewInprocBus[*anypb.Any]()
	dep := NewMockDependencyResolver()

	runners := map[string]Agent{
		"GoAgent": NewGoAgent("go-1", "service", "/bin/true",
			nil, nil, nil, nil, "", "", "", nil, bus, dep),
		"PythonAgent": NewPythonAgent("py-1", "service", "/tmp/x.py", "python3",
			nil, nil, nil, nil, "", "", "", nil, bus, dep, false),
		"TimerAgent": NewTimerAgent("t-1", "/tmp/x.py", "OnUnitActiveSec=60s",
			"python3", bus, eventbus.NewInprocBus[*anypb.Any]()),
	}

	for name, a := range runners {
		t.Run(name, func(t *testing.T) {
			a.setSchemaHash(want)
			if got := a.Describe()["schema_hash"]; got != want {
				t.Fatalf("%s Describe()[schema_hash] = %q, want %q", name, got, want)
			}
		})
	}
}

// TestARunnerWithNoHashDescribesEmpty is the compatibility half.
//
// An agent predating the field reports the empty string, and the map
// must carry that faithfully rather than substituting a value. The
// daemon reads empty as "cannot answer" and stays silent, which is what
// keeps an entire older fleet from being flagged on the first upgrade -
// operator decision 71 makes this a diagnostic, and a diagnostic that
// fires on everything is noise.
func TestARunnerWithNoHashDescribesEmpty(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	dep := NewMockDependencyResolver()

	a := NewGoAgent("go-1", "service", "/bin/true",
		nil, nil, nil, nil, "", "", "", nil, bus, dep)

	if got := a.Describe()["schema_hash"]; got != "" {
		t.Fatalf("a runner that reported no hash describes %q, want empty", got)
	}
}

// TestDiscoveryCarriesTheReportedHashOntoTheAgent covers the CALL SITE,
// and it exists because the two cases above did not.
//
// Neutering `a.setSchemaHash(meta.SchemaHash)` in processDiscovered left
// this package entirely green: those cases drive the setter directly, so
// they prove the plumbing exists and say nothing about whether discovery
// uses it. A test that survives deletion of what it tests is not a test.
//
// processDiscovered had no coverage of any kind before this.
func TestDiscoveryCarriesTheReportedHashOntoTheAgent(t *testing.T) {
	const reported = "c0ffee1234567890"

	am := NewAgentManager(eventbus.NewInprocBus[*anypb.Any](), nil,
		"../../adk/python/agent/runner.py", false, nil)

	var agents []Agent
	err := am.processDiscovered("/tmp/probe.go.service", schema.AgentDescribe{
		SchemaVersion: "1.0.0",
		ID:            "probe",
		Type:          "service",
		SchemaHash:    reported,
	}, &agents)
	if err != nil {
		t.Fatalf("processDiscovered: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("want exactly one agent, got %d", len(agents))
	}

	if got := agents[0].Describe()["schema_hash"]; got != reported {
		t.Fatalf("discovery dropped the reported contract: describe has %q, "+
			"the agent reported %q", got, reported)
	}
}

// TestDiscoveryReportsEmptyForAnAgentThatSentNoHash keeps the
// compatibility path honest at the same seam: discovery must not invent
// a value for an agent predating the field.
func TestDiscoveryReportsEmptyForAnAgentThatSentNoHash(t *testing.T) {
	am := NewAgentManager(eventbus.NewInprocBus[*anypb.Any](), nil,
		"../../adk/python/agent/runner.py", false, nil)

	var agents []Agent
	err := am.processDiscovered("/tmp/probe.go.service", schema.AgentDescribe{
		SchemaVersion: "1.0.0",
		ID:            "probe",
		Type:          "service",
	}, &agents)
	if err != nil {
		t.Fatalf("processDiscovered: %v", err)
	}
	if got := agents[0].Describe()["schema_hash"]; got != "" {
		t.Fatalf("discovery invented %q for an agent that sent no hash", got)
	}
}
