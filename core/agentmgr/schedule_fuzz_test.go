package agentmgr

import (
	"strings"
	"testing"
	"time"
)

// Fixed clocks. ParseSchedule reads the wall clock and the host boot
// time; a fuzz target that did the same could produce a crasher its own
// seed could not reproduce. ParseScheduleAt exists exactly so the clock
// can be pinned, so the target uses it.
var (
	fuzzNow  = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	fuzzBoot = fuzzNow.Add(-72 * time.Hour)
)

// FuzzParseScheduleAt fuzzes the systemd-style timer expression parser,
// which eats operator strings straight out of an agent manifest.
//
// Invariants:
//
//   - totality: it returns a Schedule or an error, never both and never
//     neither, and never panics;
//   - the systemd branch is exact: a string carrying one of the three
//     recognized prefixes is accepted if and only if the remainder is a
//     parseable Go duration;
//   - the result is usable: Next on the returned Schedule does not panic
//     for any parseable input;
//   - determinism: two parses of the same string under the same pinned
//     clocks yield the same first fire time;
//   - the one-shot contract holds: a OnceSchedule yields exactly one
//     non-zero fire time and the zero time forever after. This is the
//     only way a one-shot terminates (GAPI-DIV-036).
//
// NOTE: this target does NOT assert that Next(t) is after t. It is not,
// for negative durations ("-5s", "@every -1s"), and what those inputs
// should mean is an open question - see the GAPI-DIV-042 report.
func FuzzParseScheduleAt(f *testing.F) {
	for _, s := range []string{
		"OnUnitActiveSec=5s", // repeating systemd form
		"OnStartupSec=1m",    // one-shot, anchored at now
		"OnBootSec=30s",      // one-shot, anchored at boot
		"OnBootSec=-1h",      // boundary: elapse point already in the past
		"OnBootSec=",         // prefix with no duration
		"OnBootSec=x",        // prefix with junk duration
		"*/5 * * * *",        // cron
		"@hourly",            // cron descriptor
		"@every 1h",
		"@every -1s", // known-nasty: a delay that walks backwards
		"5s",         // raw duration
		"-5s",        // known-nasty: negative raw interval
		"1m30s",
		"",                         // empty
		"   ",                      // whitespace only
		"0 0 31 2 *",               // valid cron that can never fire
		"* * * * * *",              // six fields: not this parser's grammar
		"2562047h47m16.854775807s", // boundary: max Go duration
		"OnUnitActiveSec=9223372036854775807ns",
	} {
		f.Add(s)
	}

	prefixes := []string{"OnUnitActiveSec=", "OnStartupSec=", "OnBootSec="}

	f.Fuzz(func(t *testing.T, s string) {
		sched, err := ParseScheduleAt(s, fuzzNow, fuzzBoot)

		if (sched == nil) == (err == nil) {
			t.Fatalf("ParseScheduleAt(%q) returned schedule=%v err=%v; exactly one must be non-nil", s, sched, err)
		}

		trimmed := strings.TrimSpace(s)
		for _, p := range prefixes {
			if !strings.HasPrefix(trimmed, p) {
				continue
			}
			_, derr := time.ParseDuration(strings.TrimPrefix(trimmed, p))
			if derr == nil && err != nil {
				t.Fatalf("rejected %q whose duration part parses: %v", s, err)
			}
			if derr != nil && err == nil {
				t.Fatalf("accepted %q whose duration part does not parse", s)
			}
			break
		}

		if err != nil {
			return
		}

		first := sched.Next(fuzzNow)

		// Determinism: a second parse under the same clocks must agree.
		again, err2 := ParseScheduleAt(s, fuzzNow, fuzzBoot)
		if err2 != nil {
			t.Fatalf("ParseScheduleAt(%q) succeeded then failed: %v", s, err2)
		}
		if got := again.Next(fuzzNow); !got.Equal(first) {
			t.Fatalf("non-deterministic schedule %q: first fire %v then %v", s, first, got)
		}

		// One-shot contract: exactly one fire, then exhausted forever.
		if once, ok := sched.(*OnceSchedule); ok {
			if first.IsZero() {
				t.Fatalf("one-shot %q was exhausted before it ever fired", s)
			}
			if next := once.Next(fuzzNow); !next.IsZero() {
				t.Fatalf("one-shot %q fired twice: %v then %v", s, first, next)
			}
			if next := once.Next(fuzzNow.Add(time.Hour)); !next.IsZero() {
				t.Fatalf("one-shot %q rearmed after exhaustion: %v", s, next)
			}
			return
		}

		// Repeating schedules must stay callable: Next is a total
		// function of the time handed to it.
		for _, at := range []time.Time{fuzzNow, fuzzBoot, first, fuzzNow.Add(365 * 24 * time.Hour)} {
			_ = sched.Next(at)
		}
	})
}
