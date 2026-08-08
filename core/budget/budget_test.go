// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package budget

import (
	"errors"
	"testing"
	"time"
)

// THESE TESTS ARE IN-PACKAGE ON PURPOSE, AND ONLY FOR THE PERTURBATION.
// The gate GAPI-DIV-107 sets is that both budgets derive from ONE table
// and that CHANGING THE TABLE MOVES BOTH, and there is no way to prove
// shared derivation from outside the package - a test that instead
// hardcoded 250ms and 10s would pass just as happily against two
// independent constants, which is the two-declarations defect this repo
// keeps paying for. Every assertion below is still made through the
// exported functions; only the perturbation reaches inside.

// withP95 replaces the measured table for the duration of a test and
// restores it after. The restore is registered before the mutation so a
// failing assertion cannot leak a doctored table into the next test.
func withP95(t *testing.T, lang string, p95 time.Duration) {
	t.Helper()
	prev, existed := firstFrameP95[lang]
	t.Cleanup(func() {
		if existed {
			firstFrameP95[lang] = prev
			return
		}
		delete(firstFrameP95, lang)
	})
	firstFrameP95[lang] = p95
}

// TestChangingTheTableMovesBothBudgets is the gate. One table, two
// factors: a language whose measured p95 doubles must see BOTH its
// budgets double, because a budget that did not move would be reading
// from somewhere else.
//
// The p95 values are chosen above both floors (>10ms clears
// floorReadiness/factorReadiness, the higher of the two thresholds) so
// the derivation rather than a floor is what is under test.
func TestChangingTheTableMovesBothBudgets(t *testing.T) {
	const lang = "test-lang"

	withP95(t, lang, 20*time.Millisecond)
	silence1, readiness1 := SilenceBudget(lang), DefaultReadinessBudget(lang)

	withP95(t, lang, 40*time.Millisecond)
	silence2, readiness2 := SilenceBudget(lang), DefaultReadinessBudget(lang)

	if silence2 != 2*silence1 {
		t.Errorf("doubling the measured p95 moved the silence budget from %s to %s, want %s: the silence budget is not reading the measured table",
			silence1, silence2, 2*silence1)
	}
	if readiness2 != 2*readiness1 {
		t.Errorf("doubling the measured p95 moved the readiness budget from %s to %s, want %s: the readiness budget is not reading the measured table",
			readiness1, readiness2, 2*readiness1)
	}
}

// TestBothBudgetsShareOneTableEntry is the other half: the two budgets
// must stand in exactly the ratio of their factors, which they can only
// do if they are multiplying the same number.
func TestBothBudgetsShareOneTableEntry(t *testing.T) {
	const lang = "test-lang"
	withP95(t, lang, 20*time.Millisecond)

	silence, readiness := SilenceBudget(lang), DefaultReadinessBudget(lang)
	want := time.Duration(factorReadiness/factorSilence) * silence
	if readiness != want {
		t.Errorf("readiness %s is not %dx silence %s (want %s): the two budgets are not derived from one table entry",
			readiness, factorReadiness/factorSilence, silence, want)
	}
}

// TestWhichArmWinsPerLanguage pins the SHAPE of each language's
// derivation - floored or measured - without writing any number twice.
// Every expected value below is computed from the same constants the
// functions use, so this test cannot drift from them; it fails only if
// the arm changes, which is a policy change and should be seen.
func TestWhichArmWinsPerLanguage(t *testing.T) {
	// Go is sub-millisecond, so both its budgets are the floors: the
	// measurement is 183x under the silence floor and 8000x under the
	// readiness floor.
	if got := SilenceBudget("go"); got != floorSilence {
		t.Errorf("go silence budget = %s, want the floor %s", got, floorSilence)
	}
	if got := DefaultReadinessBudget("go"); got != floorReadiness {
		t.Errorf("go readiness budget = %s, want the floor %s", got, floorReadiness)
	}

	// Python's measurement clears both floors, so both its budgets are
	// measured rather than floored. This is the per-language derivation
	// actually doing something.
	if got, want := SilenceBudget("python"), firstFrameP95["python"]*factorSilence; got != want {
		t.Errorf("python silence budget = %s, want the measured %s", got, want)
	}
	if got, want := DefaultReadinessBudget("python"), firstFrameP95["python"]*factorReadiness; got != want {
		t.Errorf("python readiness budget = %s, want the measured %s", got, want)
	}
}

// TestSilenceIsMuchTighterThanReadiness is decision 43(3) as an
// assertion. If these were ever equal the entry's whole distinction
// would have collapsed back into one constant bounding two deadlines.
func TestSilenceIsMuchTighterThanReadiness(t *testing.T) {
	for _, lang := range []string{"go", "python", "never-measured"} {
		s, r := SilenceBudget(lang), DefaultReadinessBudget(lang)
		if s >= r {
			t.Errorf("%s: silence %s is not tighter than readiness %s", lang, s, r)
		}
	}
}

