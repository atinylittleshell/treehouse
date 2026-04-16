//go:build !windows

package process

import (
	"syscall"
	"time"
)

func terminate(pids []int32, gracePeriod time.Duration) {
	for _, pid := range pids {
		_ = syscall.Kill(int(pid), syscall.SIGTERM)
	}

	deadline := time.Now().Add(gracePeriod)
	for time.Now().Before(deadline) {
		if !anyAlive(pids) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	for _, pid := range pids {
		if isAlive(pid) {
			_ = syscall.Kill(int(pid), syscall.SIGKILL)
		}
	}
}

// isAlive uses signal 0 which validates process existence without signaling it.
func isAlive(pid int32) bool {
	return syscall.Kill(int(pid), 0) == nil
}

func anyAlive(pids []int32) bool {
	for _, pid := range pids {
		if isAlive(pid) {
			return true
		}
	}
	return false
}
