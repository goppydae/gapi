// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

// Package budget derives the supervisor's start deadlines from one
// measured table (GAPI-DIV-107).
//
// ONE CONSTANT USED TO BOUND TWO DEADLINES THAT BEHAVE NOTHING ALIKE.
// core/lifecycle.Controller.WaitStart was 10s for every agent of every
// language and type, and it expressed only the second of the two
// intervals a start actually contains:
//
//   - exec to first control frame. The ADK coming up. The supervisor
//     CAN know it, it barely varies, and it is language-dependent.
//   - first frame to RUNNING. The agent's own start(). Application
//     code the supervisor has never seen, and therefore unbounded.
//
// Two phenomena, so two policy multipliers over ONE measured table
// (operator decision 63). A second measured table would be the
// duplication decision 43(1) forbids - "reusing (3)'s derivation rather
// than duplicating it" is honoured by the floor, not by the factor. A
// single shared value would make decision 43(3)'s "much tighter"
// vacuous by making the two numbers equal, which re-creates exactly the
// defect this entry names.
//
// This package imports nothing but the standard library, on purpose.
// core/lifecycle owns the waits, core/agentmgr owns the instrument, and
// core/schema owns the declaration; all three need the derivation, and
// a leaf package adds no edge between any of them.
package budget

import (
	"fmt"
	"strings"
	"time"
)

// firstFrameP95 is the MEASURED floor, and the only measured input in
// this package. Everything else here is policy applied to it.
//
// PROVENANCE, because this entry rests entirely on being able to tell a
// measured constant from an invented one:
//
//	date     measured 2026-08-06; recorded in gapi divergence.jsonl
//	         under GAPI-DIV-107
//	sample   40 runs of real agents through the real runners,
//	         40 of 40 speaking, zero failures
//	host     ONE host, ONE build, UNLOADED. Not a CI runner, and not
//	         under contention.
//	go       median 1.069ms  min 0.783ms  max 1.365ms  sd 0.155ms
//	         p95 1.248ms
//	python   median 34.603ms min 32.816ms max 38.710ms sd 1.473ms
//	         p95 37.237ms
//
// Python is 32.4x Go at the median, which is why the derivation is
// per-language or it is wrong for one of them.
//
// THE NARROWNESS OF THE SAMPLE IS WHY THE FACTORS ARE LARGE, and that
// is a constraint rather than a note. One unloaded host is precisely
// the case that cannot see a CI runner's second mode: GAPI-DIV-120
// measures test/adk's own PASSING distribution at median 189s sd 14s
// and still caught a 552s run on the same infrastructure. A tight
// factor over this sample would produce a number that LOOKS measured
// and is not.
var firstFrameP95 = map[string]time.Duration{
	"go":     1248 * time.Microsecond,
	"python": 37237 * time.Microsecond,
}

const (
	// factorSilence and factorReadiness are POLICY (operator decisions
	// 63 and 66), not measurement. They are the two multipliers over the
	// one table above, and factorReadiness is the larger because the
	// interval it bounds contains the agent's own start().
	factorSilence   = 100
	factorReadiness = 1000

	// floorSilence exists so Go's sub-millisecond p95 cannot produce a
	// budget smaller than process scheduling noise. It bounds EXACTLY
	// what was measured - exec to first frame - and Go's worst observed
	// first frame is 1.365ms, so this is 183x headroom over the slowest
	// sample rather than a guess.
	floorSilence = 250 * time.Millisecond

	// floorReadiness IS 10s AND THE VALUE IS LOAD-BEARING. It is not a
	// scheduling-noise floor like floorSilence; it is the value this
	// budget REPLACES.
	//
	// WaitStart is 10s for every agent today. A lower floor - 2s was
	// proposed and rejected as decision 66 - would TIGHTEN Go by 5x on
	// infrastructure GAPI-DIV-120 has measured producing second modes,
	// which is a new way to fail that nobody asked for. At 10s this
	// change only ever LOOSENS, so it cannot introduce a start-timeout
	// failure that did not already exist.
	//
	// The asymmetry with floorSilence is the whole point of two factors:
	// silence bounds a measured interval, readiness also covers the
	// agent's own start(), which is unbounded user code.
	floorReadiness = 10 * time.Second

	// Ceiling is the supervisor-owned MAXIMUM a descriptor may declare.
	// Decision 43(2) is explicit that this is not a default and must not
	// be spelled as one: boot targets (decision 5) mean a single agent
	// declaring 24h holds a phase open forever.
	//
	// The value is 1.6x the slowest derived default (python, 37.2s), so
	// no language's default can ever exceed it, while still admitting an
	// agent that is genuinely slow to become ready. It is deliberately
	// well under core/config.TestAgentStartTimeout's 120s, which leaves
	// the harness slack by construction - see that constant.
	Ceiling = 60 * time.Second

	// Spawn bounds the runner's Start CALL: fork, exec, pipe and socket
	// setup. It is not the readiness budget and it is not the silence
	// budget.
	//
	// PRESERVED, NOT DERIVED. This entry measured exec-to-first-frame
	// and first-frame-to-RUNNING; it did not measure the exec call
	// itself, so there is no measurement here to derive from. WaitStart
	// was doing this job as a second job (see core/lifecycle), and
	// carrying its 10s across unchanged is the only move that adds no
	// invented number and changes no behaviour. It is deliberately NOT
	// per-agent: a descriptor declaring a short readiness budget is
	// asking for its own start() to be judged sooner, not for fork/exec
	// to be given less time.
	Spawn = 10 * time.Second
)

