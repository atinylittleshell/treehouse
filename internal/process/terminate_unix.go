//go:build !windows

package process

import (
	"syscall"
	"time"
)

// Package-level seams so unix termination can be driven deterministically in
// tests instead of racing real signal delivery and reaping.
var (
	killProcess = func(pid int32, sig syscall.Signal) { _ = syscall.Kill(int(pid), sig) }
	// isAlive uses signal 0 which validates process existence without signaling it.
	isAlive = func(pid int32) bool { return syscall.Kill(int(pid), 0) == nil }
	// reapPollInterval bounds how often the post-signal waits re-check liveness.
	reapPollInterval = 50 * time.Millisecond
)

func terminate(pids []int32, gracePeriod time.Duration) {
	for _, pid := range pids {
		killProcess(pid, syscall.SIGTERM)
	}

	if waitForExit(pids, gracePeriod) {
		return
	}

	for _, pid := range pids {
		if isAlive(pid) {
			killProcess(pid, syscall.SIGKILL)
		}
	}

	// A SIGKILLed process is not reaped instantly: it can briefly linger holding
	// index.lock, open file descriptors, and a cwd inside the worktree. Mirror
	// the SIGTERM wait so a caller that runs git next (e.g. return's reset) does
	// not race a still-dying writer. The wait is bounded, so an unkillable
	// process degrades to the prior behaviour instead of hanging.
	waitForExit(pids, gracePeriod)
}

// waitForExit polls until no pid is alive or timeout elapses, reporting whether
// every pid has exited.
func waitForExit(pids []int32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !anyAlive(pids) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(reapPollInterval)
	}
}

func anyAlive(pids []int32) bool {
	for _, pid := range pids {
		if isAlive(pid) {
			return true
		}
	}
	return false
}
