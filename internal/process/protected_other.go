//go:build !windows

package process

func isProtectedSystemProcess(_ int32) (bool, error) {
	return false, nil
}
