package process

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeProcessProbe struct {
	pid         int32
	status      []string
	statusErr   error
	statusCalls *int
	username    string
	usernameErr error
	cwd         string
	cwdErr      error
	name        string
	exists      bool
	existsErr   error
}

func (p fakeProcessProbe) PID() int32 { return p.pid }
func (p fakeProcessProbe) Status() ([]string, error) {
	if p.statusCalls != nil {
		(*p.statusCalls)++
	}
	return p.status, p.statusErr
}
func (p fakeProcessProbe) Username() (string, error) { return p.username, p.usernameErr }
func (p fakeProcessProbe) Cwd() (string, error)      { return p.cwd, p.cwdErr }
func (p fakeProcessProbe) Name() (string, error)     { return p.name, nil }
func (p fakeProcessProbe) Exists() (bool, error)     { return p.exists, p.existsErr }

func TestFindProcessesInWorktreeStrictFailsClosed(t *testing.T) {
	worktree := t.TempDir()
	cwdErr := errors.New("access denied")

	for _, tc := range []struct {
		name  string
		probe fakeProcessProbe
		want  string
	}{
		{
			name: "empty cwd",
			probe: fakeProcessProbe{
				pid: 1, username: "ravi", cwd: "", exists: true,
			},
			want: "empty path",
		},
		{
			name: "cwd error",
			probe: fakeProcessProbe{
				pid: 2, username: "ravi", cwdErr: cwdErr, exists: true,
			},
			want: "access denied",
		},
		{
			name: "owner error",
			probe: fakeProcessProbe{
				pid: 3, usernameErr: cwdErr, exists: true,
			},
			want: "access denied",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := findProcessesInWorktreeStrict(worktree, "ravi", []processProbe{tc.probe})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("strict scan error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestShouldSkipCurrentAndWindowsIdleProcesses(t *testing.T) {
	if !shouldSkipProcess("windows", 0, 42) {
		t.Fatal("Windows idle process should be excluded from user-process inspection")
	}
	if !shouldSkipProcess("darwin", 42, 42) {
		t.Fatal("current process should be excluded from user-process inspection")
	}
	for _, tc := range []struct {
		goos       string
		pid        int32
		currentPID int32
	}{
		{goos: "windows", pid: 1, currentPID: 42},
		{goos: "linux", pid: 0, currentPID: 42},
		{goos: "darwin", pid: 0, currentPID: 42},
	} {
		if shouldSkipProcess(tc.goos, tc.pid, tc.currentPID) {
			t.Fatalf("unexpected system-process exclusion for %s pid %d", tc.goos, tc.pid)
		}
	}
}

func TestFindProcessesInWorktreeStrictSkipsVanishedAndOtherUserProcesses(t *testing.T) {
	worktree := t.TempDir()
	probes := []processProbe{
		fakeProcessProbe{
			pid: 0, status: []string{"zombie"}, username: "ravi",
			cwdErr: errors.New("defunct"), exists: true,
		},
		fakeProcessProbe{pid: 1, usernameErr: errors.New("gone"), exists: false},
		fakeProcessProbe{pid: 2, username: "other", cwd: worktree, exists: true},
	}

	got, err := findProcessesInWorktreeStrict(worktree, "ravi", probes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("strict scan found excluded processes: %v", got)
	}
}

func TestFindProcessesInWorktreeStrictFindsChildDirectory(t *testing.T) {
	worktree := t.TempDir()
	child := filepath.Join(worktree, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	statusCalls := 0
	probe := fakeProcessProbe{
		pid:         42,
		username:    "ravi",
		cwd:         child,
		name:        "agent",
		exists:      true,
		statusCalls: &statusCalls,
	}

	got, err := findProcessesInWorktreeStrict(worktree, "ravi", []processProbe{probe})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PID != 42 {
		t.Fatalf("strict scan = %v, want pid 42", got)
	}
	if statusCalls != 0 {
		t.Fatalf("normal process scan called Status %d times", statusCalls)
	}
}

func TestFindProcessesInWorktreeStrictDoesNotCallStatusOnWindows(t *testing.T) {
	worktree := t.TempDir()
	statusCalls := 0
	probe := fakeProcessProbe{
		pid:         42,
		statusErr:   errors.New("not implemented"),
		statusCalls: &statusCalls,
		username:    "ravi",
		cwd:         t.TempDir(),
		exists:      true,
	}

	got, err := findProcessesInWorktreeStrictForOS(worktree, "ravi", "windows", []processProbe{probe})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("strict Windows scan found outside process: %v", got)
	}
	if statusCalls != 0 {
		t.Fatalf("Windows process scan called Status %d times", statusCalls)
	}
}
