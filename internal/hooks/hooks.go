// Package hooks runs user-configured shell commands at worktree lifecycle
// points. Commands run sequentially in a given working directory. A failing
// command is logged but does not stop later commands or fail the caller.
package hooks

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/kunchenguid/treehouse/internal/deadline"
)

// Run executes each command in commands sequentially in workDir. Each command
// is passed to the OS shell (/bin/sh -c on Unix, %COMSPEC% /d /s /c on Windows).
// Stdout and stderr from the commands are streamed to the given writers.
// Failures are logged to stderr and do not stop subsequent commands.
//
// Hook commands are user code with no bound on how long they may take, so time
// spent here is not charged to the caller's deadline: Run grants a fresh budget
// on the way out. Without that, a pre_destroy hook slower than the timeout
// would leave prune and destroy to start their removal phase already past the
// deadline, failing a cleanup that was healthy and stranding the reservations
// the hook was run to protect.
func Run(commands []string, workDir string, stdout, stderr io.Writer) {
	defer deadline.Restart()

	for _, command := range commands {
		runOne(command, workDir, stdout, stderr)
	}
}

func runOne(command, workDir string, stdout, stderr io.Writer) {
	cmd := newHookCommand(command)
	cmd.Dir = workDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "🌳 hook command failed: %q (exit %d): %v\n", command, exitCode, err)
	}
}

func windowsShellCommandLine(shell, command string) string {
	return `"` + shell + `" /d /s /c "` + command + `"`
}
