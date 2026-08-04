// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
)

// ENABLED was parsed and discarded: the field had zero consumers, so
// 'ENABLED = False' did nothing while three docs told operators it
// disabled an agent (GAPI-DIV-034).
//
// The dangerous part of honouring it is the DEFAULT. Go agents do not
// emit "enabled" at all, so a plain bool would unmarshal their silence
// as false and turn every Go agent off. The field is therefore a
// pointer, and these tests pin that distinction.

func TestAbsentEnabledMeansEnabled(t *testing.T) {
	// Exactly what a Go agent's --describe emits: no "enabled" key.
	var d struct {
		Enabled *bool `json:"enabled"`
	}
	body := `{"id":"a","type":"service","language":"go"}`
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Enabled != nil {
		t.Fatal("absent enabled decoded to a non-nil pointer")
	}
	if resolved := d.Enabled == nil || *d.Enabled; !resolved {
		t.Fatal("an agent that does not mention ENABLED was treated as disabled; " +
			"this would turn off every Go agent")
	}
}

func TestExplicitFalseDisables(t *testing.T) {
	var d struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(`{"enabled":false}`), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Enabled == nil {
		t.Fatal("explicit false decoded as absent")
	}
	if resolved := d.Enabled == nil || *d.Enabled; resolved {
		t.Fatal("an explicit ENABLED = False did not disable the agent")
	}
}

// Every runner must carry the flag, and must default to enabled when
// built directly - a constructor that zero-valued it would make agents
// silently un-startable.
func TestAllRunnersDefaultToEnabled(t *testing.T) {
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
		if !AgentEnabled(a) {
			t.Errorf("%s: freshly constructed runner is disabled", name)
		}
		es, ok := a.(enabledSetter)
		if !ok {
			t.Errorf("%s: does not implement enabledSetter, so discovery cannot disable it", name)
			continue
		}
		es.SetEnabled(false)
		if AgentEnabled(a) {
			t.Errorf("%s: SetEnabled(false) did not take effect", name)
		}
		es.SetEnabled(true)
		if !AgentEnabled(a) {
			t.Errorf("%s: SetEnabled(true) did not restore it", name)
		}
	}
}

// A type that knows nothing about the flag counts as enabled. The safe
// direction: the alternative is a runner that silently never starts.
func TestUnknownTypeCountsAsEnabled(t *testing.T) {
	if !AgentEnabled(nil) {
		t.Fatal("an agent that does not carry the flag was treated as disabled")
	}
}
