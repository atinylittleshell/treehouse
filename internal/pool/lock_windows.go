//go:build windows

package pool

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLockFile attempts to take the exclusive lock without blocking. It reports
// whether the lock was taken; a false return with a nil error means another
// process holds it.
func tryLockFile(f *os.File) (bool, error) {
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, windows.ERROR_LOCK_VIOLATION), errors.Is(err, windows.ERROR_IO_PENDING):
		return false, nil
	default:
		return false, err
	}
}

func unlockFile(f *os.File) error {
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(h, 0, 1, 0, ol)
}
