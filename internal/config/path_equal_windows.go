//go:build windows

package config

import (
	"path/filepath"
	"strings"
)

func samePath(a string, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
