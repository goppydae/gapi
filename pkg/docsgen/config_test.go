// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package docsgen_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/goppydae/gapi/pkg/docsgen"
)

// The schema under test is synthetic. Pointing these at gapi's real
// core/config would make them a test of core/config reached through
// docsgen, and they would go red for a reason that is not this
// package's - which is the difference between a failure that names a
// cause and one that names a neighbour.
type probeTLS struct {
	Cert string `mapstructure:"cert"`
	// Empty means no CA is pinned.
	CA string `mapstructure:"ca"`
}

type probeConfig struct {
	Address string            `mapstructure:"address"`
	Port    int               `mapstructure:"port"`
	Enabled bool              `mapstructure:"enabled"`
	TLS     probeTLS          `mapstructure:"tls"`
	Labels  map[string]string `mapstructure:"labels"`
	Hidden  string            `mapstructure:"-"`
	Bare    string
}

func probeDefaults() *viper.Viper {
	v := viper.New()
	v.SetDefault("address", "127.0.0.1:29979")
	v.SetDefault("port", 8080)
	v.SetDefault("enabled", false)
	v.SetDefault("tls.cert", "")
	v.SetDefault("tls.ca", "")
	return v
}

func probeEnvKey(path string) string {
	return "PROBE_" + strings.ToUpper(strings.ReplaceAll(path, ".", "_"))
}

func buildProbeModel(t *testing.T, srcDir string) *docsgen.ConfigModel {
	t.Helper()
	m, err := docsgen.BuildConfigModel(docsgen.ConfigOptions{
		Product:   "probe",
		Schema:    &probeConfig{},
		Defaults:  probeDefaults(),
		SourceDir: srcDir,
		EnvKeyFor: probeEnvKey,
	})
	if err != nil {
		t.Fatalf("BuildConfigModel: %v", err)
	}
	return m
}

