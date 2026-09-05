//go:build windows

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestQuoteWindowsReturnPathCmdPastePreservesPercentEnv feeds the quoted form
// through cmd.exe's command-line parser (interactive paste, not a .bat file)
// and checks the child process argv is the literal %NAME% path. A naive
// "%NAME%" inside one pair of double quotes expands before the process starts.
func TestQuoteWindowsReturnPathCmdPastePreservesPercentEnv(t *testing.T) {
	path := `C:\Users\%USERNAME%\repo`
	quoted := quoteWindowsReturnPath(path)
	wantLiteral := path

	probe := buildCmdArgvProbe(t)
	got := runViaCmdArgv(t, probe, quoted)
	t.Logf("quoteWindowsReturnPath(%q) = %s", path, quoted)
	t.Logf("cmd.exe argv after paste-style parse: %q", got)
	if got != wantLiteral {
		t.Fatalf("cmd paste argv = %q, want literal path %q", got, wantLiteral)
	}

	naive := `"` + path + `"`
	expanded := runViaCmdArgv(t, probe, naive)
	t.Logf("cmd.exe argv for naive quoted form: %q", expanded)
	user := os.Getenv("USERNAME")
	if expanded == wantLiteral || user == "" || !strings.Contains(expanded, user) {
		t.Fatalf("control case should expand %%USERNAME%%, got %q", expanded)
	}
}

func buildCmdArgvProbe(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	exe := filepath.Join(dir, "printarg.exe")
	if err := os.WriteFile(src, []byte(`package main
import ("fmt"; "os")
func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	fmt.Print(os.Args[1])
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(mod, []byte("module cmdargvprobe\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", exe, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build argv probe: %v\n%s", err, out)
	}
	return exe
}

func runViaCmdArgv(t *testing.T, probeExe, quotedArg string) string {
	t.Helper()
	// Interactive paste is `probe <quoted-path>`. Spawning that through
	// `cmd /C` needs /S plus an outer quote pair so cmd's multi-quote
	// stripping leaves the probe invocation intact (otherwise paths with
	// %"NAME"% trip "filename syntax is incorrect"). Raw CmdLine avoids
	// Go's Windows argv escaping rewriting the pasted quotes.
	inner := `"` + probeExe + `" ` + quotedArg
	cmdline := `cmd.exe /S /C "` + inner + `"`
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: cmdline}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cmd argv probe: %v\ncmdline=%s\n%s", err, cmdline, out)
	}
	return string(out)
}