// TestNoDefaultTightensToday is the safety property decision 66 was
// made for: this budget REPLACES a WaitStart that is 10s for everyone,
// so no language's default may come out below 10s. If one did, this
// change would have introduced a start-timeout failure that did not
// exist before, on infrastructure GAPI-DIV-120 shows producing second
// modes.
func TestNoDefaultTightensToday(t *testing.T) {
	const waitStartWas = 10 * time.Second
	for _, lang := range []string{"go", "python", "never-measured", ""} {
		if got := DefaultReadinessBudget(lang); got < waitStartWas {
			t.Errorf("%q: default readiness %s is tighter than the %s WaitStart it replaces", lang, got, waitStartWas)
		}
	}
}

// TestNoDefaultExceedsTheCeiling holds the invariant that makes the
// ceiling coherent: a value the supervisor would refuse if declared
// must never be handed out as a default.
func TestNoDefaultExceedsTheCeiling(t *testing.T) {
	for _, lang := range []string{"go", "python", "never-measured", ""} {
		if got := DefaultReadinessBudget(lang); got > Ceiling {
			t.Errorf("%q: default readiness %s exceeds the ceiling %s", lang, got, Ceiling)
		}
	}
}

// TestUnmeasuredLanguageGetsTheSlowestMeasured records the deliberate
// choice: no evidence means the most generous known answer, not the
// floor and not a zero.
func TestUnmeasuredLanguageGetsTheSlowestMeasured(t *testing.T) {
	slowest := "python"
	for lang := range firstFrameP95 {
		if firstFrameP95[lang] > firstFrameP95[slowest] {
			slowest = lang
		}
	}
	if got, want := SilenceBudget("rust"), SilenceBudget(slowest); got != want {
		t.Errorf("unmeasured language silence budget = %s, want the slowest measured (%s) %s", got, slowest, want)
	}
	if got, want := DefaultReadinessBudget("rust"), DefaultReadinessBudget(slowest); got != want {
		t.Errorf("unmeasured language readiness budget = %s, want the slowest measured (%s) %s", got, slowest, want)
	}
}

// TestLangIsNormalised keeps a descriptor's casing from silently
// costing an agent its measured entry.
func TestLangIsNormalised(t *testing.T) {
	if got, want := SilenceBudget(" Python "), SilenceBudget("python"); got != want {
		t.Errorf("SilenceBudget(%q) = %s, want %s", " Python ", got, want)
	}
}

// TestResolveDistinguishesAbsentFromDeclared is why the parameter is a
// pointer: absence means "use the derived default" and must not be
// expressible as a value.
func TestResolveDistinguishesAbsentFromDeclared(t *testing.T) {
	if got, want := Resolve("go", nil), DefaultReadinessBudget("go"); got != want {
		t.Errorf("Resolve with no declaration = %s, want the default %s", got, want)
	}
	declared := 3 * time.Second
	if got := Resolve("go", &declared); got != declared {
		t.Errorf("Resolve with a declaration = %s, want the declared %s", got, declared)
	}
}

// TestCheckDeclaredRefusesAboveTheCeiling is task 3's gate at the level
// of the derivation. The error must be matchable as data and must name
// all three facts an operator needs.
func TestCheckDeclaredRefusesAboveTheCeiling(t *testing.T) {
	err := CheckDeclared("boot-hog", 24*time.Hour)
	if err == nil {
		t.Fatal("a 24h declaration was accepted; decision 43(2) says one agent must not be able to hold a boot phase open")
	}
	var above *AboveCeiling
	if !errors.As(err, &above) {
		t.Fatalf("expected *AboveCeiling, got %T: %v", err, err)
	}
	if above.AgentID != "boot-hog" {
		t.Errorf("AgentID = %q, want %q", above.AgentID, "boot-hog")
	}
	if above.Declared != 24*time.Hour {
		t.Errorf("Declared = %s, want %s", above.Declared, 24*time.Hour)
	}
	if above.Ceiling != Ceiling {
		t.Errorf("Ceiling = %s, want %s", above.Ceiling, Ceiling)
	}
}

func TestCheckDeclaredRefusesNonPositive(t *testing.T) {
	for _, d := range []time.Duration{0, -1 * time.Second} {
		var np *NotPositive
		if err := CheckDeclared("zero-agent", d); !errors.As(err, &np) {
			t.Errorf("CheckDeclared(%s) = %v, want *NotPositive", d, err)
		}
	}
}

func TestCheckDeclaredAcceptsTheCeilingItself(t *testing.T) {
	if err := CheckDeclared("edge", Ceiling); err != nil {
		t.Errorf("the ceiling itself must be declarable, got %v", err)
	}
}
