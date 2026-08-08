// Copyright (c) 2026 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package magelib

import (
	"bytes"
	"fmt"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TWO CEILINGS, BECAUSE ONE NUMBER CANNOT MEASURE WHAT THE RULE IS FOR.
//
// The rule exists for COGNITIVE LOAD: a file should be small enough to
// hold as one mental model. Those two numbers bound the two things that
// cost a reader, and they are not the same thing.
//
// maxSourceLines bounds the MODEL - the state and control flow a reader
// has to simulate. Comments do not add to that; they reduce it, which is
// why counting them as code taxed exactly the explanations this codebase
// is built on (MAGELIB-DIV-014). Measured across all three repos before
// choosing: 464 hand-written files, median 93 source lines, p90 221,
// p99 416, max 602. 400 sits at p99, so it argues with the top one
// percent and nothing else.
//
// maxRawLines bounds the READING - the sheer length of what opens in the
// editor, which prose does add to. It exists because a source ceiling
// alone places NO bound on that: nothing in a 400-source limit prevents
// a 3000-line file. Today's corpus is well behaved (median 144 raw, p99
// 612, max 891) so this binds nothing; it is a backstop against a future
// where it would. 1000 is not invented - operator decision 23 already
// states magefiles may grow to 1000 lines before splitting is looked at.
//
// SOURCE COUNTING IS MORE PERMISSIVE THAN wc -l AND THAT WAS TAKEN
// DELIBERATELY. A 500-line raw ceiling was allowing about 318 source
// lines at the median 36% prose density, so 400 is a real loosening of
// roughly a quarter, accepted because it is measuring the right quantity
// rather than because the old number was wrong.
//
// Package constants on purpose: a per-repo limit is exactly the drift
// that makes a shared rule meaningless. Debt is expressed through the
// waiver list instead, which names individual files, shrinks, and cannot
// rot (see CheckFileLength).
//
// Both gates fail strictly ABOVE their number: a 400-source file passes,
// a 401-source file fails.
const (
	maxSourceLines = 400
	maxRawLines    = 1000
)

// CheckFileLength fails when any hand-written Go file in the repo
// rooted at the current directory breaches either ceiling - more than
// maxSourceLines of source, or more than maxRawLines to read. Like
// CheckTerminology it is meant to be wired into a Lint target with
// mg.Deps, so it rides a CI context that already blocks merges rather
// than becoming a target nobody invokes - the failure mode GAPI-DIV-030
// and MAGELIB-DIV-001 both recorded.
//
// Scope follows the manifesto's own carve-outs and nothing else:
//
//   - Only .go files are counted. Data, config and docs are exempt by
//     the rule; other languages are out of scope until someone teaches
//     this gate their comment syntax.
//   - Vendored dependencies and build output are skipped through the
//     shared skipDirs, so this gate and every other walk in magelib
//     agree on what "the project's code" means.
//   - Generated code is skipped (see generatedMarker).
//   - TEST FILES COUNT. The manifesto says "hand-written project code"
//     and exempts generated, vendored and data files; a _test.go file
//     is none of those. A test that has outgrown 500 lines is testing
//     more than one thing, which is the same cohesion signal the rule
//     exists to raise everywhere else.
//
// # Waivers and skips are different things and must not be conflated
//
// A WAIVER is hand-written code that violates the rule and is accepted
// for now. It is DEBT, and it must shrink. The rule applies to it, the
// file is measured, and the gate stays quiet only until the file is
// fixed. Waivers exist so the gate can arrive repo-wide instead of
// checking only changed files, which would leave the same debt
// invisible and unbounded. Two rules keep the list honest, and both are
// failures rather than warnings:
//
//   - A waiver whose file is now at or under the limit FAILS, naming
//     the waiver to delete. Without it a fix silently leaves its waiver
//     behind and the list becomes a permanent carve-out.
//   - A waiver that names nothing this gate measured FAILS. It is a
//     typo, a deleted file, or a path the walk never reaches, and in
//     every case the list is lying about what it covers.
//
// A SKIP is a path the rule does not apply to at all:
// generated output whose header the marker does not recognise, vendored
// trees outside skipDirs, data that happens to carry a .go suffix. It
// is an EXEMPTION THE RULE ITSELF GRANTS, not debt. It will never
// shrink, and it is never measured - so it can never be reported as a
// violation and is never subject to the stale-waiver failure. Failing a
// skip because its file came under the limit would be nonsense: nobody
// ever promised it would.
//
// Forcing an exempt file through the waiver list would destroy the
// property the design rests on. gapi's gopy-generated adk.go is the
// case that proves it: at 1312 lines it can never come under the limit,
// so as a waiver it would sit there permanently, undeletable, and a
// reader could not tell it apart from the five real violations that are
// supposed to disappear. The list would no longer only shrink.
//
// A path may therefore be a waiver or a skip, never both. Claiming both
// is a contradiction about which of the two it is, and it is a config
// error.
//
// Skips are the shared Skip type, validated by compileSkips exactly as
// CheckTerminology's are - one parser, so the two gates cannot drift
// apart on what a usable skip is. A skip carries a required reason,
// which is what makes the waiver/skip classification auditable at the
// point it is made. Waivers are validated here, by repoRelative, since
// they are this gate's alone: an absolute or escaping waiver is a config
// ERROR for the same reason a bad skip is, because it could only ever
// match nothing.
//
// Every violation is reported, not just the first: a gate that stops at
// the first hit turns one fix into N build cycles.
//
// Unreadable files are an ERROR, not a skip: a gate that cannot read a
// file must not report it clean.
func CheckFileLength(waivers []string, skips ...Skip) error {
	waived := make(map[string]bool, len(waivers))
	for _, w := range waivers {
		cleaned, err := repoRelative("waiver", w)
		if err != nil {
			return err
		}
		waived[cleaned] = true
	}
	skipPaths, skipNames, err := compileSkips("file length check", skips)
	if err != nil {
		return err
	}
	// A path claimed as both is not a redundancy to resolve silently:
	// the two mechanisms make opposite promises about the file's future,
	// and choosing one for the caller would hide which promise is in
	// force. The skip would win mechanically, because it prunes before
	// anything is measured, and the waiver would then be permanently
	// unfalsifiable - exactly the rot the stale-waiver rule exists to
	// prevent.
	for w := range waived {
		if skipPaths[w] || skipNames[filepath.Base(w)] {
			return fmt.Errorf(
				"file length check: %q is listed as both a waiver and a skip; a waiver is debt the rule applies to, a skip is an exemption the rule grants - pick one",
				w)
		}
	}

	var violations []string
	// checked records the line count of every waived file the walk
	// actually measured, which is what lets a stale or bogus waiver be
	// told apart from a live one after the walk.
	checked := make(map[string]int, len(waived))
	checkedRaw := make(map[string]int, len(waived))

	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if skipPaths[filepath.Clean(path)] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if skipDirs[d.Name()] || skipNames[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("file length check: stat %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		// #nosec G122,G304 -- walking the repo we were invoked in; WalkDir does
		// not follow symlinks and non-regular files are skipped above.
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("file length check: read %s: %w", path, err)
		}
		if isGenerated(data) {
			return nil
		}
		rel := filepath.Clean(path)
		src, raw := sourceLines(data), countLines(data)
		if waived[rel] {
			checked[rel] = src
			checkedRaw[rel] = raw
			return nil
		}
		if src > maxSourceLines {
			violations = append(violations, fmt.Sprintf(
				"  %s: %d source lines (limit %d)", rel, src, maxSourceLines))
		}
		if raw > maxRawLines {
			violations = append(violations, fmt.Sprintf(
				"  %s: %d lines to read (limit %d)", rel, raw, maxRawLines))
		}
		return nil
	})
	if err != nil {
		return err
	}

	for w := range waived {
		n, measured := checked[w]
		switch {
		// A waiver is stale only when the file clears BOTH ceilings.
		// Reporting it stale while it still breaches the other would ask
		// for a deletion that immediately turns the gate red.
		case measured && n <= maxSourceLines && checkedRaw[w] <= maxRawLines:
			violations = append(violations, fmt.Sprintf(
				"  waiver %q: file is now %d source lines, at or under the limit %d - delete the waiver",
				w, n, maxSourceLines))
		case !measured:
			violations = append(violations, fmt.Sprintf(
				"  waiver %q: %s - delete the waiver", w, waiverMiss(w)))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("file length check failed:\n%s", strings.Join(violations, "\n"))
	}
	return nil
}

// repoRelative validates a configured path and returns it cleaned. Same
// argument as CheckTerminology's skip validation: an entry that could
// never match any walked path is silently inert, and silence from a
// gate is indistinguishable from a clean tree. kind names the list the
// entry came from, so the operator knows which one to edit.
func repoRelative(kind, p string) (string, error) {
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("file length check: %s %q must be repo-relative, not absolute", kind, p)
	}
	cleaned := filepath.Clean(p)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("file length check: %s %q does not name a path inside the repo", kind, p)
	}
	return cleaned, nil
}

