//go:build windows

package process

import "golang.org/x/sys/windows"

func isProtectedSystemProcess(pid int32) (bool, error) {
	if pid == 0 || pid == 4 {
		return true, nil
	}
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(pid), &sessionID); err != nil {
		return false, err
	}
	return sessionID == 0, nil
}
