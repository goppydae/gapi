// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package version

import (
	"fmt"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

// Stamp points. A field is STAMPED at build time where the build can
// supply it and DERIVED where it cannot; a concrete stamp always wins
// (cli-contract.md, Field sources).
//
// THE OLD COMMENT HERE SAID "Injected at build time via -ldflags" AND
// EVERY INJECTION SITE SET EXACTLY ONE OF THESE (GAPI-DIV-126). The
// Magefile and both nix derivations passed a single -X for GAPIVersion,
// so five of eleven rows were placeholders in every build ever made.
// GoADKVersion and PythonADKVersion survived only because adkVersion
// falls back to KernelVersion - the two fields WITH a fallback were
// right in every build and the five without one were wrong in every
// build, which is what makes this one defect rather than five.
//
// THE TWO BUILD PATHS DIFFER IN WHAT THEY CAN SUPPLY AND NEITHER COVERS
// THE OTHER. A mage build has vcs.revision and vcs.time for free. A nix
// build has neither: the derivation builds from a source copy with no
// .git, under -trimpath, so the main module records "(devel)" and there
// are no vcs settings at all. A design using only one mechanism leaves
// one path reporting placeholders, which is the state this replaced.
var (
	GAPIVersion      = "dev"
	GoADKVersion     = "dev"
	PythonADKVersion = "dev"
	BuildTag         = "dev"
	SchemaHash       = "unknown"
	Commit           = "unknown"

	// SourceDate is the commit's time, not a wall clock. A build time
	// would make the nix derivation non-reproducible, so the value comes
	// from a fixed input - and a row holding the commit's time under a
	// label saying "built" is a quieter version of this same defect,
	// which is why the row is Source Date.
	SourceDate = "unknown"

	BuiltBy = "unknown"
)

// unknownValue is the placeholder a derivation must reject as an answer.
// Paired with devVersion below: the block keeps a constant shape, so an
// unresolvable field reads `unknown` rather than vanishing.
const unknownValue = "unknown"

// THESE TWO CONSTANTS SPELL THE SAME STRING AND MUST NOT BE MERGED
// BACK (GAPI-DIV-128). One is a display name, the other was a
// control-flow key, and their being a single value is what inverted
// this package's most visible rule.
//
// The old `runtimeCoreLabel` was both the fallback name for a binary
// that had registered no identity AND the key in `if name !=
// runtimeCoreLabel`. So the row was suppressed exactly when the binary
// was UNIDENTIFIED and never when it was the kernel - the opposite of
// the documented intent, reached by a condition that reads correctly.
// It was invisible while SetBinaryNameAndVersion had no callers,
// because the fallback fired for everyone; GAPI-DIV-056 gave it its
// first caller and turned the row on for every binary including gapid.
// A fix that satisfied its own exit inverted a neighbouring rule, and
// nothing could notice, because a redundant row is well-formed.
//
// The suppression now keys off a DECLARATION - see shipsKernel - and
// never off a label or a version string.

// unidentifiedBinaryLabel names the block when no binary has registered
// an identity. A display value, read by nothing that branches.
const unidentifiedBinaryLabel = "Runtime Core"

// embeddedKernelRowLabel is the label of the embedded-kernel row.
const embeddedKernelRowLabel = "Runtime Core"

// devVersion is the unstamped placeholder. It is a value the resolution
// must REJECT as an answer, not merely a default it happens to return.
const devVersion = "dev"

// buildInfoReader is a seam so the resolution can be tested at all.
//
// It exists because a TEST binary carries no dependency information:
// measured at len(BuildInfo.Deps) == 0 from goblin's internal/cli, which
// links 38 gapi packages, while the shipped binary records
// "dep github.com/goppydae/gapi v0.1.0-proto2f". Without the seam the
// dependency path is unreachable from any test, and an untestable
// resolution is how a version reports "dev" for an entire release.
var buildInfoReader = debug.ReadBuildInfo

// KernelVersion reports the version of the gapi kernel this binary
// embeds, which is not always this binary's own version: goblin vendors
// the kernel and stamps only internal/version.Version, so before this
// existed `goblind version` answered "dev" - in the release that first
// embedded the new kernel. See GAPI-DIV-066.
//
// Resolution order:
//
//  1. The stamped GAPIVersion, when it is not the placeholder. gapi's
//     own builds set it deliberately and must not be second-guessed.
//  2. The gapi entry in the binary's build info - as the main module for
//     gapi's binaries, as a dependency for goblin's.
//  3. The placeholder.
//
// Step 2 deliberately adds NO new stamp point. The value comes from the
// module graph, so it cannot drift from the module actually linked; an
// eighth hand-maintained ldflag would be the same defect class that
// MAGELIB-DIV-006 spent a release closing.
func KernelVersion() string {
	if isConcreteVersion(GAPIVersion) {
		return GAPIVersion
	}
	if bi, ok := buildInfoReader(); ok && bi != nil {
		if v := gapiVersionFrom(bi); v != "" {
			return v
		}
	}
	return devVersion
}

// gapiVersionFrom finds the kernel's own module in build info, without
// ever spelling its name.
//
// The kernel's module is, by construction, THE MODULE THAT CONTAINS THIS
// PACKAGE - so it is identified by asking rather than by a literal. Two
// reasons, and the first is a rule: goblind links this code and its
// operators have never heard of the vendor, so the kernel does not spell
// its own name in string literals (GAPI-DIV-061, enforced by
// core/product's scan). The second is that a derived value cannot drift
// if the module is ever renamed, where a literal silently stops matching
// and the version quietly reverts to the placeholder.
//
// The longest containing module path wins, because a parent path could
// also prefix this package without being the module actually linked.
//
// A replace directive is honoured when it names a version: the
// replacement is what was really built.
func gapiVersionFrom(bi *debug.BuildInfo) string {
	self := selfPackagePath()
	bestPath, bestVersion := "", ""

	consider := func(path, version string) {
		if !moduleContains(path, self) || len(path) <= len(bestPath) {
			return
		}
		bestPath, bestVersion = path, version
	}

	consider(bi.Main.Path, bi.Main.Version)
	for _, dep := range bi.Deps {
		if dep == nil {
			continue
		}
		version := dep.Version
		if dep.Replace != nil && isConcreteVersion(dep.Replace.Version) {
			version = dep.Replace.Version
		}
		consider(dep.Path, version)
	}

	if isConcreteVersion(bestVersion) {
		// Go module versions are canonically "v"-prefixed and build info
		// records them that way; the VERSION file and every stamped path
		// spell the version without it, and cli-contract.md fixes the
		// user-facing form as "Runtime Core: 0.1.0-proto2b". Leaving the
		// prefix on would make goblind and gapictl print one fact two
		// ways and put goblind in violation of that contract.
		//
		// Only the DERIVED value is normalised. GAPIVersion comes from the
		// VERSION file, which carries no prefix, so trimming it there
		// would quietly accept a malformed stamp instead of showing it.
		return strings.TrimPrefix(bestVersion, "v")
	}
	return ""
}

// selfPackagePath returns this package's import path, taken from a type
// declared in it rather than written down.
func selfPackagePath() string {
	return reflect.TypeOf(Info{}).PkgPath()
}

// moduleContains reports whether modulePath is the module holding pkgPath,
// matching on path segments so a shared prefix is not mistaken for one.
func moduleContains(modulePath, pkgPath string) bool {
	if modulePath == "" {
		return false
	}
	return pkgPath == modulePath || strings.HasPrefix(pkgPath, modulePath+"/")
}

// isConcreteVersion rejects the two ways the toolchain says "no version
// here". "(devel)" is what a workspace build records when the module is
// resolved from a sibling checkout - an honest answer to a different
// question, and reporting it as the embedded kernel would swap one
// misleading string for another.
func isConcreteVersion(v string) bool {
	return v != "" && v != devVersion && v != "(devel)"
}

// adkVersion resolves one ADK row. The ADKs ship inside gapi and are in
// lockstep today, so an unstamped row is not unknown - it is the
// kernel's version, and printing the placeholder discards a value we
// hold.
//
// The stamp still wins, and that is the point rather than an accident:
// the ADKs are designed to be able to ship independently for hotfixes,
// so this must stay a fallback. When that day comes, stamping one ADK is
// the only change required.
//
// Residual, recorded rather than solved: while lockstep holds, "nobody
// stamped it" and "correctly in lockstep" render identically. That is
// accepted only because both states have the same CORRECT value today;
// once an ADK diverges, a missing stamp becomes a visibly wrong number.
func adkVersion(stamped string) string {
	if isConcreteVersion(stamped) {
		return stamped
	}
	return KernelVersion()
}

type Info struct {
	Name      string
	Version   string
	Commit    string
	BuildDate string
	BuiltBy   string
	GoVersion string
	Platform  string
}

var (
	mu     sync.RWMutex
	active Info

	// shipsKernel is the invoking binary's own statement that it IS the
	// gapi kernel rather than a process embedding one.
	//
	// A DECLARATION AND NOT AN INFERENCE, which is the load-bearing part
	// (GAPI-DIV-128). The renderer must not decide this by comparing the
	// binary's version against the kernel's: they coincide whenever
	// goblin happens to tag a matching string, and the block's SHAPE
	// would then depend on an accident of two values while the contract
	// requires it to be constant.
	//
	// It is deliberately NOT a field of Info and is not touched by
	// SetBinaryNameAndVersion. Coupling the two is what made the old
	// behaviour depend on call order, and order-dependence is how a
	// display change silently becomes a control-flow change.
	//
	// False is the correct default: a binary that names itself and says
	// nothing further is a consumer, so goblind and goblinctl keep the
	// row without goblin changing a line.
	shipsKernel bool
)

func init() {
	active = Info{
		Commit:    resolveCommit(),
		BuildDate: resolveSourceDate(),
		BuiltBy:   BuiltBy,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// vcsSetting reads one build setting, which is present only when the
// build had a repository to read.
//
// It goes through buildInfoReader rather than debug.ReadBuildInfo for
// the reason that seam already exists: a TEST binary's build info does
// not resemble a shipped one, so a resolution reachable only through the
// real function is a resolution no test can drive.
func vcsSetting(key string) string {
	info, ok := buildInfoReader()
	if !ok || info == nil {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}

// resolveCommit answers the Commit row.
//
// Precedence is adkVersion's shape, deliberately: a concrete stamp wins
// and the derivation answers otherwise, so ONE resolution serves both
// build paths rather than two code paths that can disagree. The nix
// derivation stamps because it has no VCS data; mage does not need to.
func resolveCommit() string {
	if isConcreteValue(Commit) {
		return Commit
	}
	if rev := vcsSetting("vcs.revision"); rev != "" {
		return rev
	}
	return unknownValue
}

// resolveSourceDate answers the Source Date row, on the same precedence.
func resolveSourceDate() string {
	if isConcreteValue(SourceDate) {
		return SourceDate
	}
	if t := vcsSetting("vcs.time"); t != "" {
		return t
	}
	return unknownValue
}

// isConcreteValue rejects BOTH placeholders as answers.
//
// Two spellings rather than one because the stamp points do not agree on
// which they use - a version reads "dev" while a commit reads "unknown" -
// and a resolution that knew only one of them would accept the other as
// a real value and skip the derivation that had the answer.
func isConcreteValue(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && s != devVersion && s != unknownValue
}

// BinaryVersion returns the registered version string for the current binary.
func BinaryVersion() string {
	mu.RLock()
	defer mu.RUnlock()

	if active.Version != "" {
		return active.Version
	}
	return GAPIVersion // fallback from linker flags
}

// SetBinaryNameAndVersion lets a binary like gapictl override the top-level label
func SetBinaryNameAndVersion(name, version string) {
	mu.Lock()
	defer mu.Unlock()
	active.Name = name
	active.Version = version
}

// SetShipsKernel records whether the invoking binary is the kernel
// itself. gapi's own binaries declare true; a process that embeds the
// kernel declares nothing and gets the Runtime Core row.
//
// Separate from SetBinaryNameAndVersion on purpose, so that the two can
// be called in either order and neither can change what the other
// means.
func SetShipsKernel(v bool) {
	mu.Lock()
	defer mu.Unlock()
	shipsKernel = v
}

// SetBuildMetadata lets downstream components override specific build details
func SetBuildMetadata(overrides Info) {
	mu.Lock()
	defer mu.Unlock()
	if overrides.Commit != "" {
		active.Commit = overrides.Commit
	}
	if overrides.BuildDate != "" {
		active.BuildDate = overrides.BuildDate
	}
	if overrides.BuiltBy != "" {
		active.BuiltBy = overrides.BuiltBy
	}
	if overrides.GoVersion != "" {
		active.GoVersion = overrides.GoVersion
	}
	if overrides.Platform != "" {
		active.Platform = overrides.Platform
	}
}

// Summary prints a single version block, merging binary and GAPI info
func Summary() string {
	mu.RLock()
	defer mu.RUnlock()

	name := active.Name
	version := active.Version
	identified := name != ""
	if !identified {
		name = unidentifiedBinaryLabel
	}
	if version == "" {
		// The kernel's own row takes the resolved version too. Reading
		// GAPIVersion directly here would leave gapid reporting "dev" in
		// any build that did not stamp it, which is the same defect this
		// resolution exists to remove, one row further up.
		version = KernelVersion()
	}

	schemaHash := truncate16(SchemaHash)
	commit := truncate16(active.Commit)

	// GROUPED BY RELATIONSHIP, SEPARATED BY BLANK LINES (cli-contract.md):
	// the binary's own identity, the components it embeds, the provenance
	// of the build, and the platform it was built on. No headings -
	// grouping is whitespace, so the block stays greppable and
	// line-oriented.
	//
	// One column width across EVERY group, taken from the longest label
	// anywhere in the block. The width is computed over all groups rather
	// than per group, or the columns would step as the block goes down.
	// This block used to carry four different hardcoded paddings - %-11s
	// for the name, separate widths for the ADK rows and Platform, and a
	// 21-character "Protobuf Schema Hash:" that aligned with nothing - so
	// adding a field meant re-guessing the alignment.
	embedded := [][2]string{}

	// The embedded-kernel row is emitted only by a binary that both
	// identified itself and did not declare that it ships the kernel.
	//
	// Both halves are required. Without the declaration gapid prints its
	// own version twice, which is the defect this replaced. Without
	// `identified`, a binary that registered nothing would print a
	// Runtime Core row underneath a block already headed Runtime Core -
	// the fallback name and the row label are the same string, and that
	// coincidence is precisely what must not drive a branch.
	if identified && !shipsKernel {
		embedded = append(embedded, [2]string{embeddedKernelRowLabel, KernelVersion()})
	}
	embedded = append(embedded,
		[2]string{"Go ADK", adkVersion(GoADKVersion)},
		[2]string{"Python ADK", adkVersion(PythonADKVersion)},
		[2]string{"Protobuf Schema Hash", schemaHash},
	)

	groups := [][][2]string{
		{{name, version}},
		embedded,
		{
			{"Commit", commit},
			{"Build Tag", BuildTag},
			{"Source Date", active.BuildDate},
			{"Built By", active.BuiltBy},
		},
		{
			{"Built With Go", active.GoVersion},
			{"Platform", active.Platform},
		},
	}

	width := 0
	for _, g := range groups {
		for _, r := range g {
			if n := len(r[0]) + 1; n > width { // +1 for the colon
				width = n
			}
		}
	}

	var out strings.Builder
	for i, g := range groups {
		if i > 0 {
			out.WriteString("\n")
		}
		for _, r := range g {
			fmt.Fprintf(&out, "%-*s %s\n", width, r[0]+":", r[1])
		}
	}
	return out.String()
}

// truncate16 bounds the hash-shaped fields, which are long enough to
// dominate the block and are identifying at 16 characters.
func truncate16(s string) string {
	if len(s) > 16 {
		return s[:16]
	}
	return s
}
