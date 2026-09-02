package shell

import (
	"slices"
	"testing"
)

func TestShellArgs(t *testing.T) {
	tests := []struct {
		goos         string
		hasUserShell bool
		want         []string
	}{
		{"darwin", true, []string{"-i", "-l"}},
		{"linux", false, nil},
		{"windows", true, nil},
	}

	for _, test := range tests {
		if got := shellArgs(test.goos, test.hasUserShell); !slices.Equal(got, test.want) {
			t.Errorf("shellArgs(%q, %t) = %q, want %q", test.goos, test.hasUserShell, got, test.want)
		}
	}
}
