package shell

import (
	"slices"
	"testing"
)

func TestShellArgs(t *testing.T) {
	tests := []struct {
		goos         string
		shellPath    string
		hasUserShell bool
		want         []string
	}{
		{"darwin", "/bin/zsh", true, []string{"-i", "-l"}},
		{"linux", "/usr/bin/bash", true, []string{"-i", "-l"}},
		{"linux", "/usr/local/bin/fish", true, []string{"-i", "-l"}},
		{"linux", "/bin/sh", true, nil},
		{"linux", "/tmp/shell-wrapper", true, nil},
		{"linux", "", false, nil},
		{"windows", "C:\\Windows\\System32\\cmd.exe", true, nil},
	}

	for _, test := range tests {
		if got := shellArgs(test.goos, test.shellPath, test.hasUserShell); !slices.Equal(got, test.want) {
			t.Errorf("shellArgs(%q, %q, %t) = %q, want %q", test.goos, test.shellPath, test.hasUserShell, got, test.want)
		}
	}
}
