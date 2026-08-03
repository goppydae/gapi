package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

// Injected at build time via -ldflags
var (
	GAPIVersion      = "dev"
	GoDDKVersion     = "dev"
	PythonDDKVersion = "dev"
	BuildTag         = "dev"
	SchemaHash       = "unknown"
	Commit           = "unknown"
	Date             = "unknown"
	BuiltBy          = "unknown"
)

// runtimeCoreLabel is the kernel's own row, and the fallback name when
// no binary has registered an identity.
const runtimeCoreLabel = "Runtime Core"

// gapiModulePath is the kernel's module path, looked up in the build
// info of whatever binary embeds it.
const gapiModulePath = "github.com/goppydae/gapi"

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

// gapiVersionFrom finds the kernel's module in build info. A replace
// directive is honoured when it names a version, because the replacement
// is what was actually linked.
func gapiVersionFrom(bi *debug.BuildInfo) string {
	if bi.Main.Path == gapiModulePath && isConcreteVersion(bi.Main.Version) {
		return bi.Main.Version
	}
	for _, dep := range bi.Deps {
		if dep == nil || dep.Path != gapiModulePath {
			continue
		}
		if dep.Replace != nil && isConcreteVersion(dep.Replace.Version) {
			return dep.Replace.Version
		}
		if isConcreteVersion(dep.Version) {
			return dep.Version
		}
	}
	return ""
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
)

func init() {
	active = Info{
		Commit:    Commit,
		BuildDate: Date,
		BuiltBy:   BuiltBy,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
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
	if name == "" {
		name = runtimeCoreLabel
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

	// Rows first, then one column width taken from the longest label
	// (cli-contract.md). The block used to carry four different hardcoded
	// paddings - %-11s for the name, and separate widths for Go DDK,
	// Platform and a 21-character "Protobuf Schema Hash:" that aligned
	// with nothing - so adding a field meant re-guessing the alignment.
	rows := [][2]string{{name, version}}
	// Runtime Core is emitted only when the invoking binary is not the
	// kernel itself, so gapid does not print its own version twice.
	if name != runtimeCoreLabel {
		rows = append(rows, [2]string{runtimeCoreLabel, KernelVersion()})
	}
	rows = append(rows,
		[2]string{"Go DDK", adkVersion(GoDDKVersion)},
		[2]string{"Python DDK", adkVersion(PythonDDKVersion)},
		[2]string{"Protobuf Schema Hash", schemaHash},
		[2]string{"Go Version", active.GoVersion},
		[2]string{"Platform", active.Platform},
		[2]string{"Commit", commit},
		[2]string{"Build Tag", BuildTag},
		[2]string{"Built Date", active.BuildDate},
		[2]string{"Built By", active.BuiltBy},
	)

	width := 0
	for _, r := range rows {
		if n := len(r[0]) + 1; n > width { // +1 for the colon
			width = n
		}
	}

	var out strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&out, "%-*s %s\n", width, r[0]+":", r[1])
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
