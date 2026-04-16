//go:build windows

package process

import (
	"time"

	"golang.org/x/sys/windows"
)

func terminate(pids []int32, _ time.Duration) {
	// Windows has no graceful SIGTERM equivalent for arbitrary processes;
	// TerminateProcess is the standard way to end a process from outside it.
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
		if err != nil {
			continue
		}
		_ = windows.TerminateProcess(h, 1)
		_ = windows.CloseHandle(h)
	}
}
