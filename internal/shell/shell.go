package shell

import (
	"os"
	"os/exec"
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

	cmd := exec.Command(shellPath, shellArgs(runtime.GOOS, hasUserShell)...)
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

func shellArgs(goos string, hasUserShell bool) []string {
	if goos != "windows" && hasUserShell {
		return []string{"-i", "-l"}
	}
	return nil
}
