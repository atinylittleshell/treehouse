package process

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeProcessProbe struct {
	pid           int32
	status        []string
	statusErr     error
	statusCalls   *int
	username      string
	usernameErr   error
	usernameCalls *int
	cwd           string
	cwdErr        error
	cwdCalls      *int
	name          string
	exists        bool
	existsErr     error
}

func (p fakeProcessProbe) PID() int32 { return p.pid }
func (p fakeProcessProbe) Status() ([]string, error) {
	if p.statusCalls != nil {
		(*p.statusCalls)++
	}
	return p.status, p.statusErr
}
func (p fakeProcessProbe) Username() (string, error) {
	if p.usernameCalls != nil {
		(*p.usernameCalls)++
	}
	return p.username, p.usernameErr
}
func (p fakeProcessProbe) Cwd() (string, error) {
	if p.cwdCalls != nil {
		(*p.cwdCalls)++
	}
	return p.cwd, p.cwdErr
}
func (p fakeProcessProbe) Name() (string, error) { return p.name, nil }
func (p fakeProcessProbe) Exists() (bool, error) { return p.exists, p.existsErr }

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

func TestProcessIDsForSessionExcludesSystemSession(t *testing.T) {
	got := processIDsForSession(2, map[int32]uint32{1: 0, 10: 1, 20: 2, 21: 2})
	if len(got) != 2 || !got[20] || !got[21] {
		t.Fatalf("session process scope = %v, want only session 2", got)
	}
}

func TestFindProcessesInWorktreeStrictSkipsVanishedZombieAndOutsideProcesses(t *testing.T) {
	worktree := t.TempDir()
	probes := []processProbe{
		fakeProcessProbe{
			pid: 0, status: []string{"zombie"},
			cwdErr: errors.New("defunct"), exists: true,
		},
		fakeProcessProbe{pid: 1, cwdErr: errors.New("gone"), exists: false},
		fakeProcessProbe{pid: 2, cwd: t.TempDir(), exists: true},
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
	usernameCalls := 0
	probe := fakeProcessProbe{
		pid:           42,
		cwd:           child,
		name:          "agent",
		exists:        true,
		statusCalls:   &statusCalls,
		usernameCalls: &usernameCalls,
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
	if usernameCalls != 0 {
		t.Fatalf("matching process scan called Username %d times", usernameCalls)
	}
}

func TestFindProcessesInWorktreeStrictChecksWindowsOwnerBeforeCwd(t *testing.T) {
	worktree := t.TempDir()
	statusCalls := 0
	usernameCalls := 0
	cwdCalls := 0
	probe := fakeProcessProbe{
		pid:           42,
		statusErr:     errors.New("not implemented"),
		statusCalls:   &statusCalls,
		username:      "SYSTEM",
		exists:        true,
		usernameCalls: &usernameCalls,
		cwdErr:        errors.New("access denied"),
		cwdCalls:      &cwdCalls,
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
	if usernameCalls != 1 {
		t.Fatalf("Windows process scan called Username %d times, want 1", usernameCalls)
	}
	if cwdCalls != 0 {
		t.Fatalf("other-user Windows process scan called Cwd %d times", cwdCalls)
	}
}
