package process

import (
	"errors"
	"testing"

	gopsutilprocess "github.com/shirou/gopsutil/v4/process"
)

func TestFilterProtectedProcesses_SkipsCurrentProcessAndAncestors(t *testing.T) {
	procs := []ProcessInfo{
		{PID: 100, Name: "shell"},
		{PID: 200, Name: "treehouse"},
		{PID: 300, Name: "server"},
	}

	filtered, err := filterProtectedProcesses(procs, 200, func(pid int32) (int32, error) {
		switch pid {
		case 200:
			return 100, nil
		case 100:
			return 1, nil
		case 1:
			return 0, nil
		default:
			return 0, errors.New("unknown pid")
		}
	})
	if err != nil {
		t.Fatalf("filterProtectedProcesses: %v", err)
	}

	if len(filtered) != 1 {
		t.Fatalf("expected 1 process after filtering, got %d", len(filtered))
	}
	if filtered[0].PID != 300 {
		t.Fatalf("expected pid 300 to remain, got %d", filtered[0].PID)
	}
	if filtered[0].Name != "server" {
		t.Fatalf("expected server to remain, got %q", filtered[0].Name)
	}
}

func TestFilterProtectedProcesses_ReturnsErrorWhenParentLookupFails(t *testing.T) {
	procs := []ProcessInfo{
		{PID: 100, Name: "shell"},
		{PID: 200, Name: "treehouse"},
		{PID: 300, Name: "server"},
	}

	// A parent-lookup failure must surface as an error, not silently protect
	// every process and report "nothing to kill".
	filtered, err := filterProtectedProcesses(procs, 200, func(pid int32) (int32, error) {
		if pid == 200 {
			return 0, errors.New("cannot inspect parent")
		}
		return 0, nil
	})
	if err == nil {
		t.Fatalf("expected an error when parent lookup fails, got filtered=%+v", filtered)
	}
	if filtered != nil {
		t.Fatalf("expected no filtered processes on error, got %+v", filtered)
	}
}

// An ancestor that has already exited (gopsutil's ErrorProcessNotRunning) ends
// the walk instead of failing termination. This is the normal Windows case
// where a parent exits and leaves a dangling parent PID; termination must still
// target the remaining foreign processes rather than error out.
func TestFilterProtectedProcesses_ExitedAncestorEndsWalk(t *testing.T) {
	procs := []ProcessInfo{
		{PID: 100, Name: "shell"},
		{PID: 200, Name: "treehouse"},
		{PID: 300, Name: "server"},
	}

	filtered, err := filterProtectedProcesses(procs, 200, func(pid int32) (int32, error) {
		if pid == 200 {
			return 100, nil
		}
		// The parent (100) has exited before we could read its own parent.
		return 0, gopsutilprocess.ErrorProcessNotRunning
	})
	if err != nil {
		t.Fatalf("expected an exited ancestor to end the walk cleanly, got: %v", err)
	}
	if len(filtered) != 1 || filtered[0].PID != 300 {
		t.Fatalf("expected only the foreign process 300 to remain, got %+v", filtered)
	}
}
