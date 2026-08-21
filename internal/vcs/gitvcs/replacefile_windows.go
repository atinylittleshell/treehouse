//go:build windows

package gitvcs

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const replaceFileWriteThrough = 0x1

var procReplaceFileW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

func replaceFile(source, dest string) error {
	replaced, err := windows.UTF16PtrFromString(dest)
	if err != nil {
		return err
	}
	replacement, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	r1, _, callErr := procReplaceFileW.Call(
		uintptr(unsafe.Pointer(replaced)),
		uintptr(unsafe.Pointer(replacement)),
		0,
		replaceFileWriteThrough,
		0,
		0,
	)
	if r1 == 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return syscall.EINVAL
	}
	return nil
}
