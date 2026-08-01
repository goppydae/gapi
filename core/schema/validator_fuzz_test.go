package schema

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Fuzz targets for the manifest validators. These are a hostile
// boundary: every string here arrives from an operator-authored agent
// manifest, so the validators must be total (error or accept, never
// panic) and their acceptances must mean something downstream.
//
// Each target asserts a named invariant, not merely "did not panic".

// memUnits mirrors the unit table in ValidateMemoryLimit, longest match
// first. The test carries its own copy so the assertions below are a
// re-derivation of the spec rather than a call back into the code under
// test.
var memUnits = []struct {
	suffix string
	mult   int64
}{
	{"GB", 1 << 30},
	{"MB", 1 << 20},
	{"KB", 1 << 10},
	{"B", 1},
	{"G", 1 << 30},
	{"M", 1 << 20},
	{"K", 1 << 10},
}

// splitMemLimit extracts the numeric prefix and unit of an already
// normalized (trimmed, upper-cased) memory limit.
func splitMemLimit(limit string) (num, unit string, ok bool) {
	for _, u := range memUnits {
		if strings.HasSuffix(limit, u.suffix) {
			return strings.TrimSuffix(limit, u.suffix), u.suffix, true
		}
	}
	return "", "", false
}

func memMult(unit string) int64 {
	for _, u := range memUnits {
		if u.suffix == unit {
			return u.mult
		}
	}
	return 0
}

// FuzzValidateCPULimit checks that ValidateCPULimit is total and that an
// acceptance is meaningful:
//
//   - it never panics;
//   - the verdict does not depend on surrounding whitespace;
//   - err == nil implies the limit denotes a FINITE, POSITIVE number of
//     CPUs. "NaN cores" and "Inf cores" are not quantities, and a limit
//     that is not a quantity cannot be applied to a cgroup.
func FuzzValidateCPULimit(f *testing.F) {
	for _, s := range []string{
		"0.5",   // valid decimal
		"500m",  // valid millicpu
		"1",     // valid integer
		"0",     // boundary: zero is rejected
		"1e-9",  // boundary: smallest sane positive
		"-1",    // negative
		"",      // empty
		"m",     // unit with no number
		"  2  ", // whitespace tolerated
		"NaN",   // known-nasty: strconv.ParseFloat accepts these
		"Inf",   // and neither survives a "val <= 0" rejection test
		"+Inf",
		"nanm",
		"1e400", // out of range: ParseFloat reports ErrRange
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		err := ValidateCPULimit(s)

		if (ValidateCPULimit(strings.TrimSpace(s)) == nil) != (err == nil) {
			t.Fatalf("verdict changed under TrimSpace: %q", s)
		}
		if err != nil {
			return
		}

		trimmed := strings.TrimSpace(s)
		num := strings.TrimSuffix(trimmed, "m")
		v, perr := strconv.ParseFloat(num, 64)
		if perr != nil {
			t.Fatalf("accepted %q but numeric part %q does not parse: %v", s, num, perr)
		}
		if math.IsNaN(v) {
			t.Fatalf("accepted %q: NaN is not a cpu quantity", s)
		}
		if math.IsInf(v, 0) {
			t.Fatalf("accepted %q: infinity is not a cpu quantity", s)
		}
		if v <= 0 {
			t.Fatalf("accepted %q: cpu quantity %v is not positive", s, v)
		}
	})
}

