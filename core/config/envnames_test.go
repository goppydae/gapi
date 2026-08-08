// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/product"
)

// This is GAPI-DIV-059's gate. It walks the whole tree, not just this
// package, because the defect it prevents was never confined to one:
// the prefix constant lived here while the names that disagreed with it
// were spread across Go, Python, shell, nix and eleven markdown files.
//
// It enforces two properties, and the second is the one that would have
// caught the original defect years earlier:
//
//  1. No project-owned environment name carries an old prefix. A single
//     reintroduced RUNTIME_ literal fails.
//  2. No GAPI_ name appears in documentation without a reader in code.
//     agent_paths.go documented GAPI_AGENT_PATH in its own doc comment
//     while the code beneath read RUNTIME_AGENT_PATH, and nothing
//     noticed because documentation is not compiled.
//
// It lives in core/config because that package owns EnvKeyFor, which is
// the declaration the rest of the tree is being checked against. The
// PREFIX itself moved to core/product in GAPI-DIV-061, and that move is
// why composed() below grew a second source: once a name is assembled
// from a product identity at runtime, no literal spells it and a
// scanning gate goes blind. See configOverrideNames.

// notEnvNames are identifiers that match an old-prefix pattern and are
// not environment variables at all. GAPID_PID is a shell local holding
// the daemon's pid in three test scripts; renaming it would be
// meaningless and leaving it unlisted would make this gate cry wolf.
var notEnvNames = map[string]bool{
	"GAPID_PID": true,
}

// envDocWaivers is DEBT: GAPI_ names that documentation describes and no
// code reads, carried deliberately with the entry that tracks each.
//
// The list can only shrink. If a waived name gains a reader the gate
// FAILS, naming the waiver to delete - the same shape as magelib's
// fileLengthWaivers, and for the same reason: a waiver that silently
// stops applying is indistinguishable from one nobody needed. It fails
// the same way when a waived name LEAVES the documentation, which is
// the other way a waiver stops excusing anything (GAPI-DIV-060).
var envDocWaivers = map[string]string{}

// envKnownAbsent are names documentation states do NOT exist, so that a
// reader appearing for one is also a defect - the documentation would
// then be wrong in the opposite direction. This is the mirror of the
// waiver list and it fails the same way.
var envKnownAbsent = map[string]bool{
	"GAPI_AGENTS_DIR":   true, // docs/content/user/configuration-examples.md, nix/tests/module-boot.nix
	"GAPI_LOG_LEVEL":    true, // goppydae-docs, develop section
	"GAPI_TRACE_EVENTS": true, // goppydae-docs, develop section
	"GAPI_SOCKET":       true, // docs/content/user/agent-metadata.md
}

var (
	// ADK_ joins GAPI_ here because the kernel-to-agent contract is a
	// second namespace with the same obligation: a name documentation
	// describes and nothing reads is indistinguishable from a feature,
	// whoever owns the prefix (core/agentmgr/agent_env.go).
	envNameRe = regexp.MustCompile(`\b(?:GAPI|ADK)_[A-Z0-9_]+\b`)
	oldNameRe = regexp.MustCompile(`\b(?:RUNTIME|GAPID)_[A-Z0-9_]+\b`)
)

// readsEnv reports whether body actually READS OR SETS name, as opposed
// to merely mentioning it.
//
// The distinction is load-bearing and cost a false positive to learn:
// nix/tests/module-boot.nix asserts `"GAPI_AGENTS_DIR" not in unit_env`,
// which mentions the name in order to prove it is absent. A gate that
// counts mentions concludes the opposite of what the file asserts.
//
// Matched forms: a Getenv/LookupEnv/Setenv call in Go or Python, and an
// assignment - which covers Go's "NAME=value" env slices, shell exports,
// nix attributes and Dockerfile ENV lines alike.
func readsEnv(body, name string) bool {
	q := regexp.QuoteMeta(name)
	call := regexp.MustCompile(`(?i)(?:getenv|lookupenv|setenv)\(\s*"` + q + `"`)
	assign := regexp.MustCompile(`\b` + q + `\s*=[^=]`)
	return call.MatchString(body) || assign.MatchString(body)
}

