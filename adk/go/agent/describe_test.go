package agent

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// pythonDescribeKeys is the key set adk/python/agent/runner.py builds in
// describe_data. It is written out here rather than derived because the
// point is to FAIL when the two drift: discovery has one decoder for both
// languages, so a key present on one side and absent on the other is a
// parity break the supervisor experiences as a missing field.
//
// "interval" is deliberately absent: the Python runner adds it only when
// the agent declares one, and Go has no interval declaration.
// Sorted, because the assertion sorts what it collects from the JSON.
// Note "required_by" precedes "requires": '_' sorts before 's'.
var pythonDescribeKeys = []string{
	"capabilities", "cpu_limit", "description", "enabled", "id",
	"language", "listen_stream", "memory_limit", "name", "required_by",
	"requires", "schedule", "schema_version", "type", "version",
	"wanted_by", "wants",
}

func TestDescribe_EmitsThePythonKeySet(t *testing.T) {
	s := &Spec{ID: "probe", Type: "service", Start: func(context.Context) error { return nil }}

	raw, err := s.describeJSON()
	if err != nil {
		t.Fatalf("describeJSON: %v", err)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	body, ok := env["describe"]
	if !ok {
		t.Fatal("no \"describe\" key: discovery reads the envelope, not the payload")
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	var got []string
	for k := range fields {
		got = append(got, k)
	}
	sort.Strings(got)

	if strings.Join(got, ",") != strings.Join(pythonDescribeKeys, ",") {
		t.Errorf("describe keys drifted from the Python runner's set:\n got: %v\nwant: %v",
			got, pythonDescribeKeys)
	}
}

// TestDescribe_AbsentListsAreEmptyNotNull is the one that catches a
// difference no key-set check can see. A nil Go slice marshals to null,
// and null is not [] to anything that reads this - the sort of divergence
// the parity contract exists to keep out.
func TestDescribe_AbsentListsAreEmptyNotNull(t *testing.T) {
	s := &Spec{ID: "probe", Type: "service", Start: func(context.Context) error { return nil }}

	raw, err := s.describeJSON()
	if err != nil {
		t.Fatalf("describeJSON: %v", err)
	}
	if strings.Contains(string(raw), "null") {
		t.Errorf("describe contains null; absent lists must render as []:\n%s", raw)
	}
}

func TestDescribe_LanguageAndDefaults(t *testing.T) {
	s := &Spec{ID: "probe", Type: "service", Start: func(context.Context) error { return nil }}
	d := s.describe().Describe

	if d.Language != "go" {
		t.Errorf("language = %q, want %q", d.Language, "go")
	}
	// Absent Enabled resolves to enabled. Discovery treats an absent
	// field as enabled too, so emitting false here would turn every
	// agent that never mentioned the flag off.
	if !d.Enabled {
		t.Error("an agent that declares no Enabled resolved to disabled")
	}
	// Name falls back to ID rather than being empty, matching the
	// Python runner's behaviour for a module with no NAME.
	if d.Name != "probe" {
		t.Errorf("name = %q, want the id %q", d.Name, "probe")
	}
	if d.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version = %q, want %q", d.SchemaVersion, SchemaVersion)
	}
}

func TestDescribe_EnabledFalseSurvives(t *testing.T) {
	s := &Spec{
		ID: "probe", Type: "service", Enabled: boolPtr(false),
		Start: func(context.Context) error { return nil },
	}
	if s.describe().Describe.Enabled {
		t.Error("an explicit Enabled = false was reported as enabled")
	}
}

// TestCapabilities_FollowTheRunnersOrder pins the ORDER, not just the
// set. The cross-ADK suite compares the two languages' capability arrays
// element by element, so a Go side that reported the same names in a
// different order would fail parity while looking correct here.
func TestCapabilities_FollowTheRunnersOrder(t *testing.T) {
	noop := func() error { return nil }
	s := &Spec{
		ID: "probe", Type: "service",
		Restart:    noop, // declared out of order on purpose
		Stop:       noop,
		Initialize: noop,
		Reload:     noop,
		Start:      func(context.Context) error { return nil },
	}

	want := []string{"initialize", "start", "stop", "reload", "restart"}
	got := s.capabilities()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("capabilities %v, want %v", got, want)
	}
}

func TestCapabilities_OmitUndeclaredLifecycleFunctions(t *testing.T) {
	s := &Spec{ID: "probe", Type: "service", Start: func(context.Context) error { return nil }}

	want := []string{"start"}
	got := s.capabilities()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("capabilities %v, want %v - an agent with only Start must not "+
			"advertise lifecycle verbs it cannot serve", got, want)
	}
}

// TestCapabilities_DeclaredExtrasFollowLifecycle is the parity case for
// Python's @capability decorator. The cross-ADK suite compares the two
// languages' arrays element by element, and its capabilities fixture
// advertises "custom_action" - a name no lifecycle function produces. A
// Go agent with no way to declare it could not reach parity at all.
func TestCapabilities_DeclaredExtrasFollowLifecycle(t *testing.T) {
	noop := func() error { return nil }
	s := &Spec{
		ID: "probe", Type: "service",
		Initialize:   noop,
		Stop:         noop,
		Reload:       noop,
		Start:        func(context.Context) error { return nil },
		Capabilities: []string{"custom_action"},
	}

	want := []string{"initialize", "start", "stop", "reload", "custom_action"}
	if got := s.capabilities(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("capabilities %v, want %v", got, want)
	}
}

// TestCapabilities_DeclaredDuplicateIsDropped matches runner.py, which
// runs dict.fromkeys over the combined list. An agent declaring "stop"
// alongside a Stop function must not advertise it twice.
func TestCapabilities_DeclaredDuplicateIsDropped(t *testing.T) {
	s := &Spec{
		ID: "probe", Type: "service",
		Stop:         func() error { return nil },
		Start:        func(context.Context) error { return nil },
		Capabilities: []string{"stop", "extra"},
	}

	want := []string{"start", "stop", "extra"}
	if got := s.capabilities(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("capabilities %v, want %v", got, want)
	}
}
