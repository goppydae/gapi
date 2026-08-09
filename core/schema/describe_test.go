// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"encoding/json"
	"testing"
)

// THE POINT OF ONE DECLARATION IS THAT NOTHING IS DROPPED IN TRANSIT
// (GAPI-DIV-115).
//
// This shape used to be spelled three times in Go - twice in
// core/agentmgr/discovery.go and once here - with a field-by-field copy
// between the parsed struct and the validated one. The copy omitted
// SchemaVersion, so a field BOTH ADKs emit and operator decision 29
// calls load-bearing was discarded at the boundary and the validator's
// field was populated by nobody.
//
// The assertion is deliberately field-by-field rather than a reflect
// count: a test that only counted would pass against a struct that
// parsed the right NUMBER of wrong names.
func TestDescribeEnvelopeParsesEveryDeclaredField(t *testing.T) {
	// The object the ADKs actually print. adk/go/agent/describe.go and
	// adk/python/agent/runner.py both emit schema_version at this level.
	const raw = `{
	  "describe": {
	    "schema_version": "1.0.0",
	    "id": "sample_agent",
	    "type": "service",
	    "cpu_limit": "0.5",
	    "memory_limit": "100MB",
	    "schedule": "OnUnitActiveSec=30s",
	    "listen_stream": "0.0.0.0:8080",
	    "requires": ["dep_a"],
	    "wants": ["dep_b"],
	    "wanted_by": ["dep_c"],
	    "required_by": ["dep_d"],
	    "capabilities": ["cap_a"],
	    "enabled": false,
	    "schema_hash": "abc123"
	  }
	}`

	var env DescribeEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal describe envelope: %v", err)
	}
	d := env.Describe

	// SchemaVersion FIRST, because it is the one the duplicated structs
	// silently dropped and the only one whose loss was invisible.
	if d.SchemaVersion != "1.0.0" {
		t.Errorf("schema_version = %q, want %q - both ADKs emit it and it must survive parsing", d.SchemaVersion, "1.0.0")
	}

	for _, tc := range []struct {
		field, got, want string
	}{
		{"id", d.ID, "sample_agent"},
		{"type", d.Type, "service"},
		{"cpu_limit", d.CPULimit, "0.5"},
		{"memory_limit", d.MemoryLimit, "100MB"},
		{"schedule", d.Schedule, "OnUnitActiveSec=30s"},
		{"listen_stream", d.ListenStream, "0.0.0.0:8080"},
		{"schema_hash", d.SchemaHash, "abc123"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}

	for _, tc := range []struct {
		field string
		got   []string
		want  string
	}{
		{"requires", d.Requires, "dep_a"},
		{"wants", d.Wants, "dep_b"},
		{"wanted_by", d.WantedBy, "dep_c"},
		{"required_by", d.RequiredBy, "dep_d"},
		{"capabilities", d.Capabilities, "cap_a"},
	} {
		if len(tc.got) != 1 || tc.got[0] != tc.want {
			t.Errorf("%s = %v, want [%q]", tc.field, tc.got, tc.want)
		}
	}

	// Explicit false must be distinguishable from absent.
	if d.Enabled == nil {
		t.Fatal("enabled = nil for an explicit false; the pointer exists to tell those apart")
	}
	if *d.Enabled {
		t.Error("enabled = true, want false")
	}
}

// A Go agent emits no "enabled" key at all. Absent must stay absent, or
// honouring the field turns every Go agent off.
func TestAbsentEnabledStaysNil(t *testing.T) {
	var env DescribeEnvelope
	if err := json.Unmarshal([]byte(`{"describe":{"id":"a","type":"service"}}`), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Describe.Enabled != nil {
		t.Errorf("enabled = %v for an absent key, want nil", *env.Describe.Enabled)
	}
}

// The struct discovery parses IS the struct the validator takes. When
// they were two types joined by a hand-written copy, a field could be
// parsed and never validated; this asserts the single path.
func TestParsedDescribeValidatesDirectly(t *testing.T) {
	var env DescribeEnvelope
	if err := json.Unmarshal([]byte(`{"describe":{"id":"a","type":"service","cpu_limit":"0.5"}}`), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := ValidateAgentDescribe(env.Describe); err != nil {
		t.Fatalf("ValidateAgentDescribe on the parsed struct: %v", err)
	}
}

// TestDescribeEnvelopeCarriesTheSchemaHash covers GAPI-DIV-127's
// registration path. Both ADKs emit the key and discovery has ONE
// decoder for both, so a field that fails to survive parsing here is a
// mismatch detector that receives nothing - which is indistinguishable
// from a fleet with no skew in it.
func TestDescribeEnvelopeCarriesTheSchemaHash(t *testing.T) {
	const raw = `{"describe":{"schema_version":"1.0.0","id":"a","type":"service",` +
		`"schema_hash":"abc123"}}`

	var env DescribeEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Describe.SchemaHash != "abc123" {
		t.Fatalf("schema_hash = %q, want %q", env.Describe.SchemaHash, "abc123")
	}
}

// TestDescribeEnvelopeToleratesNoSchemaHash is the compatibility floor,
// and it is a contract requirement rather than politeness.
//
// An agent built before this field existed must still register.
// Operator decision 71 makes the hash a diagnostic and never an
// enforcement input, and refusing to parse an agent that cannot answer
// would be enforcement wearing a parser's clothes - it would also flag
// every older agent in a fleet on the first upgrade.
func TestDescribeEnvelopeToleratesNoSchemaHash(t *testing.T) {
	const raw = `{"describe":{"schema_version":"1.0.0","id":"a","type":"service"}}`

	var env DescribeEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("an agent predating schema_hash must still parse: %v", err)
	}
	if env.Describe.SchemaHash != "" {
		t.Fatalf("absent schema_hash = %q, want empty", env.Describe.SchemaHash)
	}
}
