package shell

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSpawnTreatsSymlinkedShellWrapperAsFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Unix shell-wrapper fixture is not executable on Windows")
	}

	dir := t.TempDir()
	wrapper := filepath.Join(dir, "wrapper")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\n[ \"$#\" -eq 0 ]\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	shellPath := filepath.Join(dir, "bash")
	if err := os.Symlink(wrapper, shellPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shellPath)

	exitCode, err := Spawn(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatalf("Spawn() exit code = %d, want 0; symlinked wrappers must receive no shell arguments", exitCode)
	}
}

func TestSpawnDocumentsRegularSameNamedWrapperAsAcceptedLimitation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Unix shell-wrapper fixture is not executable on Windows")
	}

	// Pins the documented contract: identification is by resolved basename
	// only, so a regular wrapper executable named bash still receives -i -l.
	// Probing PATH identity instead would strip the flags from genuine second
	// installs of the same shell, which is the worse failure.
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "bash")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nexit 42\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", wrapper)

	exitCode, err := Spawn(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 42 {
		t.Fatalf("Spawn() exit code = %d, want 42; a same-named regular wrapper is the documented accepted limitation and must still receive -i -l", exitCode)
	}
}
