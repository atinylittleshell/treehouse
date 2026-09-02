package shell

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSpawnTreatsSameNamedShellWrappersAsFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Unix shell-wrapper fixture is not executable on Windows")
	}

	for _, test := range []struct {
		name    string
		symlink bool
	}{
		{name: "regular"},
		{name: "symlink", symlink: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			wrapper := filepath.Join(dir, "wrapper")
			if err := os.WriteFile(wrapper, []byte("#!/bin/sh\n[ \"$#\" -eq 0 ]\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			shellPath := filepath.Join(dir, "bash")
			if test.symlink {
				if err := os.Symlink(wrapper, shellPath); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Rename(wrapper, shellPath); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SHELL", shellPath)

			exitCode, err := Spawn(dir, nil)
			if err != nil {
				t.Fatal(err)
			}
			if exitCode != 0 {
				t.Fatalf("Spawn() exit code = %d, want 0; same-named wrappers must receive no shell arguments", exitCode)
			}
		})
	}
}