// SilenceBudget is how long a spawned child may write NOTHING to its
// control descriptor before the supervisor calls it silent rather than
// slow (GAPI-DIV-104's discriminator, GAPI-DIV-107's tighter deadline).
func SilenceBudget(lang string) time.Duration {
	return derive(lang, factorSilence, floorSilence)
}

// DefaultReadinessBudget is what an agent that declares nothing gets:
// the per-language measured floor times the safety factor, per decision
// 43(1) as amended by decision 51. Declaring is how an agent asks for
// something else.
func DefaultReadinessBudget(lang string) time.Duration {
	return derive(lang, factorReadiness, floorReadiness)
}

// Resolve is the one place a declared budget becomes an effective one.
// A nil declaration means ABSENT, which is distinct from zero and is
// why the parameter is a pointer.
func Resolve(lang string, declared *time.Duration) time.Duration {
	if declared != nil {
		return *declared
	}
	return DefaultReadinessBudget(lang)
}

// derive is the shared derivation both budgets go through. Both read
// measuredP95, so a change to the table moves both or neither.
func derive(lang string, factor int, floor time.Duration) time.Duration {
	d := measuredP95(lang) * time.Duration(factor)
	if d < floor {
		return floor
	}
	return d
}

// measuredP95 reports the measured p95 for lang, or the SLOWEST
// measured language for one the table has never seen.
//
// The slowest rather than the fastest, and rather than a zero that
// would fall through to the floors: an unmeasured language is one this
// project has no evidence about, and guessing low would tighten a
// deadline on the strength of no measurement at all. Guessing high
// costs a slower failure for an agent that was going to fail anyway.
func measuredP95(lang string) time.Duration {
	if p95, ok := firstFrameP95[strings.ToLower(strings.TrimSpace(lang))]; ok {
		return p95
	}
	var slowest time.Duration
	for _, p95 := range firstFrameP95 {
		if p95 > slowest {
			slowest = p95
		}
	}
	return slowest
}

// AboveCeiling is a declared readiness budget exceeding the
// supervisor's maximum, as data rather than as a sentence. It names the
// declared value, the ceiling and the agent, because an operator
// reading it needs to know which agent to edit and what the bound is.
type AboveCeiling struct {
	AgentID  string
	Declared time.Duration
	Ceiling  time.Duration
}

func (e *AboveCeiling) Error() string {
	return fmt.Sprintf(
		"agent %s declares a readiness budget of %s, above the supervisor ceiling of %s: a boot phase cannot be held open that long by one agent",
		e.AgentID, e.Declared, e.Ceiling)
}

// NotPositive is a declared readiness budget of zero or less. Zero is
// not absence - absence is the field not being there at all - so a
// declared zero is an author saying something they cannot have meant.
type NotPositive struct {
	AgentID  string
	Declared time.Duration
}

func (e *NotPositive) Error() string {
	return fmt.Sprintf(
		"agent %s declares a readiness budget of %s: a budget must be positive, and omitting the field is how an agent asks for the default",
		e.AgentID, e.Declared)
}

// CheckDeclared refuses a declaration the supervisor will not honour.
//
// AT DECLARATION TIME, NOT AT START. The exit calls for "a
// declaration-time refusal of a budget above the ceiling", and that is
// the difference between a config that cannot be valid failing to build
// and one failing at 3am.
func CheckDeclared(agentID string, declared time.Duration) error {
	if declared <= 0 {
		return &NotPositive{AgentID: agentID, Declared: declared}
	}
	if declared > Ceiling {
		return &AboveCeiling{AgentID: agentID, Declared: declared, Ceiling: Ceiling}
	}
	return nil
}
