//go:build linux

package procsig_test

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/procsig"
)

// binPath resolves a binary from PATH: hardcoded FHS paths differ
// between sandboxes and NixOS hosts.
func binPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not in PATH: %v", name, err)
	}
	return p
}

func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(binPath(t, "sleep"), "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd
}

func TestStartEpoch_SelfIsNonZero(t *testing.T) {
	epoch, err := procsig.StartEpoch(os.Getpid())
	if err != nil {
		t.Fatalf("StartEpoch(self): %v", err)
	}
	if epoch == 0 {
		t.Fatal("StartEpoch(self) = 0, want > 0")
	}
}

func TestStartEpoch_GonePid(t *testing.T) {
	// PID 0 is never a signalable process from userspace.
	if _, err := procsig.StartEpoch(0); err == nil {
		t.Fatal("StartEpoch(0) succeeded, want error")
	}
}

func TestSignal_DeliversWithMatchingEpoch(t *testing.T) {
	cmd := startSleeper(t)
	pid := cmd.Process.Pid

	epoch, err := procsig.StartEpoch(pid)
	if err != nil {
		t.Fatalf("StartEpoch: %v", err)
	}
	if err := procsig.Signal(pid, epoch, syscall.SIGTERM); err != nil {
		t.Fatalf("Signal: %v", err)
	}

	state, err := cmd.Process.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	ws, ok := state.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
		t.Fatalf("process exit = %v, want killed by SIGTERM", state)
	}
}

func TestSignal_StaleEpochRejectedNoDelivery(t *testing.T) {
	cmd := startSleeper(t)
	pid := cmd.Process.Pid

	epoch, err := procsig.StartEpoch(pid)
	if err != nil {
		t.Fatalf("StartEpoch: %v", err)
	}

	err = procsig.Signal(pid, epoch+12345, syscall.SIGKILL)
	if !errors.Is(err, procsig.ErrStaleEpoch) {
		t.Fatalf("stale-epoch Signal error = %v, want ErrStaleEpoch", err)
	}

	// The process must NOT have been signaled: still alive shortly after.
	time.Sleep(50 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("process is gone after a rejected signal: %v", err)
	}
}

func TestSignal_GoneProcess(t *testing.T) {
	cmd := exec.Command(binPath(t, "true"))
	if err := cmd.Start(); err != nil {
		t.Fatalf("start true: %v", err)
	}
	pid := cmd.Process.Pid
	epoch, err := procsig.StartEpoch(pid)
	if err != nil {
		t.Skipf("true exited before epoch read: %v", err)
	}
	if _, err := cmd.Process.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	err = procsig.Signal(pid, epoch, syscall.SIGTERM)
	// The pid is dead (usually unreused: ErrProcessGone); if the OS
	// recycled it already, the epoch guard must still refuse.
	if !errors.Is(err, procsig.ErrProcessGone) && !errors.Is(err, procsig.ErrStaleEpoch) {
		t.Fatalf("dead-pid Signal error = %v, want ErrProcessGone or ErrStaleEpoch", err)
	}
}

func TestIdentify_SelfMatchesStartEpoch(t *testing.T) {
	id, err := procsig.Identify(os.Getpid())
	if err != nil {
		t.Fatalf("Identify(self): %v", err)
	}
	if id.Pid != os.Getpid() {
		t.Fatalf("Identify pid = %d, want %d", id.Pid, os.Getpid())
	}
	epoch, err := procsig.StartEpoch(os.Getpid())
	if err != nil {
		t.Fatalf("StartEpoch: %v", err)
	}
	if id.StartEpoch != epoch {
		t.Fatalf("Identify epoch = %d, StartEpoch = %d", id.StartEpoch, epoch)
	}
	if id.PidNsInode == 0 {
		t.Fatal("Identify pid namespace inode = 0, want nonzero")
	}
}

func TestIdentify_ChildSharesNamespace(t *testing.T) {
	cmd := startSleeper(t)
	self, err := procsig.Identify(os.Getpid())
	if err != nil {
		t.Fatalf("Identify(self): %v", err)
	}
	child, err := procsig.Identify(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("Identify(child): %v", err)
	}
	if child.PidNsInode != self.PidNsInode {
		t.Fatalf("child pidns inode %d != self %d (same namespace expected)", child.PidNsInode, self.PidNsInode)
	}
}
