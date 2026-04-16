package process

import (
	"errors"
	"testing"
)

func TestFilterProtectedProcesses_SkipsCurrentProcessAndAncestors(t *testing.T) {
	procs := []ProcessInfo{
		{PID: 100, Name: "shell"},
		{PID: 200, Name: "treehouse"},
		{PID: 300, Name: "server"},
	}

	filtered := filterProtectedProcesses(procs, 200, func(pid int32) (int32, error) {
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

func TestFilterProtectedProcesses_KeepsProcessesWhenParentLookupFails(t *testing.T) {
	procs := []ProcessInfo{
		{PID: 100, Name: "shell"},
		{PID: 200, Name: "treehouse"},
		{PID: 300, Name: "server"},
	}

	filtered := filterProtectedProcesses(procs, 200, func(pid int32) (int32, error) {
		if pid == 200 {
			return 0, errors.New("cannot inspect parent")
		}
		return 0, nil
	})

	if len(filtered) != 2 {
		t.Fatalf("expected 2 processes after filtering, got %d", len(filtered))
	}
	if filtered[0].PID != 100 || filtered[1].PID != 300 {
		t.Fatalf("expected non-self processes to remain, got %+v", filtered)
	}
}
