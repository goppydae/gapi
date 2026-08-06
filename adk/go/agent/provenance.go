// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agent

import (
	"fmt"
	"os"
)

// SourceHash is the BLAKE3 hash of the assembled package this binary was
// built from, stamped at link time by `agent build` (GAPI-DIV-103).
//
// IT IS DECLARED HERE BECAUSE `-X` FAILS SILENTLY. The build already
// passed `-X main.SourceHash=<hash>`, but the generated main declared no
// such variable, and the Go linker ignores `-X` for a symbol that does
// not exist - no error, no warning. The value was computed over the
// stage, formatted into a flag, and dropped on the floor, while the code
// that did it read as if the binary were stamped. A variable that
// exists, in a package the build names by a constant, is what makes the
// flag's target checkable rather than hopeful.
//
// It is empty in any build that did not come from `agent build` - a
// developer running `go build ./...` over the shipped ADK source, for
// one. That is a legitimate state and is reported as such, never as a
// hash.
var SourceHash string

// emitProvenance prints the stamped source hash, or fails loudly.
//
// AN UNSTAMPED BINARY IS NOT A BINARY WITH AN UNKNOWN HASH. Printing
// "unknown" on stdout beside the digits would put a non-value where a
// caller comparing output expects a value, which is the confusion this
// whole entry is about. The absence goes to stderr with a non-zero
// exit, so a script asking a binary what it was built from cannot
// mistake silence for an answer.
func emitProvenance() int {
	if SourceHash == "" {
		fmt.Fprintln(os.Stderr,
			"no source hash: this binary was not produced by `agent build`")
		return 1
	}
	fmt.Println(SourceHash)
	return 0
}
