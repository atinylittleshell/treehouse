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
	username    string
	usernameErr error
	cwd         string
	cwdErr      error
	name        string
	exists      bool
	existsErr   error
}

func (p fakeProcessProbe) PID() int32                { return p.pid }
func (p fakeProcessProbe) Status() ([]string, error) { return p.status, p.statusErr }
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
			name: "status error",
			probe: fakeProcessProbe{
				pid: 0, statusErr: cwdErr, exists: true,
			},
			want: "access denied",
		},
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

func TestShouldSkipWindowsIdleProcess(t *testing.T) {
	if !shouldSkipSystemProcess("windows", 0) {
		t.Fatal("Windows idle process should be excluded from user-process inspection")
	}
	for _, tc := range []struct {
		goos string
		pid  int32
	}{
		{goos: "windows", pid: 1},
		{goos: "linux", pid: 0},
		{goos: "darwin", pid: 0},
	} {
		if shouldSkipSystemProcess(tc.goos, tc.pid) {
			t.Fatalf("unexpected system-process exclusion for %s pid %d", tc.goos, tc.pid)
		}
	}
}

func TestFindProcessesInWorktreeStrictSkipsVanishedAndOtherUserProcesses(t *testing.T) {
	worktree := t.TempDir()
	probes := []processProbe{
		fakeProcessProbe{pid: 0, status: []string{"zombie"}, exists: true},
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
	probe := fakeProcessProbe{
		pid:      42,
		username: "ravi",
		cwd:      child,
		name:     "agent",
		exists:   true,
	}

	got, err := findProcessesInWorktreeStrict(worktree, "ravi", []processProbe{probe})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PID != 42 {
		t.Fatalf("strict scan = %v, want pid 42", got)
	}
}
