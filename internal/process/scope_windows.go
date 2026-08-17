//go:build windows

package process

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

func processIDsInScope(pids []int32, currentPID int32) (map[int32]bool, error) {
	var currentSession uint32
	if err := windows.ProcessIdToSessionId(uint32(currentPID), &currentSession); err != nil {
		return nil, err
	}
	if currentSession == 0 {
		return nil, fmt.Errorf("safe process inspection is unavailable from Windows Session 0")
	}

	sessions := make(map[int32]uint32, len(pids))
	for _, pid := range pids {
		var session uint32
		if err := windows.ProcessIdToSessionId(uint32(pid), &session); err != nil {
			if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
				continue
			}
			return nil, err
		}
		sessions[pid] = session
	}
	return processIDsForSession(currentSession, sessions), nil
}