// composedEnvNames renders every environment name the kernel reads that
// no literal spells, from the two places that compose them.
//
// The first is the config loader: walk Config's mapstructure tags
// exactly as bindEnvOverrides does and render each through the
// production EnvKeyFor. GAPI-DIV-059's exit predicted this as an
// unavoidable residual - "a literal-scanning test cannot see a name
// built by concatenation at runtime". It can, if it composes the same
// names from the same struct rather than scanning for them:
// docs/content/user/configuration-examples.md documents
// GAPI_LOGGING_FORMAT and
// GAPI_METRICS_ENABLED, which no literal anywhere spells and the loader
// nonetheless binds.
//
// The second is core/product's registry of DIRECT reads - the names that
// are not config keys, like GAPI_CONFIG and GAPI_CGROUPS_DISABLE. Those
// were literals until GAPI-DIV-061 made them derive from the product
// identity. Without this half the gate would still have passed, and for
// a bad reason: test/adk/framework.go and test/e2e.sh spell several of
// them, so a production reader could disappear entirely while a test
// harness kept the gate green.
func composedEnvNames() map[string]bool {
	out := map[string]bool{}
	for _, n := range product.DirectEnvNames() {
		out[n] = true
	}
	var walk func(t reflect.Type, prefix string)
	walk = func(t reflect.Type, prefix string) {
		for i := range t.NumField() {
			f := t.Field(i)
			tag := f.Tag.Get("mapstructure")
			if tag == "" || tag == "-" {
				continue
			}
			path := tag
			if prefix != "" {
				path = prefix + "." + tag
			}
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				walk(ft, path)
				continue
			}
			out[config.EnvKeyFor(path)] = true
		}
	}
	walk(reflect.TypeOf(config.Config{}), "")
	return out
}

// repoRoot walks up from the test's directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test's working directory")
		}
		dir = parent
	}
}

// scanned reports whether a path participates. vendor/ is other
// people's code and .git/ is not source.
func scanned(rel string) bool {
	if strings.HasPrefix(rel, "vendor/") || strings.HasPrefix(rel, ".git/") {
		return false
	}
	switch filepath.Ext(rel) {
	case ".go", ".md", ".py", ".sh", ".nix", ".yml", ".yaml", ".tmpl":
		return true
	}
	return false
}