// waiverMiss explains why a waiver matched nothing. The distinction is
// worth a stat call: "no such file" means a typo or a deleted file,
// while an existing path means the waiver is aimed somewhere this gate
// does not look and was never doing anything.
func waiverMiss(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "no such file"
	}
	return "names no Go file this gate measures (generated, skipped, or not .go)"
}

// countLines counts lines the way wc -l does, plus a final partial
// line when the file does not end in a newline. gofmt always emits the
// trailing newline, so the second clause only ever fires on a file that
// is already unformatted - but a gate must not undercount by one on
// input it did not expect.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte("\n"))
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// sourceLines counts lines carrying at least one NON-COMMENT token.
//
// A CALCULATION, NOT A FILTER, AND THE ENTRY INSISTS ON THAT FOR A
// REASON. Any regex reaching for `^\s*//` gets two cases wrong: a "//"
// inside a STRING LITERAL is not a comment, and a /* */ BLOCK spanning
// lines is one without any line starting that way. go/scanner tokenises
// the file, so both fall out of the language's own definition rather
// than out of a pattern somebody hoped was equivalent.
//
// A raw string literal spanning lines counts as source throughout: those
// lines are data the program carries, and a reader holding the file has
// to hold them.
func sourceLines(data []byte) int {
	fset := token.NewFileSet()
	file := fset.AddFile("f", fset.Base(), len(data))
	var s scanner.Scanner
	// Errors are ignored deliberately: this gate measures length, and a
	// file that does not parse is a compiler's problem rather than a
	// reason to report no violation.
	s.Init(file, data, func(token.Position, string) {}, scanner.ScanComments)

	lines := make(map[int]bool)
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.COMMENT {
			continue
		}
		p := fset.Position(pos)
		lines[p.Line] = true
		if tok == token.STRING && strings.Contains(lit, "\n") {
			for i := 0; i < strings.Count(lit, "\n"); i++ {
				lines[p.Line+i+1] = true
			}
		}
	}
	return len(lines)
}
