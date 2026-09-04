package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The behavior this pins: killing a process does not kill what that process
// spawned. git runs its network transports as separate processes, and in the
// field those survived the deadline, kept the repository open, and on Windows
// even blocked the test harness from removing its own temp directory.
//
// Descendants must be collected while the parent is still alive - afterwards
// they are reparented to init and cannot be reached by walking down from it -
// which is the ordering KillDescendants exists to get right.
func TestKillDescendants_KillsAGrandchild(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "grandchild.pid")

	parent := exec.Command(os.Args[0], "-test.run=TestSpawnGrandchildProbe", "--", ready)
	parent.Env = append(os.Environ(), "TREEHOUSE_SPAWN_GRANDCHILD_PROBE=1")
	if err := parent.Start(); err != nil {
		t.Fatalf("starting parent: %v", err)
	}
	t.Cleanup(func() {
		_ = parent.Process.Kill()
		_ = parent.Wait()
	})

	grandchild := waitForRecordedPID(t, ready)
	if !Exists(grandchild) {
		t.Fatalf("grandchild %d should be running before the kill", grandchild)
	}

	// The real call order: descendants first, parent second.
	KillDescendants(int32(parent.Process.Pid))
	_ = parent.Process.Kill()
	_ = parent.Wait()

	stop := time.Now().Add(10 * time.Second)
	for time.Now().Before(stop) {
		if !Exists(grandchild) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("grandchild %d outlived the kill: a transport helper would still hold the repository", grandchild)
}

// A pid with no children, or one that no longer exists, is a no-op rather than
// an error: the caller is already failing on a deadline and nothing here
// changes that outcome.
func TestKillDescendants_ToleratesNoChildrenAndDeadProcesses(t *testing.T) {
	KillDescendants(int32(os.Getpid()) + 1<<20) // almost certainly not a live pid
	KillDescendants(-1)
}

func waitForRecordedPID(t *testing.T, path string) int32 {
	t.Helper()
	stop := time.Now().Add(20 * time.Second)
	for time.Now().Before(stop) {
		data, err := os.ReadFile(path)
		if err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
				return int32(pid)
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("probe never recorded its grandchild pid")
	return 0
}

// TestSpawnGrandchildProbe is the helper process: it spawns a child of its own,
// records that child's pid, and then blocks. It is inert unless re-executed
// with TREEHOUSE_SPAWN_GRANDCHILD_PROBE set.
func TestSpawnGrandchildProbe(t *testing.T) {
	if os.Getenv("TREEHOUSE_SPAWN_GRANDCHILD_PROBE") != "1" {
		return
	}
	argStart := -1
	for i := len(os.Args) - 1; i >= 0; i-- {
		if os.Args[i] == "--" {
			argStart = i
			break
		}
	}
	if argStart == -1 || len(os.Args)-argStart < 2 {
		t.Fatal("missing pid path")
	}

	child := exec.Command(os.Args[0], "-test.run=TestBlockForeverProbe")
	child.Env = append(os.Environ(), "TREEHOUSE_BLOCK_FOREVER_PROBE=1")
	if err := child.Start(); err != nil {
		t.Fatalf("starting grandchild: %v", err)
	}
	if err := os.WriteFile(os.Args[argStart+1], []byte(strconv.Itoa(child.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}
	select {}
}

// TestBlockForeverProbe is the grandchild: it does nothing until it is killed.
func TestBlockForeverProbe(t *testing.T) {
	if os.Getenv("TREEHOUSE_BLOCK_FOREVER_PROBE") != "1" {
		return
	}
	select {}
}
