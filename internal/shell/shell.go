package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func Spawn(dir string, env []string) (int, error) {
	shellPath := os.Getenv("SHELL")
	hasUserShell := shellPath != ""
	if shellPath == "" {
		if runtime.GOOS == "windows" {
			shellPath = os.Getenv("COMSPEC")
			if shellPath == "" {
				shellPath = "cmd.exe"
			}
		} else {
			shellPath = "/bin/sh"
		}
	}

	cmd := exec.Command(shellPath, shellArgs(runtime.GOOS, shellPath, hasUserShell)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

func shellArgs(goos, shellPath string, hasUserShell bool) []string {
	if goos != "windows" && hasUserShell && supportsLoginShellArgs(shellPath) {
		return []string{"-i", "-l"}
	}
	return nil
}

func supportsLoginShellArgs(shellPath string) bool {
	executable, err := exec.LookPath(shellPath)
	if err != nil {
		return false
	}
	resolvedPath, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return false
	}

	switch filepath.Base(resolvedPath) {
	case "bash", "fish", "zsh":
		// Contract: a supported resolved basename means the executable is that
		// shell and accepts -i -l. A same-named regular (non-symlink) wrapper
		// that cannot parse those flags stays an accepted limitation: probing
		// for PATH identity would reject genuine second installs.
		return true
	default:
		return false
	}
}
