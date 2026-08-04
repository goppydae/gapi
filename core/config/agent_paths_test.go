// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// clearAgentEnv removes every variable that steers the search path, so a
// test asserts the DEFAULT tiers rather than whatever the developer's
// shell happens to export.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"GAPI_AGENT_PATH", "GAPI_DEV_AGENTS", "GAPI_SKIP_SYSTEM_AGENTS",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR",
	} {
		t.Setenv(k, "")
		mustUnset(t, k)
	}
}

func indexOf(paths []string, want string) int {
	return slices.Index(paths, want)
}

// TestSystemScope_ConfigOutranksVendor is the regression for the
// inversion this rewrite fixed.
//
// /etc/<p>/agents used to be appended AFTER /usr/lib/<p>/agents, and
// discovery is first-ID-wins, so a package-owned agent MASKED the
// operator's override of it - the exact opposite of the mechanism
// /etc exists to provide. The assertion is on relative ORDER, not on
// membership, because both paths were present before and after; only
// their precedence was wrong.
func TestSystemScope_ConfigOutranksVendor(t *testing.T) {
	clearAgentEnv(t)
	paths := AgentSearchPathsFor(ScopeSystem)

	etc := indexOf(paths, "/etc/gapi/agents")
	usrLocal := indexOf(paths, "/usr/local/lib/gapi/agents")
	usr := indexOf(paths, "/usr/lib/gapi/agents")

	for _, c := range []struct {
		name string
		idx  int
	}{{"/etc/gapi/agents", etc}, {"/usr/local/lib/gapi/agents", usrLocal}, {"/usr/lib/gapi/agents", usr}} {
		if c.idx < 0 {
			t.Fatalf("%s is not in the system search path: %v", c.name, paths)
		}
	}

	if etc > usr || etc > usrLocal {
		t.Errorf("operator config does not outrank vendor: /etc at %d, "+
			"/usr/local/lib at %d, /usr/lib at %d\n%v", etc, usrLocal, usr, paths)
	}
	if usrLocal > usr {
		t.Errorf("/usr/local/lib must outrank /usr/lib: %d vs %d", usrLocal, usr)
	}
}

// TestSystemScope_OrderIsConfigRuntimeVendor pins the whole documented
// ordering rule, not just the pair above.
func TestSystemScope_OrderIsConfigRuntimeVendor(t *testing.T) {
	clearAgentEnv(t)
	paths := AgentSearchPathsFor(ScopeSystem)

	want := []string{
		"/etc/gapi/agents",
		"/run/gapi/agents",
		"/usr/local/lib/gapi/agents",
		"/usr/lib/gapi/agents",
	}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("system search path\n got: %v\nwant: %v", paths, want)
	}
}

// TestSystemScope_ContainsNoHomeDirectory is the security boundary.
//
// safeToExecute refuses foreign-owned binaries at execution time, but a
// user-writable directory has no business in the system manager's
// DISCOVERY list either, and the two defences are not substitutes.
func TestSystemScope_ContainsNoHomeDirectory(t *testing.T) {
	clearAgentEnv(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory to check against: %v", err)
	}

	for _, p := range AgentSearchPathsFor(ScopeSystem) {
		if underPrefix(p, home) {
			t.Errorf("system scope includes the home-directory path %q", p)
		}
	}
}

