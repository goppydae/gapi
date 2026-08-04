// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package adk

import "github.com/goppydae/gapi/core/adk/meta"

// Agent is the standard contract for all supervised agents
type Agent interface {
	Initialize() error
	Start() error
	Stop() error
	Describe() *meta.AgentInfo
	Info() *meta.AgentInfo
}

// OptionalHooks can be optionally implemented by agents
type OptionalHooks interface {
	Restart() error
	Reload() error
}