// FuzzValidateMemoryLimit checks that ValidateMemoryLimit is total and
// that an acceptance survives conversion:
//
//   - it never panics;
//   - the verdict does not depend on surrounding whitespace;
//   - err == nil implies the limit splits into a positive integer and a
//     known unit, and that the resulting BYTE COUNT fits in int64. The
//     conversion downstream is "value * multiplier" in int64; a limit
//     the validator accepts but that wraps on conversion becomes a
//     negative byte count, which is worse than a rejected manifest.
//   - parse-format-parse is stable: the canonical rendering of an
//     accepted limit is accepted and splits to the same value and unit.
func FuzzValidateMemoryLimit(f *testing.F) {
	for _, s := range []string{
		"100MB", // valid
		"1GB",
		"512M",
		"1G",
		"1B",                    // boundary: smallest unit
		"0MB",                   // boundary: zero is rejected
		"-1MB",                  // negative
		"MB",                    // unit with no number
		"",                      // empty
		" 1kb ",                 // whitespace and lower case
		"9223372036854775808MB", // just past int64: ParseInt rejects
		"9223372036854775807GB", // known-nasty: fits int64, overflows bytes
		"9007199254740993KB",    // fits int64, overflows bytes
		"+5MB",                  // signed
		"1MBB",                  // trailing unit noise
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		err := ValidateMemoryLimit(s)

		if (ValidateMemoryLimit(strings.TrimSpace(s)) == nil) != (err == nil) {
			t.Fatalf("verdict changed under TrimSpace: %q", s)
		}
		if err != nil {
			return
		}

		norm := strings.TrimSpace(strings.ToUpper(s))
		num, unit, ok := splitMemLimit(norm)
		if !ok {
			t.Fatalf("accepted %q but it carries no known unit", s)
		}
		v, perr := strconv.ParseInt(num, 10, 64)
		if perr != nil {
			t.Fatalf("accepted %q but numeric part %q does not parse: %v", s, num, perr)
		}
		if v <= 0 {
			t.Fatalf("accepted %q: memory quantity %d is not positive", s, v)
		}
		mult := memMult(unit)
		if v > math.MaxInt64/mult {
			t.Fatalf("accepted %q: %d%s overflows int64 bytes", s, v, unit)
		}

		canonical := fmt.Sprintf("%d%s", v, unit)
		if cerr := ValidateMemoryLimit(canonical); cerr != nil {
			t.Fatalf("accepted %q but rejected its canonical form %q: %v", s, canonical, cerr)
		}
		cnum, cunit, cok := splitMemLimit(canonical)
		if !cok || cunit != unit || cnum != strconv.FormatInt(v, 10) {
			t.Fatalf("canonical form %q of %q does not re-split to (%d, %s)", canonical, s, v, unit)
		}
	})
}

// FuzzValidateSchedule checks that ValidateSchedule is total and that
// its systemd branch is exact:
//
//   - it never panics;
//   - the verdict does not depend on surrounding whitespace;
//   - err == nil implies the trimmed schedule is non-empty;
//   - a schedule carrying one of the three systemd prefixes is accepted
//     if and only if the remainder is a parseable Go duration;
//   - a schedule containing "=" that carries none of those prefixes is
//     rejected. Without this, a typo like "OnBootSecs=5s" would fall
//     through to the permissive cron branch and be silently accepted.
//
// NOTE: this target deliberately does NOT assert that an accepted
// schedule is one agentmgr.ParseScheduleAt can parse. It is not, and
// closing that gap is an open question - see the GAPI-DIV-042 report.
func FuzzValidateSchedule(f *testing.F) {
	for _, s := range []string{
		"OnUnitActiveSec=5s", // valid systemd
		"OnBootSec=30s",
		"OnStartupSec=1m",
		"*/5 * * * *", // valid cron
		"@hourly",
		"5s",             // raw duration
		"",               // empty
		"   ",            // whitespace only
		"OnBootSec=",     // prefix with no duration
		"OnBootSec=x",    // prefix with junk duration
		"OnBootSecs=5s",  // known-nasty: near-miss prefix
		"Foo=bar",        // unknown key
		"OnBootSec=1m=2", // duration with an embedded separator
		"@every -1s",
	} {
		f.Add(s)
	}

	systemdPrefixes := []string{"OnUnitActiveSec=", "OnBootSec=", "OnStartupSec="}

	f.Fuzz(func(t *testing.T, s string) {
		err := ValidateSchedule(s)

		if (ValidateSchedule(strings.TrimSpace(s)) == nil) != (err == nil) {
			t.Fatalf("verdict changed under TrimSpace: %q", s)
		}

		trimmed := strings.TrimSpace(s)
		if err == nil && trimmed == "" {
			t.Fatalf("accepted an empty schedule from %q", s)
		}

		var matched string
		for _, p := range systemdPrefixes {
			if strings.HasPrefix(trimmed, p) {
				matched = p
				break
			}
		}
		if matched != "" {
			if _, derr := time.ParseDuration(strings.TrimPrefix(trimmed, matched)); derr != nil {
				if err == nil {
					t.Fatalf("accepted %q whose duration part does not parse", s)
				}
			} else if err != nil {
				t.Fatalf("rejected %q whose duration part parses: %v", s, err)
			}
			return
		}

		if err == nil && strings.Contains(trimmed, "=") {
			t.Fatalf("accepted %q: a key=value schedule with no known prefix must be rejected", s)
		}
	})
}

