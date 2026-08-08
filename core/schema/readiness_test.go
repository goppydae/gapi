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
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/budget"
)

// TestDeclaredReadinessBudgetRoundTrips is task 2's gate. The value an
// agent writes must survive the wire, the unmarshal and the parse - a
// field that parses but arrives as something else is the drift
// GAPI-DIV-115 was filed for.
func TestDeclaredReadinessBudgetRoundTrips(t *testing.T) {
	const raw = `{"describe":{"schema_version":"1.0.0","id":"slow-svc","type":"service","readiness_budget":"45s"}}`

	var env DescribeEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Describe.ReadinessBudget == nil {
		t.Fatal("readiness_budget was declared and arrived nil: the field is not being parsed")
	}
	if got := *env.Describe.ReadinessBudget; got != "45s" {
		t.Fatalf("readiness_budget = %q, want %q", got, "45s")
	}

	got, err := ParseReadinessBudget(env.Describe)
	if err != nil {
		t.Fatalf("ParseReadinessBudget: %v", err)
	}
	if got == nil {
		t.Fatal("a declared budget parsed to nil, which is the spelling of absence")
	}
	if *got != 45*time.Second {
		t.Errorf("parsed budget = %s, want %s", *got, 45*time.Second)
	}

	// And it survives back out, so an agent and the supervisor agree on
	// what was declared.
	out, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"readiness_budget":"45s"`) {
		t.Errorf("re-marshalled descriptor lost the budget: %s", out)
	}
}

// TestAbsentReadinessBudgetStaysNil is the distinction the pointer
// exists for. A descriptor with no such field must be parseable as
// "absent" rather than as any duration, including zero.
func TestAbsentReadinessBudgetStaysNil(t *testing.T) {
	const raw = `{"describe":{"schema_version":"1.0.0","id":"plain-svc","type":"service"}}`

	var env DescribeEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Describe.ReadinessBudget != nil {
		t.Fatalf("an undeclared budget arrived as %q; absent must stay absent", *env.Describe.ReadinessBudget)
	}

	got, err := ParseReadinessBudget(env.Describe)
	if err != nil {
		t.Fatalf("ParseReadinessBudget on an absent field: %v", err)
	}
	if got != nil {
		t.Fatalf("an absent budget parsed to %s; absence must not become a value", *got)
	}
}

// TestAbsentResolvesToTheDerivedDefaultPerLanguage is the rest of task
// 2's gate: a test per language, asserting the two do NOT get the same
// answer. If they did, the per-language derivation would be decoration.
func TestAbsentResolvesToTheDerivedDefaultPerLanguage(t *testing.T) {
	desc := AgentDescribe{SchemaVersion: "1.0.0", ID: "plain-svc", Type: "service"}
	declared, err := ParseReadinessBudget(desc)
	if err != nil {
		t.Fatalf("ParseReadinessBudget: %v", err)
	}

	for _, lang := range []string{"go", "python"} {
		got := budget.Resolve(lang, declared)
		want := budget.DefaultReadinessBudget(lang)
		if got != want {
			t.Errorf("%s: an undeclared budget resolved to %s, want the derived default %s", lang, got, want)
		}
	}

	if budget.Resolve("go", declared) == budget.Resolve("python", declared) {
		t.Error("go and python resolved to the same default: the derivation is not per-language")
	}
}

// TestDeclaredBeatsTheDefault is decision 43(1)'s "declaring is how an
// agent asks for something else", asserted rather than assumed.
func TestDeclaredBeatsTheDefault(t *testing.T) {
	budgetText := "45s"
	desc := AgentDescribe{SchemaVersion: "1.0.0", ID: "slow-svc", Type: "service", ReadinessBudget: &budgetText}

	declared, err := ParseReadinessBudget(desc)
	if err != nil {
		t.Fatalf("ParseReadinessBudget: %v", err)
	}
	for _, lang := range []string{"go", "python"} {
		if got := budget.Resolve(lang, declared); got != 45*time.Second {
			t.Errorf("%s: declared 45s resolved to %s", lang, got)
		}
	}
}

// TestDescriptorAboveTheCeilingIsRefused is task 3's gate, and it is a
// DECLARATION-time refusal: ValidateAgentDescribe is what discovery
// runs before an agent is ever constructed, so the config that cannot
// be valid fails there rather than at 3am.
func TestDescriptorAboveTheCeilingIsRefused(t *testing.T) {
	budgetText := "24h"
	desc := AgentDescribe{
		SchemaVersion:   "1.0.0",
		ID:              "boot-hog",
		Type:            "service",
		ReadinessBudget: &budgetText,
	}

	err := ValidateAgentDescribe(desc)
	if err == nil {
		t.Fatal("a 24h readiness budget validated; decision 43(2) says one agent must not be able to hold a boot phase open forever")
	}

	// ERRORS ARE DATA. The gate is not that something failed, it is that
	// the failure names the declared value, the ceiling and the agent.
	var above *budget.AboveCeiling
	if !errors.As(err, &above) {
		t.Fatalf("expected a *budget.AboveCeiling, got %T: %v", err, err)
	}
	if above.AgentID != "boot-hog" {
		t.Errorf("AgentID = %q, want %q", above.AgentID, "boot-hog")
	}
	if above.Declared != 24*time.Hour {
		t.Errorf("Declared = %s, want %s", above.Declared, 24*time.Hour)
	}
	if above.Ceiling != budget.Ceiling {
		t.Errorf("Ceiling = %s, want %s", above.Ceiling, budget.Ceiling)
	}

	// And the rendered message carries all three, for the operator who
	// reads a log line rather than matching on a type.
	msg := err.Error()
	for _, want := range []string{"boot-hog", "24h", budget.Ceiling.String()} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q does not name %q", msg, want)
		}
	}
}

// TestDescriptorAtTheCeilingIsAccepted keeps the refusal from being a
// off-by-one that quietly forbids the documented maximum.
func TestDescriptorAtTheCeilingIsAccepted(t *testing.T) {
	budgetText := budget.Ceiling.String()
	desc := AgentDescribe{SchemaVersion: "1.0.0", ID: "edge-svc", Type: "service", ReadinessBudget: &budgetText}
	if err := ValidateAgentDescribe(desc); err != nil {
		t.Errorf("the ceiling itself must be declarable, got %v", err)
	}
}

// TestMalformedReadinessBudgetIsRefusedNamingTheText is the third way a
// declaration can be wrong, and the one an author hits most.
func TestMalformedReadinessBudgetIsRefusedNamingTheText(t *testing.T) {
	budgetText := "30 seconds"
	desc := AgentDescribe{SchemaVersion: "1.0.0", ID: "typo-svc", Type: "service", ReadinessBudget: &budgetText}

	err := ValidateAgentDescribe(desc)
	if err == nil {
		t.Fatal("a readiness budget of \"30 seconds\" validated")
	}
	var bad *MalformedReadinessBudget
	if !errors.As(err, &bad) {
		t.Fatalf("expected a *MalformedReadinessBudget, got %T: %v", err, err)
	}
	if bad.Declared != "30 seconds" {
		t.Errorf("Declared = %q, want the text the author wrote", bad.Declared)
	}
}

// TestZeroReadinessBudgetIsRefused holds the line between absence and
// zero. Omitting the field is how an agent asks for the default;
// writing "0s" is an author saying something they cannot have meant,
// and silently treating it as absence would hide the mistake.
func TestZeroReadinessBudgetIsRefused(t *testing.T) {
	budgetText := "0s"
	desc := AgentDescribe{SchemaVersion: "1.0.0", ID: "zero-svc", Type: "service", ReadinessBudget: &budgetText}

	err := ValidateAgentDescribe(desc)
	if err == nil {
		t.Fatal("a readiness budget of 0s validated; zero is not absence")
	}
	var np *budget.NotPositive
	if !errors.As(err, &np) {
		t.Fatalf("expected a *budget.NotPositive, got %T: %v", err, err)
	}
}

// TestSchemaJSONDeclaresTheField is the second declaration of this
// field's existence, and the one nothing in Go would catch. schema.json
// is what an author reads; a field honoured by the validator and absent
// from the schema is a feature only the source discloses.
func TestSchemaJSONDeclaresTheField(t *testing.T) {
	// go test runs in the package directory, so the schema sits beside
	// the source that honours it.
	raw, err := os.ReadFile("schema.json")
	if err != nil {
		t.Fatalf("read schema.json: %v", err)
	}
	var doc struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse schema.json: %v", err)
	}

	p, ok := doc.Properties["readiness_budget"]
	if !ok {
		t.Fatal("schema.json does not declare readiness_budget")
	}
	if p.Type != "string" {
		t.Errorf("schema.json spells readiness_budget as %q, want %q", p.Type, "string")
	}
	for _, r := range doc.Required {
		if r == "readiness_budget" {
			t.Error("schema.json marks readiness_budget required; decision 51 dropped the required form to avoid fifteen copies of the first example")
		}
	}
}