func TestBuildConfigModel_WalksScalarLeavesAndSkipsTheRest(t *testing.T) {
	m := buildProbeModel(t, "")

	var got []string
	for _, k := range m.Keys {
		got = append(got, k.Path)
	}
	want := []string{"address", "enabled", "port", "tls.ca", "tls.cert"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

// Sorted, because the drift gate compares bytes and something upstream
// of every renderer walks a map.
func TestBuildConfigModel_KeysAreSorted(t *testing.T) {
	m := buildProbeModel(t, "")
	for i := 1; i < len(m.Keys); i++ {
		if m.Keys[i-1].Path > m.Keys[i].Path {
			t.Fatalf("keys are not sorted: %q before %q", m.Keys[i-1].Path, m.Keys[i].Path)
		}
	}
}

func TestBuildConfigModel_JoinsTypeValueAndEnv(t *testing.T) {
	m := buildProbeModel(t, "")
	byPath := map[string]docsgen.Key{}
	for _, k := range m.Keys {
		byPath[k.Path] = k
	}

	addr := byPath["address"]
	if addr.Type != "string" || addr.Value != "127.0.0.1:29979" || addr.Env != "PROBE_ADDRESS" {
		t.Errorf("address = %+v", addr)
	}
	if got := byPath["port"]; got.Type != "int" || got.Value != "8080" {
		t.Errorf("port = %+v", got)
	}
	// An empty default must survive as empty rather than becoming
	// "<nil>", which is what fmt would produce for a missing key.
	if got := byPath["tls.cert"]; got.Value != "" {
		t.Errorf("tls.cert default = %q, want empty", got.Value)
	}
}

// Reflection cannot see comments, so this is the only path by which a
// key's explanation reaches its documentation.
func TestBuildConfigModel_JoinsDocCommentsFromSource(t *testing.T) {
	dir := t.TempDir()
	src := `package probe

type probeTLS struct {
	Cert string ` + "`mapstructure:\"cert\"`" + `
	// Empty means no CA is pinned.
	CA string ` + "`mapstructure:\"ca\"`" + `
}
`
	if err := os.WriteFile(filepath.Join(dir, "probe.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	// A test file in the same directory must not contribute.
	if err := os.WriteFile(filepath.Join(dir, "probe_test.go"), []byte("package probe\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := buildProbeModel(t, dir)
	for _, k := range m.Keys {
		if k.Path == "tls.ca" && k.Doc != "Empty means no CA is pinned." {
			t.Errorf("tls.ca doc = %q, want the field comment", k.Doc)
		}
	}
}

// THE GATE THAT FALLS OUT OF THE JOIN. A default with no struct field
// would be published as settable and then silently discarded at
// unmarshal.
func TestBuildConfigModel_DefaultWithNoFieldIsRefused(t *testing.T) {
	v := probeDefaults()
	v.SetDefault("transport.retired", "ghost")

	_, err := docsgen.BuildConfigModel(docsgen.ConfigOptions{
		Product: "probe", Schema: &probeConfig{}, Defaults: v, EnvKeyFor: probeEnvKey,
	})
	if err == nil {
		t.Fatal("a default naming no struct field must fail generation")
	}
	if !strings.Contains(err.Error(), "transport.retired") {
		t.Errorf("error %q does not name the orphaned key", err)
	}
}

func TestBuildConfigModel_RejectsIncompleteOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    docsgen.ConfigOptions
	}{
		{"no schema", docsgen.ConfigOptions{Defaults: probeDefaults(), EnvKeyFor: probeEnvKey}},
		{"no defaults", docsgen.ConfigOptions{Schema: &probeConfig{}, EnvKeyFor: probeEnvKey}},
		{"no env func", docsgen.ConfigOptions{Schema: &probeConfig{}, Defaults: probeDefaults()}},
		{"not a struct", docsgen.ConfigOptions{Schema: "nope", Defaults: probeDefaults(), EnvKeyFor: probeEnvKey}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := docsgen.BuildConfigModel(tc.o); err == nil {
				t.Fatalf("%s must be rejected", tc.name)
			}
		})
	}
}

// The field names are the contract with magelib's defaults gate. A
// rename on one side alone produces a gate that scans for nothing.
func TestDefaultsJSON_ShapeMatchesTheGateContract(t *testing.T) {
	m := buildProbeModel(t, "")
	data, err := docsgen.DefaultsJSON(m, "core/config.setDefaults")
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]struct {
		Value  string `json:"value"`
		Env    string `json:"env"`
		Type   string `json:"type"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("defaults.json does not parse: %v", err)
	}
	got, ok := parsed["address"]
	if !ok {
		t.Fatal("defaults.json has no entry for address")
	}
	if got.Value != "127.0.0.1:29979" || got.Env != "PROBE_ADDRESS" ||
		got.Type != "string" || got.Source != "core/config.setDefaults" {
		t.Errorf("address entry = %+v", got)
	}
}

func TestDefaultsJSON_RequiresASource(t *testing.T) {
	if _, err := docsgen.DefaultsJSON(buildProbeModel(t, ""), ""); err == nil {
		t.Fatal("defaults.json without a source must be refused")
	}
}

func TestConfigMan_CarriesTheInjectedVersionAndNoClock(t *testing.T) {
	out := string(docsgen.ConfigMan(buildProbeModel(t, ""), "0.1.0-proto2k"))

	if !strings.Contains(out, `.TH PROBE.CONF 5 "0.1.0-proto2k"`) {
		t.Errorf("man header does not carry the injected version:\n%s", firstLine(out))
	}
	for _, key := range []string{"address", "tls.cert", "PROBE_TLS_CERT"} {
		if !strings.Contains(out, key) {
			t.Errorf("section 5 page omits %q", key)
		}
	}
	// An empty default must read as a decision, not a blank cell.
	if !strings.Contains(out, "Default: (none).") {
		t.Error("an empty default is not rendered as (none)")
	}
}

// A backslash is roff's escape character and a leading period is a roff
// request; both fail silently by eating the rest of the line.
func TestConfigMan_EscapesRoffMetacharacters(t *testing.T) {
	v := viper.New()
	v.SetDefault("address", `.hidden\path`)
	v.SetDefault("port", 1)
	v.SetDefault("enabled", false)
	v.SetDefault("tls.cert", "")
	v.SetDefault("tls.ca", "")

	m, err := docsgen.BuildConfigModel(docsgen.ConfigOptions{
		Product: "probe", Schema: &probeConfig{}, Defaults: v, EnvKeyFor: probeEnvKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := string(docsgen.ConfigMan(m, "1.0.0"))
	if strings.Contains(out, `\path`) && !strings.Contains(out, `\e`) {
		t.Error("backslash was not escaped, so roff will consume the next character")
	}
	if !strings.Contains(out, `\&.hidden`) {
		t.Error("a value starting with a period was not escaped and reads as a roff request")
	}
}

func TestConfigMarkdown_TablesEveryKeyAndNotesTheDocumentedOnes(t *testing.T) {
	dir := t.TempDir()
	src := "package probe\n\ntype probeTLS struct {\n\t// Empty means no CA is pinned.\n\tCA string `mapstructure:\"ca\"`\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "probe.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	out := string(docsgen.ConfigMarkdown(buildProbeModel(t, dir), 30))

	if !strings.HasPrefix(out, "---\n") || !strings.Contains(out, `title: "Configuration"`) {
		t.Error("page has no relearn front matter")
	}
	if !strings.Contains(out, "| `address` | `string` | `127.0.0.1:29979` | `PROBE_ADDRESS` |") {
		t.Error("address row is missing or malformed")
	}
	if !strings.Contains(out, "| `tls.cert` | `string` | (none) | `PROBE_TLS_CERT` |") {
		t.Error("an empty default is not rendered as (none)")
	}
	if !strings.Contains(out, "Empty means no CA is pinned.") {
		t.Error("documented keys have no notes section")
	}
}

// Every artifact here is committed and byte-compared, so a renderer that
// is not a pure function of its model dirties the tree on every build.
func TestConfigRenderers_AreDeterministic(t *testing.T) {
	m := buildProbeModel(t, "")

	if a, b := docsgen.ConfigMan(m, "1.0.0"), docsgen.ConfigMan(m, "1.0.0"); string(a) != string(b) {
		t.Error("ConfigMan is not deterministic")
	}
	if a, b := docsgen.ConfigMarkdown(m, 1), docsgen.ConfigMarkdown(m, 1); string(a) != string(b) {
		t.Error("ConfigMarkdown is not deterministic")
	}
	a, err := docsgen.DefaultsJSON(m, "src")
	if err != nil {
		t.Fatal(err)
	}
	b, err := docsgen.DefaultsJSON(m, "src")
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Error("DefaultsJSON is not deterministic")
	}
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}
