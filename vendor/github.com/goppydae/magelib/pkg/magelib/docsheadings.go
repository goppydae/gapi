// Copyright (c) 2026 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package magelib

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// h1Open matches an opening <h1> tag in rendered HTML.
//
// TOLERANT OF MINIFICATION BY CONSTRUCTION, because that is how the
// first version of this check lied. Hugo runs with --minify, which emits
// unquoted attributes - `<h1 id=x>` rather than `<h1 id="x">` - and a
// pattern written against the pretty form silently matches nothing. A
// check that counts zero headings on every page reports every page as a
// violation or as clean depending on which way its comparison runs, and
// either way it is not measuring what it claims to.
var h1Open = regexp.MustCompile(`(?i)<h1[\s>]`)

// pageH1 is one rendered page and how many h1 elements it emits.
type pageH1 struct {
	Path  string
	Count int
}

// CheckRenderedH1 asserts that every rendered page carries exactly one
// h1, and it reads the RENDERED SITE rather than the markdown.
//
// THE MARKDOWN CANNOT ANSWER THIS QUESTION, which is why the defect
// survived so long. A theme emits a title heading the source never
// mentions, and a source heading may be a `#` comment inside a fenced
// code block that no reader would call a heading. Counting `^# ` across
// content/ gets both wrong in opposite directions - measured on the
// documentation hub, where a source count said one file had multiple
// headings and the rendered count said forty-three did.
//
// DESIGN-INDEPENDENT ON PURPOSE. It asserts the INVARIANT - exactly one
// h1 per page - and not which source satisfies it, so it holds under
// either resolution of the underlying question: front matter owning the
// title with no body `#`, or the body owning it with the theme's title
// suppressed. Filing and building the gate was therefore never blocked
// on deciding that.
//
// It renders into a TEMPORARY destination rather than reading
// docs/public, for the same reason CheckDocsDrift regenerates into a
// temporary tree: a gate that reads a directory somebody else produced
// is asserting something about that directory's age, not about the
// repository.
func CheckRenderedH1(cfg DocsConfig) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "magelib-h1-")
	if err != nil {
		return fmt.Errorf("h1 check: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// SYNC BUT DO NOT GENERATE, and both halves were learned the hard way.
	//
	// The first version called DocsGenerate, so a CHECK rewrote committed
	// files as a side effect and the drift gate then reported staleness
	// this check had just caused. The second version removed that call
	// entirely and broke in CI with "inspected ZERO rendered pages":
	// DocsGenerate is DocsSync plus generation, and DocsSync is what
	// materialises docs/.magelib - which is GITIGNORED, so a fresh
	// checkout has no hugo config at all and renders nothing. It passed
	// locally only because a previous build had left the directory there.
	//
	// DocsSync alone gives both properties: the assets exist, and nothing
	// TRACKED is touched. Freshness of the generated CONTENT is carried
	// by CheckDocs running CheckDocsDrift first.
	if err := DocsSync(cfg); err != nil {
		return fmt.Errorf("h1 check: sync assets: %w", err)
	}
	args := append(hugoArgs(cfg), "--minify", "--destination", tmp)
	if err := runStreamed("hugo", args...); err != nil {
		return fmt.Errorf("h1 check: render: %w", err)
	}

	pages, err := renderedPages(tmp)
	if err != nil {
		return err
	}

	// FAILS ON ZERO PAGES, AND THE CLAUSE IS NOT DEFENSIVE PADDING. A
	// walker that silently matches nothing reports success for the rest
	// of its life - the omit-rather-than-error mode MAGELIB-DIV-011
	// names, and the shape this toolchain has now shipped more than
	// once. If the render moves, the output format changes, or the tag
	// pattern stops matching, this must be loud rather than green.
	if len(pages) == 0 {
		return fmt.Errorf(
			"h1 check inspected ZERO rendered pages under %s: it cannot have verified anything. "+
				"Either the site produced no HTML, or this walker no longer finds it", cfg.Dir)
	}

	var bad []pageH1
	for _, p := range pages {
		if p.Count != 1 {
			bad = append(bad, p)
		}
	}
	if len(bad) == 0 {
		fmt.Printf("h1 check: %d rendered pages, each with exactly one h1\n", len(pages))
		return nil
	}

	sort.Slice(bad, func(i, j int) bool { return bad[i].Path < bad[j].Path })
	var b strings.Builder
	fmt.Fprintf(&b, "%d of %d rendered pages do not carry exactly one h1 (MAGELIB-DIV-012):",
		len(bad), len(pages))
	for _, p := range bad {
		fmt.Fprintf(&b, "\n  %d h1  %s", p.Count, p.Path)
	}
	return fmt.Errorf("%s", b.String())
}

// renderedPages walks a rendered site and counts h1 elements per page.
//
// PRINT AND TAXONOMY OUTPUTS ARE SKIPPED, and each exclusion is a
// judgement rather than convenience. A print view is an alternate
// rendering of a page already counted, so including it double-counts one
// document's compliance. Taxonomy indexes are generated wholly by the
// theme from terms rather than authored, so a violation there is not
// something a repository's content can fix - which would make the gate
// unactionable in exactly the place it fires.
func renderedPages(root string) ([]pageH1, error) {
	var out []pageH1
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".html" {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		slash := filepath.ToSlash(rel)
		if strings.Contains(slash, "/categories/") || strings.Contains(slash, "/tags/") ||
			strings.HasPrefix(slash, "categories/") || strings.HasPrefix(slash, "tags/") ||
			strings.Contains(slash, ".print.") || slash == "404.html" {
			return nil
		}
		// #nosec G304,G122 -- root is a temp directory this process just
		// created with MkdirTemp and renders into; nothing outside the
		// call plants entries in it, so the symlink-TOCTOU window G122
		// warns about has no reachable attacker.
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out = append(out, pageH1{Path: slash, Count: len(h1Open.FindAll(b, -1))})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("h1 check: walking %s: %w", root, err)
	}
	return out, nil
}
