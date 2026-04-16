//go:build !windows

package process

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestTerminateWorktreeProcesses_NoProcesses(t *testing.T) {
	dir := t.TempDir()
	procs, err := TerminateWorktreeProcesses(dir, 1*time.Second)
	if err != nil {
		t.Fatalf("TerminateWorktreeProcesses: %v", err)
	}
	if len(procs) != 0 {
		t.Errorf("expected 0 processes, got %d", len(procs))
	}
}

func TestTerminateWorktreeProcesses_KillsProcessInWorktree(t *testing.T) {
	dir := t.TempDir()

	cmd := exec.Command("sleep", "60")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Give the OS a moment to record the process cwd.
	time.Sleep(200 * time.Millisecond)

	procs, err := TerminateWorktreeProcesses(dir, 2*time.Second)
	if err != nil {
		t.Fatalf("TerminateWorktreeProcesses: %v", err)
	}
	if len(procs) == 0 {
		t.Fatal("expected at least one process, got none")
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process was not terminated within 5s")
	}
}

func TestTerminateWorktreeProcesses_EscalatesToKill(t *testing.T) {
	dir := t.TempDir()

	// Ignore SIGTERM; only SIGKILL should end this process.
	cmd := exec.Command("sh", "-c", "trap '' TERM; sleep 60")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sh: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	procs, err := TerminateWorktreeProcesses(dir, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("TerminateWorktreeProcesses: %v", err)
	}
	if len(procs) == 0 {
		t.Fatal("expected processes to target, got none")
	}
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Errorf("expected grace period to elapse before SIGKILL, only took %s", elapsed)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("process was not killed after escalation")
	}
}

func TestTerminateWorktreeProcesses_AlreadyDeadPIDIsNoop(t *testing.T) {
	// Kill our own fake PID that won't exist; Terminate should not error.
	// We simulate by starting a process, killing it, then calling Terminate
	// on the same cwd - FindProcessesInWorktree will return nothing, but we
	// still cover the "dead pids" case by exercising the scan-empty path.
	dir := t.TempDir()
	cmd := exec.Command("sleep", "60")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	_ = cmd.Process.Signal(syscall.SIGKILL)
	_ = cmd.Wait()

	procs, err := TerminateWorktreeProcesses(dir, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("TerminateWorktreeProcesses: %v", err)
	}
	if len(procs) != 0 {
		t.Errorf("expected 0 processes after kill, got %d", len(procs))
	}
}
