//go:build !windows

package gitvcs

import "os"

func replaceFile(source, dest string) error {
	return os.Rename(source, dest)
}
