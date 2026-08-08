// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"time"

	"github.com/goppydae/gapi/core/budget"
	"github.com/goppydae/gapi/core/lifecycle"
	"github.com/goppydae/gapi/core/schema"
)

// The start budgets, from the runner that knows its language and the
// descriptor that may have declared one (GAPI-DIV-107).
//
// TWO NARROWINGS, IN ORDER. A controller is born with the derivation's
// answer for an unmeasured language, because it does not know what it
// is supervising. The runner narrows that to its own language the
// instant it is constructed, which is why a Python agent in a unit test
// gets Python's budget without discovery being involved. Discovery then
// narrows the readiness budget again, and only that one, if the
// descriptor declared a value.

// applyLanguageBudgets gives a freshly built controller the derivation
// for the language of the runner that owns it.
func applyLanguageBudgets(ctrl *lifecycle.Controller, lang string) {
	ctrl.ReadinessBudget = budget.DefaultReadinessBudget(lang)
	ctrl.SilenceBudget = budget.SilenceBudget(lang)
}

// readinessSetter is the optional-capability idiom this package already
// uses to carry a resolved descriptor value onto a constructed agent -
// see enabledSetter, and the note at its call site about not adding
// another positional argument to constructors that are already eleven
// parameters long.
type readinessSetter interface {
	SetReadinessBudget(time.Duration)
}

// SetReadinessBudget applies a declared readiness budget to this
// agent's controller. Only the readiness budget: the silence and spawn
// budgets are supervisor policy and are not declarable.
func (a *GoAgent) SetReadinessBudget(d time.Duration) { a.ctrl.ReadinessBudget = d }

// SetReadinessBudget is GoAgent's, for the other runner.
func (a *PythonAgent) SetReadinessBudget(d time.Duration) { a.ctrl.ReadinessBudget = d }

// resolveReadinessBudget turns a validated descriptor into the budget
// this agent's controller should honour.
//
// The descriptor has already been through ValidateAgentDescribe by the
// time this runs, so a parse error here cannot be a declaration the
// supervisor would have refused - it would have been refused at
// declaration time. It is still reported rather than swallowed: a
// second parse disagreeing with the first is a defect in the parser,
// not in the descriptor, and silently falling back to the default would
// hide it.
func resolveReadinessBudget(d schema.AgentDescribe, lang string) (time.Duration, error) {
	declared, err := schema.ParseReadinessBudget(d)
	if err != nil {
		return budget.DefaultReadinessBudget(lang), err
	}
	return budget.Resolve(lang, declared), nil
}
