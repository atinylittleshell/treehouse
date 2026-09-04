package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// leaseLeaf acquires a worktree and returns the last segment of its path, the
// only part of the layout tooling can read from the working directory alone.
func leaseLeaf(t *testing.T, repoDir, homeDir string, extraEnv []string, args ...string) string {
	t.Helper()
	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, extraEnv, append([]string{"get", "--lease"}, args...)...)
	if code != 0 {
		t.Fatalf("get --lease %v failed (code %d): %s", args, code, stderr)
	}
	return filepath.Base(strings.TrimSpace(stdout))
}

func TestGetWithoutUniqueLeafKeepsRepositoryName(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)

	if got, want := leaseLeaf(t, repoDir, homeDir, nil), filepath.Base(repoDir); got != want {
		t.Errorf("worktree leaf = %q, want the unchanged default %q", got, want)
	}
}

func TestGetUniqueLeafFlagNamesWorktreeAfterItsSlot(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)

	if got, want := leaseLeaf(t, repoDir, homeDir, nil, "--unique-leaf"), filepath.Base(repoDir)+"-1"; got != want {
		t.Errorf("worktree leaf = %q, want %q", got, want)
	}
}

func TestGetUniqueLeafFlagGivesEachSlotADistinctLeaf(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)

	first := leaseLeaf(t, repoDir, homeDir, nil, "--unique-leaf")
	second := leaseLeaf(t, repoDir, homeDir, nil, "--unique-leaf")

	if first == second {
		t.Errorf("expected distinct leaf names, got %q for both slots", first)
	}
}

func TestGetUsesConfiguredUniqueLeaf(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	writeRepoConfig(t, repoDir, "unique_leaf = true\n")

	if got, want := leaseLeaf(t, repoDir, homeDir, nil), filepath.Base(repoDir)+"-1"; got != want {
		t.Errorf("worktree leaf = %q, want %q", got, want)
	}
}

func TestGetUniqueLeafEnvVarEnablesWithoutConfig(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)

	got := leaseLeaf(t, repoDir, homeDir, []string{"TREEHOUSE_UNIQUE_LEAF=1"})
	if want := filepath.Base(repoDir) + "-1"; got != want {
		t.Errorf("worktree leaf = %q, want %q", got, want)
	}
}

func TestGetUniqueLeafPrecedenceFlagOverEnvOverConfig(t *testing.T) {
	t.Run("env overrides config", func(t *testing.T) {
		repoDir, homeDir := setupTestRepo(t)
		writeRepoConfig(t, repoDir, "unique_leaf = true\n")

		got := leaseLeaf(t, repoDir, homeDir, []string{"TREEHOUSE_UNIQUE_LEAF=0"})
		if want := filepath.Base(repoDir); got != want {
			t.Errorf("worktree leaf = %q, want the env var to win with %q", got, want)
		}
	})

	t.Run("flag overrides env and config", func(t *testing.T) {
		repoDir, homeDir := setupTestRepo(t)
		writeRepoConfig(t, repoDir, "unique_leaf = false\n")

		got := leaseLeaf(t, repoDir, homeDir, []string{"TREEHOUSE_UNIQUE_LEAF=0"}, "--unique-leaf")
		if want := filepath.Base(repoDir) + "-1"; got != want {
			t.Errorf("worktree leaf = %q, want the flag to win with %q", got, want)
		}
	})

	t.Run("flag turns the option off for one acquisition", func(t *testing.T) {
		repoDir, homeDir := setupTestRepo(t)
		writeRepoConfig(t, repoDir, "unique_leaf = true\n")

		got := leaseLeaf(t, repoDir, homeDir, []string{"TREEHOUSE_UNIQUE_LEAF=1"}, "--unique-leaf=false")
		if want := filepath.Base(repoDir); got != want {
			t.Errorf("worktree leaf = %q, want %q", got, want)
		}
	})
}

func TestGetUniqueLeafReusesExistingWorktreeWithoutMovingIt(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease")
	if code != 0 {
		t.Fatalf("get --lease failed (code %d): %s", code, stderr)
	}
	shared := strings.TrimSpace(stdout)

	if _, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "return", shared); code != 0 {
		t.Fatalf("return failed (code %d): %s", code, stderr)
	}

	stdout, stderr, code = runTreehouse(t, repoDir, homeDir, nil, "get", "--lease", "--unique-leaf")
	if code != 0 {
		t.Fatalf("get --lease --unique-leaf failed (code %d): %s", code, stderr)
	}
	if recycled := strings.TrimSpace(stdout); recycled != shared {
		t.Errorf("recycled worktree = %q, want the existing %q left in place", recycled, shared)
	}
}

func TestGetUniqueLeafKeepsPathOnlyStdout(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease", "--unique-leaf")
	if code != 0 {
		t.Fatalf("get --lease --unique-leaf failed (code %d): %s", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 1 || !filepath.IsAbs(lines[0]) {
		t.Errorf("expected a single absolute path on stdout, got %q", stdout)
	}
}