// FuzzParseVersion checks that ParseVersion is total and round-trips:
//
//   - it never panics;
//   - a rejection returns the zero Version, never a partially filled one;
//   - an acceptance round-trips: formatting the parsed version and
//     re-parsing yields an identical Version.
func FuzzParseVersion(f *testing.F) {
	for _, s := range []string{
		"1.0.0",                            // valid
		"0.0.0",                            // boundary: all zero
		"4294967295.4294967295.4294967295", // boundary: max uint32
		"4294967296.0.0",                   // just past uint32
		"1.0",                              // too few parts
		"1.0.0.0",                          // too many parts
		"",                                 // empty
		"..",                               // empty parts
		"-1.0.0",                           // signed
		"+1.0.0",
		"01.02.03",                  // leading zeros
		" 1.0.0",                    // leading space
		"1.0.0-alpha",               // known-nasty: semver prerelease suffix
		"999999999999999999999.0.0", // overflow
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		v, err := ParseVersion(s)
		if err != nil {
			if v != (Version{}) {
				t.Fatalf("rejected %q but returned a non-zero version %+v", s, v)
			}
			return
		}

		formatted := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
		v2, err2 := ParseVersion(formatted)
		if err2 != nil {
			t.Fatalf("accepted %q -> %q but the formatted form does not re-parse: %v", s, formatted, err2)
		}
		if v2 != v {
			t.Fatalf("round trip changed the version: %q -> %+v -> %q -> %+v", s, v, formatted, v2)
		}
	})
}

// FuzzValidateAgentDescribe reaches the struct entry point.
//
// ValidateAgentDescribe takes a struct rather than a string, but every
// hostile field in it IS a string, so a multi-string fuzz signature
// reaches it with no extraction and no test-only shim. It is worth
// reaching: it is the function a discovered manifest actually goes
// through, and it composes three validators (validateID, validateType,
// validateListenStream) that no other target here touches.
//
// Invariant: the composite verdict agrees with its parts. An acceptance
// implies every non-empty field is individually valid, and a rejection
// implies at least one field is individually invalid - the composite
// never invents a failure and never launders one.
func FuzzValidateAgentDescribe(f *testing.F) {
	f.Add("web", "service", "0.5", "100MB", "", ":8080")
	f.Add("timer-1", "timer", "", "", "OnBootSec=30s", "")
	f.Add("", "", "", "", "", "")
	f.Add("bad id", "SERVICE", "1", "1G", "5s", "127.0.0.1:9000")
	f.Add("x", "daemon", "NaN", "0MB", "Foo=bar", "1:2:3")
	f.Add("a-b_C9", "oneshot", "500m", "1B", "@hourly", ":0")

	f.Fuzz(func(t *testing.T, id, typ, cpu, mem, sched, listen string) {
		desc := AgentDescribe{
			ID:           id,
			Type:         typ,
			CPULimit:     cpu,
			MemoryLimit:  mem,
			Schedule:     sched,
			ListenStream: listen,
		}
		err := ValidateAgentDescribe(desc)

		parts := []error{validateID(id), validateType(typ)}
		if cpu != "" {
			parts = append(parts, ValidateCPULimit(cpu))
		}
		if mem != "" {
			parts = append(parts, ValidateMemoryLimit(mem))
		}
		if sched != "" {
			parts = append(parts, ValidateSchedule(sched))
		}
		if listen != "" {
			parts = append(parts, validateListenStream(listen))
		}

		anyBad := false
		for _, p := range parts {
			if p != nil {
				anyBad = true
				break
			}
		}

		if err == nil && anyBad {
			t.Fatalf("accepted %+v although a field validator rejects it", desc)
		}
		if err != nil && !anyBad {
			t.Fatalf("rejected %+v although every field validator accepts it: %v", desc, err)
		}
	})
}
