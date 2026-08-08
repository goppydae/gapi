// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"fmt"
	"time"

	"github.com/goppydae/gapi/core/budget"
)

// MalformedReadinessBudget is a declared budget that is not a duration,
// as data. It carries the text the author actually wrote, because
// "invalid duration" without the offending string sends an operator
// back to the descriptor to guess which field was meant.
type MalformedReadinessBudget struct {
	AgentID  string
	Declared string
	Err      error
}

func (e *MalformedReadinessBudget) Error() string {
	return fmt.Sprintf("agent %s declares readiness_budget %q, which is not a duration: %v (want a Go duration such as \"30s\")",
		e.AgentID, e.Declared, e.Err)
}

func (e *MalformedReadinessBudget) Unwrap() error { return e.Err }

// ParseReadinessBudget resolves a descriptor's declared readiness
// budget, refusing one the supervisor will not honour.
//
// PARSED ONCE, AT THE BOUNDARY, per design/adk-architecture.md. This is
// the single spelling of the rule: ValidateAgentDescribe calls it to
// refuse, and discovery calls it to get the value. Two parsers would be
// two chances to disagree about what "30s" means, which is the class of
// drift GAPI-DIV-115 was filed for.
//
// ABSENT IS NOT ZERO, so the result is a pointer. A descriptor with no
// readiness_budget field returns (nil, nil) and means "use the derived
// default for my language" - a distinction a bare time.Duration cannot
// carry, and the same reason AgentDescribe.Enabled is a *bool.
//
// THE REFUSAL IS HERE, AT DECLARATION TIME, AND NOT AT START. The exit
// calls for "a declaration-time refusal of a budget above the ceiling",
// which is the difference between a config that cannot be valid failing
// to build and one failing at 3am.
func ParseReadinessBudget(desc AgentDescribe) (*time.Duration, error) {
	if desc.ReadinessBudget == nil {
		return nil, nil
	}

	d, err := time.ParseDuration(*desc.ReadinessBudget)
	if err != nil {
		return nil, &MalformedReadinessBudget{
			AgentID:  desc.ID,
			Declared: *desc.ReadinessBudget,
			Err:      err,
		}
	}

	if err := budget.CheckDeclared(desc.ID, d); err != nil {
		return nil, err
	}

	return &d, nil
}