// TestSystemScope_HasNoWorkingDirectoryTier guards the removal of the
// implicit ./agents entry. Its absence is the property: with it, a daemon
// started from the wrong directory silently discovered nothing.
func TestSystemScope_HasNoWorkingDirectoryTier(t *testing.T) {
	clearAgentEnv(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for _, p := range AgentSearchPathsFor(ScopeSystem) {
		if underPrefix(p, cwd) {
			t.Errorf("system scope includes a working-directory path %q", p)
		}
	}
}

func TestUserScope_ConfigOutranksData(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("XDG_CONFIG_HOME", "/tmp/probe-config")
	t.Setenv("XDG_DATA_HOME", "/tmp/probe-data")
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/probe-run")

	paths := AgentSearchPathsFor(ScopeUser)
	cfg := indexOf(paths, "/tmp/probe-config/gapi/agents")
	run := indexOf(paths, "/tmp/probe-run/gapi/agents")
	data := indexOf(paths, "/tmp/probe-data/gapi/agents")

	if cfg < 0 || run < 0 || data < 0 {
		t.Fatalf("user scope missing an XDG tier: %v", paths)
	}
	// config beats runtime beats data - the rule both scopes share.
	if cfg >= run || run >= data {
		t.Errorf("user tiers out of order: config=%d runtime=%d data=%d\n%v",
			cfg, run, data, paths)
	}
}

// TestUserScope_OmitsTiersWhoseRootIsUnset covers PID 1 at early boot,
// where there may be no XDG_RUNTIME_DIR and no HOME. A missing root must
// omit the tier, never produce a path rooted at "/".
func TestUserScope_OmitsTiersWhoseRootIsUnset(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("HOME", "")
	mustUnset(t, "HOME")

	for _, p := range AgentSearchPathsFor(ScopeUser) {
		if p == "/gapi/agents" || strings.HasPrefix(p, "/gapi/") {
			t.Errorf("an unset root produced the rooted path %q", p)
		}
		if !filepath.IsAbs(p) {
			t.Errorf("relative path %q in the search list", p)
		}
	}
}

func TestScopes_Differ(t *testing.T) {
	clearAgentEnv(t)
	sys := strings.Join(AgentSearchPathsFor(ScopeSystem), ",")
	usr := strings.Join(AgentSearchPathsFor(ScopeUser), ",")
	if sys == usr {
		t.Error("system and user scope return the same list; scope is not a parameter")
	}
}

func TestDevAgents_OutranksEveryTier(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("GAPI_DEV_AGENTS", "/tmp/probe-dev")

	for _, scope := range []Scope{ScopeSystem, ScopeUser} {
		paths := AgentSearchPathsFor(scope)
		if len(paths) == 0 || paths[0] != "/tmp/probe-dev" {
			t.Errorf("%s scope: DEV_AGENTS is not first: %v", scope, paths)
		}
	}
}

// TestAgentPath_PrependsRatherThanReplaces is half the GAPI-DIV-063 pair.
//
// AGENT_PATH used to REPLACE the whole search path, which made the tier
// list dead code in a packaged install: the nix module sets this variable,
// so /etc and /usr/lib were never searched on the only configuration that
// ships. The assertion is that the named directories win precedence AND
// that the built-in tiers survive underneath - either half alone passes
// against a wrong implementation.
func TestAgentPath_PrependsRatherThanReplaces(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("GAPI_AGENT_PATH", "/tmp/a:/tmp/b")

	got := AgentSearchPathsFor(ScopeSystem)
	want := []string{
		"/tmp/a", "/tmp/b",
		"/etc/gapi/agents", "/run/gapi/agents",
		"/usr/local/lib/gapi/agents", "/usr/lib/gapi/agents",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("AGENT_PATH is not additive with precedence\n got: %v\nwant: %v", got, want)
	}
}

// TestAgentPathExclusive_SearchesOnlyWhatItNames is the other half.
//
// The fence has to survive the additive change: test/adk's harness relies
// on it, and without a fence the checkout's own agents starve the fixture
// agents' state transitions (GAPI-DIV-021, a ~90s timeout).
func TestAgentPathExclusive_SearchesOnlyWhatItNames(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("GAPI_AGENT_PATH", "/tmp/only")
	t.Setenv("GAPI_AGENT_PATH_EXCLUSIVE", "1")

	got := AgentSearchPathsFor(ScopeSystem)
	want := []string{"/tmp/only"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the fence leaked: %v", got)
	}
}

// TestAgentPathExclusive_EmptyMeansEmpty pins the corner the switch makes
// reachable. Asking for an empty search path is coherent; falling back to
// the full tier list would be the opposite of what was asked, and would
// hand a test harness every agent in the checkout.
func TestAgentPathExclusive_EmptyMeansEmpty(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("GAPI_AGENT_PATH_EXCLUSIVE", "true")

	if got := AgentSearchPathsFor(ScopeSystem); len(got) != 0 {
		t.Errorf("exclusive with no AGENT_PATH returned %v, want nothing", got)
	}
}

// TestAgentPath_RanksBelowDevAgents pins the precedence between the two
// additive sources, which is otherwise the sort of thing that gets
// decided by accident.
func TestAgentPath_RanksBelowDevAgents(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("GAPI_DEV_AGENTS", "/tmp/dev")
	t.Setenv("GAPI_AGENT_PATH", "/tmp/custom")

	got := AgentSearchPathsFor(ScopeSystem)
	dev := indexOf(got, "/tmp/dev")
	custom := indexOf(got, "/tmp/custom")
	if dev < 0 || custom < 0 {
		t.Fatalf("both sources should be present: %v", got)
	}
	if dev > custom {
		t.Errorf("DEV_AGENTS must outrank AGENT_PATH: dev=%d custom=%d\n%v", dev, custom, got)
	}
}

// TestAgentPath_DropsEmptyEntries guards a trailing or doubled colon. An
// empty entry becomes "", which filepath.Join renders relative and
// reintroduces the working-directory dependence this scheme removed.
func TestAgentPath_DropsEmptyEntries(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("GAPI_AGENT_PATH", "/tmp/a::/tmp/b:")

	for _, p := range AgentSearchPathsFor(ScopeSystem) {
		if p == "" {
			t.Fatal("an empty entry survived into the search path")
		}
	}
}

// TestSkipSystemAgents_RemovesVendorTiersOnly pins what the flag means:
// package-owned directories go, operator-authored and transient ones
// stay. Dropping /etc along with /usr/lib would make the flag
// unusable for its actual purpose, which is ignoring an installed
// package while testing.
func TestSkipSystemAgents_RemovesVendorTiersOnly(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("GAPI_SKIP_SYSTEM_AGENTS", "1")

	paths := AgentSearchPathsFor(ScopeSystem)
	if indexOf(paths, "/usr/lib/gapi/agents") >= 0 {
		t.Error("vendor tier survived SKIP_SYSTEM_AGENTS")
	}
	if indexOf(paths, "/usr/local/lib/gapi/agents") >= 0 {
		t.Error("local-install tier survived SKIP_SYSTEM_AGENTS")
	}
	if indexOf(paths, "/etc/gapi/agents") < 0 {
		t.Error("SKIP_SYSTEM_AGENTS removed the operator tier")
	}
	if indexOf(paths, "/run/gapi/agents") < 0 {
		t.Error("SKIP_SYSTEM_AGENTS removed the transient tier")
	}
}

// TestClassifyPath_AgreesWithTheSearchedList is the anti-drift check.
// Every system tier must classify as SYSTEM; a label that disagrees with
// the precedence that produced it makes a log line actively misleading.
func TestClassifyPath_AgreesWithTheSearchedList(t *testing.T) {
	clearAgentEnv(t)

	for _, p := range AgentSearchPathsFor(ScopeSystem) {
		if got := ClassifyPath(p); got != PathTypeSystem {
			t.Errorf("ClassifyPath(%q) = %s, want SYSTEM", p, got)
		}
	}
}

// TestClassifyPath_DoesNotMatchOnStringPrefix catches the substring bug.
//
// The fixture has to share the ENTIRE tier string and diverge after it,
// on a non-separator boundary. An earlier version of this test used
// "/usr/lib/gapifoo/agents", which shares no string prefix with
// "/usr/lib/gapi/agents" at all - it passed against a deliberately
// broken strings.HasPrefix implementation and could never have failed.
// The revert is what exposed that; nothing else would have.
func TestClassifyPath_DoesNotMatchOnStringPrefix(t *testing.T) {
	clearAgentEnv(t)

	for _, p := range []string{
		"/etc/gapi/agentsfoo",
		"/usr/lib/gapi/agents-disabled",
	} {
		if got := ClassifyPath(p); got == PathTypeSystem {
			t.Errorf("ClassifyPath(%q) = SYSTEM; it shares a string prefix with a "+
				"tier but is an unrelated directory", p)
		}
	}
}

func TestClassifyPath_DevAgentsWins(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("GAPI_DEV_AGENTS", "/tmp/probe-dev")

	if got := ClassifyPath("/tmp/probe-dev/nested"); got != PathTypeDevelopment {
		t.Errorf("ClassifyPath under DEV_AGENTS = %s, want DEV", got)
	}
}

// mustUnset removes an environment variable, failing the test if it
// cannot. The strict errcheck policy applies to tests too: an ignored
// error here would leave a variable set and silently change what the
// assertions below are measuring.
func mustUnset(t *testing.T, key string) {
	t.Helper()
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}
