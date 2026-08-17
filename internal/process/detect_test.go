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
func (p fakeProcessProbe) Cwd() (string, error)  { return p.cwd, p.cwdErr }
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

func TestValidateStrictProcessPlatformRejectsWindows(t *testing.T) {
	if err := validateStrictProcessPlatform("windows"); err == nil {
		t.Fatal("Windows safe process inspection should be unsupported")
	}
	if err := validateStrictProcessPlatform("darwin"); err != nil {
		t.Fatalf("Darwin safe process inspection rejected: %v", err)
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
