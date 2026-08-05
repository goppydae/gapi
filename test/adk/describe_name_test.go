// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk

import (
	"encoding/json"
	"os/exec"
	"testing"
)

// describedIdentity runs --describe against a Python fixture and returns
// the id and name it reported.
//
// It drives the real invocation - python3, the runner, --module - rather
// than calling describe() in-process, because the defect this guards
// lives in the runner and the payload is what every consumer reads.
func describedIdentity(t *testing.T, module string) (id, name string) {
	t.Helper()

	runner := findPythonRunner(t)
	out, err := exec.Command("python3", runner, "--module", module, "--describe").CombinedOutput()
	if err != nil {
		t.Fatalf("--describe on %s: %v\n%s", module, err, out)
	}

	var payload struct {
		Describe struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"describe"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("parsing --describe output for %s: %v\n%s", module, err, out)
	}
	return payload.Describe.ID, payload.Describe.Name
}

// TestDescribe_ReportsTheDeclaredName is the regression for
// GAPI-DIV-081.
//
// describe() binds the name from the metadata aliases and the capability
// scan then reuses the same identifier as its loop variable, so the
// assembled payload carries the LAST member inspect.getmembers returned
// instead of the declared name. For this fixture that is "stop".
//
// The bug survived because nothing reads name: both producers emit it,
// the cross-ADK suite asserts only that the key EXISTS, and no live
// consumer reads the value.
func TestDescribe_ReportsTheDeclaredName(t *testing.T) {
	id, name := describedIdentity(t, "fixtures/python/named_agent.py")

	if id != "named_agent" {
		t.Fatalf("id = %q, want %q - the fixture is not the one under test", id, "named_agent")
	}
	if name != "Named Agent" {
		t.Errorf("name = %q, want %q: the declared name did not survive capability derivation", name, "Named Agent")
	}
}

// TestDescribe_DefaultsNameToID covers the other half. An agent that
// declares no NAME must report its id, which is what get_meta's default
// says and what every fixture but one relies on.
func TestDescribe_DefaultsNameToID(t *testing.T) {
	id, name := describedIdentity(t, "fixtures/python/lifecycle_agent.py")

	if id != "lifecycle_agent" {
		t.Fatalf("id = %q, want %q - the fixture is not the one under test", id, "lifecycle_agent")
	}
	if name != id {
		t.Errorf("name = %q, want the id %q: an agent declaring no NAME reported something else entirely", name, id)
	}
}
