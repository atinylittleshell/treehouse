package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestShellArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Unix shell fixtures are not executable on Windows")
	}

	// Copy a real shell binary under supported and unsupported names so the
	// decision boundary — the basename after symlink resolution — is exercised
	// with paths that exist on every Unix runner.
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no sh on PATH: %v", err)
	}
	dir := t.TempDir()
	named := func(name string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(sh)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}

	tests := []struct {
		shellPath    string
		hasUserShell bool
		want         []string
	}{
		{named("bash"), true, []string{"-i", "-l"}},
		{named("zsh"), true, []string{"-i", "-l"}},
		{named("fish"), true, []string{"-i", "-l"}},
		{named("sh"), true, nil},
		{named("shell-wrapper"), true, nil},
		{"", false, nil},
	}

	for _, test := range tests {
		if got := shellArgs(runtime.GOOS, test.shellPath, test.hasUserShell); !slices.Equal(got, test.want) {
			t.Errorf("shellArgs(%q, %q, %t) = %q, want %q", runtime.GOOS, test.shellPath, test.hasUserShell, got, test.want)
		}
	}
}
