// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

//go:build tools

package tools

import (
	_ "github.com/go-python/gopy"
	_ "golang.org/x/tools/cmd/goimports"

	// gopyh is the RUNTIME half of gopy and needs its own anchor: the
	// gopy COMMAND above does not import it, so anchoring the command
	// does not bring it along.
	//
	// It is imported only by adk/python/gapi/native/adk.go, which
	// `gopy gen` writes at build time and which .gitignore excludes -
	// so no committed source references it, `go mod vendor` prunes it
	// as unreachable, and the next Python ADK build fails with "cannot
	// find module providing package github.com/go-python/gopy/gopyh:
	// import lookup disabled by -mod=vendor".
	//
	// It had survived in vendor/ only because nothing re-ran
	// `go mod vendor` after it was first added; the first re-vendor to
	// follow deleted it and broke the build (GAPI-DIV-080).
	//
	// The anchor belongs HERE and not in tools/gopy/tools.go, which
	// looks like the natural home and is not: that directory is a
	// SEPARATE MODULE with its own go.mod, so this module's
	// `go mod vendor` never sees its imports.
	_ "github.com/go-python/gopy/gopyh"
)
