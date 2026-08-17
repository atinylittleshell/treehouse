package process

import "strings"

func isProtectedSystemExecutable(name string) bool {
	switch strings.ToLower(name) {
	case "csrss.exe", "memory compression", "registry", "secure system", "smss.exe":
		return true
	default:
		return false
	}
}
