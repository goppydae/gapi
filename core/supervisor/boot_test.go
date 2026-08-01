package supervisor_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/goppydae/gapi/core/mounts"
	"github.com/goppydae/gapi/core/procsig"
	"github.com/goppydae/gapi/core/supervisor"
)

type bootRecorder struct {
	pidfd     atomic.Int32
	subreaper atomic.Int32
	signals   atomic.Int32
	mounted   atomic.Int32
	reapLoop  atomic.Int32
}

func (b *bootRecorder) RequirePidfd() error    { b.pidfd.Store(1); return nil }
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
		RequirePidfd:  b.RequirePidfd,
		Subreaper:     b.BecomeSubreaper,
		InstallSignal: b.InstallSignals,
		Mount:         b.MountEarly,
		StartReap:     b.StartReapLoop,
		SkipMounts:    false,
	})
	if err != nil {
		t.Fatalf("RunPhase0: %v", err)
	}
	if b.pidfd.Load() == 0 || b.subreaper.Load() == 0 || b.signals.Load() == 0 || b.mounted.Load() == 0 || b.reapLoop.Load() == 0 {
		t.Fatalf("phase 0 skipped an obligation: %+v", map[string]int32{
			"pidfd":     b.pidfd.Load(),
			"subreaper": b.subreaper.Load(), "signals": b.signals.Load(),
			"mounted": b.mounted.Load(), "reap": b.reapLoop.Load(),
		})
	}
}

// GAPI-DIV-016: a kernel without pidfd makes signal delivery unsound
// for the whole process lifetime, because os/exec quietly falls back to
// kill-by-PID. Boot must refuse rather than run degraded.
//
// Asserting the error alone would not be enough: a check that reports
// the fault and then lets boot continue leaves exactly the hazard this
// entry names. So the test also asserts that NOTHING after the
// precondition ran.
func TestPhase0_RefusesWithoutPidfd(t *testing.T) {
	b := &bootRecorder{}
	err := supervisor.RunPhase0(context.Background(), supervisor.Phase0Deps{
		RequirePidfd:  func() error { return procsig.ErrPidfdUnsupported },
		Subreaper:     b.BecomeSubreaper,
		InstallSignal: b.InstallSignals,
		Mount:         b.MountEarly,
		StartReap:     b.StartReapLoop,
		SkipMounts:    false,
	})
	if err == nil {
		t.Fatal("phase 0 booted on a kernel without pidfd; signal delivery would be by bare PID")
	}
	if !errors.Is(err, procsig.ErrPidfdUnsupported) {
		t.Fatalf("phase 0 error = %v, want it to wrap ErrPidfdUnsupported", err)
	}
	if b.subreaper.Load() != 0 || b.signals.Load() != 0 || b.mounted.Load() != 0 || b.reapLoop.Load() != 0 {
		t.Fatalf("phase 0 continued past a failed precondition: %+v", map[string]int32{
			"subreaper": b.subreaper.Load(), "signals": b.signals.Load(),
			"mounted": b.mounted.Load(), "reap": b.reapLoop.Load(),
		})
	}
}

// The real probe must succeed on any machine that can run this suite -
// otherwise the gate above would be unfalsifiable in practice, passing
// only because nothing ever exercises the success path.
func TestRequirePidfd_RealKernel(t *testing.T) {
	if err := procsig.RequirePidfd(); err != nil {
		t.Fatalf("procsig.RequirePidfd() on this kernel = %v, want nil", err)
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
