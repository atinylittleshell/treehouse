//go:build windows

package process

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

func isProtectedSystemProcess(pid int32) (bool, error) {
	if pid == 0 || pid == 4 {
		return true, nil
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return false, err
	}
	for {
		if entry.ProcessID == uint32(pid) {
			return isProtectedSystemExecutable(windows.UTF16ToString(entry.ExeFile[:])), nil
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return false, nil
			}
			return false, err
		}
	}
}
