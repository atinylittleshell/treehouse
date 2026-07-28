//go:build !windows

package config

import "path/filepath"

func samePath(a string, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