// walkTree calls visit for every scanned file with its contents.
func walkTree(t *testing.T, visit func(rel, body string)) {
	t.Helper()
	root := repoRoot(t)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil || !scanned(rel) {
			return nil //nolint:nilerr // an unrelatable path is not ours to scan
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		visit(rel, string(raw))
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// No old-prefix environment name survives anywhere in the tree.
func TestEnvNames_NoOldPrefixSurvives(t *testing.T) {
	// This file necessarily contains the old spellings - it is the thing
	// that looks for them - and core/config/config.go records the rename
	// in EnvPrefix's doc comment. Both are prose about the rename, not
	// uses of it.
	exempt := map[string]bool{
		"core/config/envnames_test.go": true,
		"core/config/config.go":        true,
	}

	var found []string
	walkTree(t, func(rel, body string) {
		if exempt[rel] {
			return
		}
		for _, m := range oldNameRe.FindAllString(body, -1) {
			if notEnvNames[m] {
				continue
			}
			found = append(found, rel+": "+m)
		}
	})

	if len(found) > 0 {
		sort.Strings(found)
		t.Errorf("old-prefix environment names survive the GAPI-DIV-059 rename.\n"+
			"The rename is hard - nothing reads these - so each is dead or a regression:\n  %s",
			strings.Join(dedupe(found), "\n  "))
	}
}

// Every GAPI_ name in documentation is read by code, or declared as debt.
func TestEnvNames_DocumentedNamesHaveReaders(t *testing.T) {
	documented := map[string][]string{}
	var codeBodies []string

	walkTree(t, func(rel, body string) {
		if rel == "core/config/envnames_test.go" {
			return // the declarations below are not uses
		}
		if filepath.Ext(rel) == ".md" {
			for _, n := range envNameRe.FindAllString(body, -1) {
				documented[n] = append(documented[n], rel)
			}
			return
		}
		codeBodies = append(codeBodies, body)
	})

	// Names the loader binds by composing them at runtime; no literal
	// spells them, and they are read all the same.
	composed := composedEnvNames()

	inCode := func(name string) bool {
		if composed[name] {
			return true
		}
		for _, body := range codeBodies {
			if readsEnv(body, name) {
				return true
			}
		}
		return false
	}

	var orphans []string
	for name, files := range documented {
		if inCode(name) || envKnownAbsent[name] {
			continue
		}
		if _, waived := envDocWaivers[name]; waived {
			continue
		}
		sort.Strings(files)
		orphans = append(orphans, name+" (in "+strings.Join(dedupe(files), ", ")+")")
	}
	if len(orphans) > 0 {
		sort.Strings(orphans)
		t.Errorf("documented GAPI_ names that no code reads.\n"+
			"Documentation is not compiled, so a name only documentation knows "+
			"is indistinguishable from a feature - implement it, retire the "+
			"claim, or declare it in envDocWaivers with the entry that tracks it:\n  %s",
			strings.Join(orphans, "\n  "))
	}
}

// A waiver whose name gained a reader must be deleted, and a name
// documented as absent must not gain one. Both lists shrink or fail.
func TestEnvNames_DeclarationsStillApply(t *testing.T) {
	var codeBodies []string
	documented := map[string]bool{}
	walkTree(t, func(rel, body string) {
		if rel == "core/config/envnames_test.go" {
			return
		}
		if filepath.Ext(rel) == ".md" {
			for _, n := range envNameRe.FindAllString(body, -1) {
				documented[n] = true
			}
			return
		}
		codeBodies = append(codeBodies, body)
	})
	composed := composedEnvNames()
	inCode := func(name string) bool {
		if composed[name] {
			return true
		}
		for _, body := range codeBodies {
			if readsEnv(body, name) {
				return true
			}
		}
		return false
	}

	for name, why := range envDocWaivers {
		if inCode(name) {
			t.Errorf("%s now has a reader in code, so its waiver is stale - delete it.\nWaiver said: %s",
				name, why)
		}
	}

	// A waiver excuses a DOCUMENTED name that no code reads. When the
	// documentation retires the name, the waiver excuses nothing and must
	// go - otherwise it is indistinguishable from one nobody needed, which
	// is the failure mode the waiver list exists to prevent.
	//
	// This is the direction this test's own comment always claimed ("Both
	// lists shrink or fail") and the code did not implement: the loop above
	// fires only when a waived name GAINS a reader (GAPI-DIV-060).
	for name, why := range envDocWaivers {
		if !documented[name] {
			t.Errorf("%s is no longer named by any documentation, so its "+
				"waiver excuses nothing - delete it.\nWaiver said: %s",
				name, why)
		}
	}

	for name := range envKnownAbsent {
		if inCode(name) {
			t.Errorf("%s is documented as not existing and now has a reader - "+
				"the documentation is wrong in the other direction", name)
		}
	}

	// The same missing direction, for the mirror list. envKnownAbsent
	// exists because documentation SAYS a name does not exist; it earns
	// its place from that sentence. Once the documentation stops naming
	// it, the declaration is not merely dead - it is a live false
	// positive, because a legitimately new GAPI_ variable that happens to
	// reuse the name would fail the loop above with nothing in the docs to
	// justify it.
	//
	// The reason each name is listed lives in a trailing comment on its
	// declaration rather than in the map's value, so this message points
	// there instead of quoting it.
	for name := range envKnownAbsent {
		if !documented[name] {
			t.Errorf("%s is no longer named by any documentation, so declaring "+
				"it known-absent guards nothing and would fail a future reader "+
				"for no stated reason - delete it from envKnownAbsent.\n"+
				"See the comment on its declaration for which document listed "+
				"it as absent, and check whether that document dropped the "+
				"name deliberately.", name)
		}
	}
}

// dedupe removes repeats while preserving order.
func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
