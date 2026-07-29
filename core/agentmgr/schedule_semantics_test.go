package agentmgr

import (
	"testing"
	"time"
)

// The three systemd prefixes were aliases: whichever one matched was
// stripped and the duration became a repeating interval, so OnBootSec=1m
// ran every minute instead of once, a minute after boot (GAPI-DIV-036).
// These tests assert the distinction the names promise, by fire COUNT and
// fire INSTANT rather than by "it parsed".

var (
	testBoot = time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	testNow  = testBoot.Add(30 * time.Minute)
)

// fires drains a schedule, returning every instant it yields. It stops at
// a cap so a repeating schedule cannot hang the test.
func fires(s Schedule, from time.Time, cap int) []time.Time {
	var out []time.Time
	t := from
	for range cap {
		next := s.Next(t)
		if next.IsZero() {
			break
		}
		out = append(out, next)
		// Advance past the fire, as the run loop does.
		if !next.After(t) {
			t = t.Add(time.Nanosecond)
		} else {
			t = next
		}
	}
	return out
}

func TestOnUnitActiveSecRepeats(t *testing.T) {
	s, err := ParseScheduleAt("OnUnitActiveSec=5s", testNow, testBoot)
	if err != nil {
		t.Fatal(err)
	}
	got := fires(s, testNow, 4)
	if len(got) != 4 {
		t.Fatalf("expected a repeating schedule to keep firing, got %d fires", len(got))
	}
	for i, want := range []time.Time{
		testNow.Add(5 * time.Second),
		testNow.Add(10 * time.Second),
		testNow.Add(15 * time.Second),
		testNow.Add(20 * time.Second),
	} {
		if !got[i].Equal(want) {
			t.Errorf("fire %d = %v, want %v", i, got[i], want)
		}
	}
}

func TestOnStartupSecFiresOnceRelativeToNow(t *testing.T) {
	s, err := ParseScheduleAt("OnStartupSec=10s", testNow, testBoot)
	if err != nil {
		t.Fatal(err)
	}
	got := fires(s, testNow, 5)
	if len(got) != 1 {
		t.Fatalf("OnStartupSec must fire exactly once, got %d fires: %v", len(got), got)
	}
	if want := testNow.Add(10 * time.Second); !got[0].Equal(want) {
		t.Errorf("fire = %v, want %v (now + 10s)", got[0], want)
	}
}

func TestOnBootSecFiresOnceRelativeToBoot(t *testing.T) {
	s, err := ParseScheduleAt("OnBootSec=45m", testNow, testBoot)
	if err != nil {
		t.Fatal(err)
	}
	got := fires(s, testNow, 5)
	if len(got) != 1 {
		t.Fatalf("OnBootSec must fire exactly once, got %d fires: %v", len(got), got)
	}
	// 45m after boot, which is 15m from now - NOT 45m from now.
	if want := testBoot.Add(45 * time.Minute); !got[0].Equal(want) {
		t.Errorf("fire = %v, want %v (boot + 45m)", got[0], want)
	}
	if got[0].Sub(testNow) != 15*time.Minute {
		t.Errorf("fire is %v from now, want 15m; the anchor is boot, not now", got[0].Sub(testNow))
	}
}

// A host already up longer than the declared duration has missed the
// elapse point. systemd triggers such a timer immediately, once, rather
// than cancelling it; the run loop clamps the negative delay to zero.
func TestOnBootSecAlreadyElapsedStillFiresExactlyOnce(t *testing.T) {
	s, err := ParseScheduleAt("OnBootSec=5s", testNow, testBoot)
	if err != nil {
		t.Fatal(err)
	}
	got := fires(s, testNow, 5)
	if len(got) != 1 {
		t.Fatalf("a missed one-shot must still fire once, got %d fires", len(got))
	}
	if !got[0].Before(testNow) {
		t.Errorf("fire = %v, expected an instant in the past (boot + 5s)", got[0])
	}
}

// The three prefixes must not be interchangeable. This is the assertion
// whose absence let them alias.
func TestSystemdPrefixesAreNotAliases(t *testing.T) {
	unit, err := ParseScheduleAt("OnUnitActiveSec=1m", testNow, testBoot)
	if err != nil {
		t.Fatal(err)
	}
	startup, err := ParseScheduleAt("OnStartupSec=1m", testNow, testBoot)
	if err != nil {
		t.Fatal(err)
	}
	boot, err := ParseScheduleAt("OnBootSec=1m", testNow, testBoot)
	if err != nil {
		t.Fatal(err)
	}

	if n := len(fires(unit, testNow, 3)); n != 3 {
		t.Errorf("OnUnitActiveSec fired %d times, want 3 (repeating)", n)
	}
	if n := len(fires(startup, testNow, 3)); n != 1 {
		t.Errorf("OnStartupSec fired %d times, want 1", n)
	}
	if n := len(fires(boot, testNow, 3)); n != 1 {
		t.Errorf("OnBootSec fired %d times, want 1", n)
	}

	// Fresh instances: a one-shot is stateful, so the schedules drained
	// above are already exhausted.
	startup2, _ := ParseScheduleAt("OnStartupSec=1m", testNow, testBoot)
	boot2, _ := ParseScheduleAt("OnBootSec=1m", testNow, testBoot)
	sFire := fires(startup2, testNow, 1)[0]
	bFire := fires(boot2, testNow, 1)[0]
	if sFire.Equal(bFire) {
		t.Errorf("OnStartupSec and OnBootSec resolved to the same instant %v; they anchor differently", sFire)
	}
}

// A one-shot is exhausted after its single fire, and stays exhausted.
// The run loop depends on this: a zero Next is what stops it.
func TestOnceScheduleStaysExhausted(t *testing.T) {
	s, err := ParseScheduleAt("OnStartupSec=1s", testNow, testBoot)
	if err != nil {
		t.Fatal(err)
	}
	if s.Next(testNow).IsZero() {
		t.Fatal("first Next returned zero; the schedule never fired")
	}
	for i := range 3 {
		if got := s.Next(testNow); !got.IsZero() {
			t.Fatalf("Next call %d after the fire returned %v, want the zero time", i+2, got)
		}
	}
}

func TestRawDurationAndCronStillRepeat(t *testing.T) {
	for _, spec := range []string{"5s", "*/5 * * * *", "@hourly"} {
		s, err := ParseScheduleAt(spec, testNow, testBoot)
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		if n := len(fires(s, testNow, 3)); n != 3 {
			t.Errorf("%s fired %d times, want 3 (repeating)", spec, n)
		}
	}
}

func TestInvalidDurationInPrefixIsRejected(t *testing.T) {
	for _, spec := range []string{"OnBootSec=nope", "OnStartupSec=", "OnUnitActiveSec=1d"} {
		if _, err := ParseScheduleAt(spec, testNow, testBoot); err == nil {
			t.Errorf("%s parsed, want an error", spec)
		}
	}
}

func TestBootTimeIsInThePast(t *testing.T) {
	b := BootTime()
	if b.After(time.Now()) {
		t.Fatalf("BootTime() = %v, which is in the future", b)
	}
}
