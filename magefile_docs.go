// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

//go:build mage
// +build mage

// This file carries the docs LOCATION gate and nothing else.
//
// It is a second mage file rather than more of Magefile.go because that
// file is already a length waiver at 879 lines, and its waiver comment
// records the operator decision that the exit is to SHORTEN it, not to
// grow it. mage compiles every build-tagged file in the directory, so a
// split costs nothing and needs no waiver of its own.

package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// docsRoot is the tree the gate walks.
	docsRoot = "docs"

	// docsPublished is the ONLY subtree Hugo reads. Everything else
	// under docs/ is either generated non-markdown output (man pages,
	// defaults.json), module metadata, or a page nothing builds.
	docsPublished = "docs/content"
)

// docsLocationSkips are directories holding RENDERED or MATERIALISED
// output rather than source. All three are gitignored, so they are
// absent on a fresh clone and present the moment anyone runs
// `mage docs:build` or `mage docs:sync` - which means a gate that did
// not skip them would pass in CI and fail on a developer's machine
// immediately after the documented build command. That asymmetry is
// worse than no gate, because it teaches people the gate is noise.
var docsLocationSkips = []string{
	"docs/public",
	"docs/.magelib",
	"docs/resources",
}

// checkDocsLocation fails when a markdown page under docs/ sits outside
// docs/content/, naming every such file.
//
// GAPI-DIV-121: seven mkdocs-era pages survived the deletion of
// mkdocs.yml and Docs.Html. Hugo reads only docs/content, so 1724 lines
// were built by nothing, linked by nothing and checked by nothing - and
// one of them, docs/configuration.md, shadowed the GENERATED
// docs/content/reference/configuration.md by name while being four
// times its size and held to no source at all.
//
// The rule is deliberately NARROW. It judges LOCATION, never content:
// markdown only, so docs/go.mod, docs/go.sum, the generated roff under
// docs/man and docs/data/defaults.json are all outside its reach by
// construction rather than by exemption. A page that moves under
// docs/content/ is published and rendered; whether it is TRUE is the
// business of mage docs:check and of the reader, not of this gate.
//
// Inspecting zero files is a FAILURE, not a pass. A walker that matches
// nothing and reports success is this toolchain's repeat defect - the
// unanchored `man/` ignore pattern silently excluded 46 generated man
// pages while docs:check stayed green, and goblin's old Docs:Man target
// skipped a missing source in silence for months. A gate that cannot
// tell "clean" from "I found nothing to look at" asserts nothing.
func checkDocsLocation() error {
	var stray []string
	inspected := 0

	walkErr := filepath.WalkDir(docsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		slashed := filepath.ToSlash(path)

		if d.IsDir() {
			for _, skip := range docsLocationSkips {
				if slashed == skip {
					return fs.SkipDir
				}
			}
			return nil
		}

		if !strings.EqualFold(filepath.Ext(slashed), ".md") {
			return nil
		}
		inspected++

		if slashed != docsPublished && !strings.HasPrefix(slashed, docsPublished+"/") {
			stray = append(stray, slashed)
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("docs location: walking %s/: %w", docsRoot, walkErr)
	}

	if inspected == 0 {
		return fmt.Errorf(
			"docs location: inspected 0 markdown files under %s/ - the tree is "+
				"missing, empty, or entirely skipped, and this gate asserts "+
				"nothing until it reads at least one page",
			docsRoot)
	}

	if len(stray) > 0 {
		sort.Strings(stray)
		var b strings.Builder
		fmt.Fprintf(&b, "docs location: %d of %d markdown page(s) under %s/ are not under %s/, "+
			"so Hugo builds none of them:\n", len(stray), inspected, docsRoot, docsPublished)
		for _, p := range stray {
			fmt.Fprintf(&b, "\t%s\n", p)
		}
		b.WriteString("move each page under " + docsPublished + "/ or delete it; " +
			"where it duplicates generated reference, the generated page wins")
		return fmt.Errorf("%s", b.String())
	}

	fmt.Printf("docs location: %d markdown page(s) under %s/, all published under %s/\n",
		inspected, docsRoot, docsPublished)
	return nil
}
