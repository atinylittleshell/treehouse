//go:build !windows

package pool

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile attempts to take the exclusive lock without blocking. It reports
// whether the lock was taken; a false return with a nil error means another
// process holds it.
func tryLockFile(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EWOULDBLOCK):
		return false, nil
	default:
		return false, err
	}
}

func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
