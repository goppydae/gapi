package supervisor_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/goppydae/gapi/core/mounts"
	"github.com/goppydae/gapi/core/supervisor"
)

type bootRecorder struct {
	subreaper atomic.Int32
	signals   atomic.Int32
	mounted   atomic.Int32
	reapLoop  atomic.Int32
}

func (b *bootRecorder) BecomeSubreaper() error { b.subreaper.Store(1); return nil }
func (b *bootRecorder) InstallSignals()        { b.signals.Store(1) }
func (b *bootRecorder) MountEarly(_ []mounts.MountSpec) error {
	b.mounted.Store(1)
	return nil
}
func (b *bootRecorder) StartReapLoop(context.Context) { b.reapLoop.Store(1) }

// Phase 0 runs the pre-userspace obligations in order: subreaper
// before signals (a SIGCHLD must have a reaper), mounts before the
// runtime that assumes them.
func TestPhase0_RunsObligations(t *testing.T) {
	b := &bootRecorder{}
	err := supervisor.RunPhase0(context.Background(), supervisor.Phase0Deps{
		Subreaper:     b.BecomeSubreaper,
		InstallSignal: b.InstallSignals,
		Mount:         b.MountEarly,
		StartReap:     b.StartReapLoop,
		SkipMounts:    false,
	})
	if err != nil {
		t.Fatalf("RunPhase0: %v", err)
	}
	if b.subreaper.Load() == 0 || b.signals.Load() == 0 || b.mounted.Load() == 0 || b.reapLoop.Load() == 0 {
		t.Fatalf("phase 0 skipped an obligation: %+v", map[string]int32{
			"subreaper": b.subreaper.Load(), "signals": b.signals.Load(),
			"mounted": b.mounted.Load(), "reap": b.reapLoop.Load(),
		})
	}
}

// --no-early-mounts skips the mount phase for container environments,
// leaving every other obligation intact.
func TestPhase0_SkipMounts(t *testing.T) {
	b := &bootRecorder{}
	err := supervisor.RunPhase0(context.Background(), supervisor.Phase0Deps{
		Subreaper:     b.BecomeSubreaper,
		InstallSignal: b.InstallSignals,
		Mount:         b.MountEarly,
		StartReap:     b.StartReapLoop,
		SkipMounts:    true,
	})
	if err != nil {
		t.Fatalf("RunPhase0: %v", err)
	}
	if b.mounted.Load() != 0 {
		t.Fatal("--no-early-mounts did not skip the mount phase")
	}
	if b.subreaper.Load() == 0 || b.signals.Load() == 0 {
		t.Fatal("skipping mounts also skipped other obligations")
	}
}
