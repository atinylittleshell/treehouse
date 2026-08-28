package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addE2EBranch creates branch at HEAD plus one commit carrying marker, pushes
// it, and returns to main.
func addE2EBranch(t *testing.T, repoDir, branch, marker string) {
	t.Helper()
	gitCmd(t, repoDir, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(repoDir, marker), []byte(marker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repoDir, "add", marker)
	gitCmd(t, repoDir, "commit", "-m", "on "+branch)
	gitCmd(t, repoDir, "push", "origin", branch)
	gitCmd(t, repoDir, "checkout", "main")
}

func writeRepoConfig(t *testing.T, repoDir, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoDir, "treehouse.toml"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGetUsesConfiguredBaseBranch(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	addE2EBranch(t, repoDir, "develop", "develop-only.txt")
	writeRepoConfig(t, repoDir, "base_branch = \"develop\"\n")

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease")
	if code != 0 {
		t.Fatalf("get --lease failed (code %d): %s", code, stderr)
	}
	wtPath := strings.TrimSpace(stdout)
	if _, err := os.Stat(filepath.Join(wtPath, "develop-only.txt")); err != nil {
		t.Errorf("expected the worktree to be cut from develop: %v", err)
	}
}

func TestGetBaseFlagOverridesConfiguredBaseBranch(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	addE2EBranch(t, repoDir, "develop", "develop-only.txt")
	addE2EBranch(t, repoDir, "release", "release-only.txt")
	writeRepoConfig(t, repoDir, "base_branch = \"develop\"\n")

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease", "--base", "release")
	if code != 0 {
		t.Fatalf("get --lease --base release failed (code %d): %s", code, stderr)
	}
	wtPath := strings.TrimSpace(stdout)
	if _, err := os.Stat(filepath.Join(wtPath, "release-only.txt")); err != nil {
		t.Errorf("expected --base to win over base_branch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "develop-only.txt")); !os.IsNotExist(err) {
		t.Error("expected the worktree to be cut from release, not develop")
	}
}

func TestGetUnknownBaseFailsClosed(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease", "--base", "no-such-branch")
	if code == 0 {
		t.Fatalf("get with an unknown base succeeded: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stderr, "no-such-branch") {
		t.Errorf("expected the error to name the branch, got %q", stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("expected no path on stdout for a failed acquire, got %q", stdout)
	}

	statusOut, _, _ := runTreehouse(t, repoDir, homeDir, nil, "status")
	if strings.Contains(statusOut, "available") || strings.Contains(statusOut, "leased") {
		t.Errorf("expected no worktree to be created, got status:\n%s", statusOut)
	}
}

func TestGetUnknownConfiguredBaseBranchFailsClosed(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	writeRepoConfig(t, repoDir, "base_branch = \"no-such-branch\"\n")

	_, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease")
	if code == 0 {
		t.Fatal("get with an unresolvable base_branch succeeded")
	}
	if !strings.Contains(stderr, "no-such-branch") {
		t.Errorf("expected the error to name the branch, got %q", stderr)
	}
}

func TestGetLeaseJSONReportsBaseBranch(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	addE2EBranch(t, repoDir, "develop", "develop-only.txt")

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease", "--json", "--base", "develop")
	if code != 0 {
		t.Fatalf("get --lease --json --base develop failed (code %d): %s", code, stderr)
	}
	var lease leaseJSONResult
	if err := json.Unmarshal([]byte(stdout), &lease); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if lease.BaseBranch != "develop" {
		t.Errorf("base_branch = %q, want develop", lease.BaseBranch)
	}
}

func TestGetLeaseJSONReportsInferredBaseBranch(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease", "--json")
	if code != 0 {
		t.Fatalf("get --lease --json failed (code %d): %s", code, stderr)
	}
	var lease leaseJSONResult
	if err := json.Unmarshal([]byte(stdout), &lease); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout, err)
	}
	if lease.BaseBranch != "main" {
		t.Errorf("base_branch = %q, want main", lease.BaseBranch)
	}
}

func TestGetBaseKeepsPathOnlyStdout(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	addE2EBranch(t, repoDir, "develop", "develop-only.txt")

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease", "--base", "develop")
	if code != 0 {
		t.Fatalf("get --lease --base develop failed (code %d): %s", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 || !filepath.IsAbs(lines[0]) {
		t.Fatalf("expected exactly one absolute path on stdout, got %q", stdout)
	}
}

func TestGetInteractiveUsesBaseBranchAndParksSlotThere(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	addE2EBranch(t, repoDir, "develop", "develop-only.txt")
	writeRepoConfig(t, repoDir, "base_branch = \"develop\"\n")

	if err := os.WriteFile(filepath.Join(repoDir, "hotfix.txt"), []byte("hotfix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repoDir, "add", "hotfix.txt")
	gitCmd(t, repoDir, "commit", "-m", "hotfix on main")
	gitCmd(t, repoDir, "push", "origin", "main")
	developTip := gitCmd(t, repoDir, "rev-parse", "develop")

	env := []string{"SHELL=" + exitShellBin}
	_, stderr, code := runTreehouse(t, repoDir, homeDir, env, "get")
	if code != 0 {
		t.Fatalf("get failed (code %d): %s", code, stderr)
	}
	wtPath := extractWorktreePath(stderr, homeDir)
	if wtPath == "" {
		t.Fatal("could not extract worktree path from stderr")
	}
	if got := gitCmd(t, wtPath, "rev-parse", "HEAD"); got != developTip {
		t.Errorf("returned slot HEAD = %s, want develop tip %s", got, developTip)
	}
}

func TestStatusShowsConfiguredBaseBranch(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	addE2EBranch(t, repoDir, "develop", "develop-only.txt")
	writeRepoConfig(t, repoDir, "base_branch = \"develop\"\n")

	if _, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease"); code != 0 {
		t.Fatalf("get --lease failed: %s", stderr)
	}

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "status")
	if code != 0 {
		t.Fatalf("status failed (code %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "base") || !strings.Contains(stdout, "develop") {
		t.Errorf("expected status to report the configured base branch, got:\n%s", stdout)
	}
}

func TestStatusShowsInferredBaseBranch(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "status")
	if code != 0 {
		t.Fatalf("status failed (code %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "main") {
		t.Errorf("expected status to report the inferred base branch, got:\n%s", stdout)
	}
}

func TestStatusFlagsUnresolvableConfiguredBaseBranch(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	writeRepoConfig(t, repoDir, "base_branch = \"no-such-branch\"\n")

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "status")
	if code != 0 {
		t.Fatalf("status must stay a read-only report and exit 0, got %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "no-such-branch") {
		t.Errorf("expected status to name the unresolvable base branch, got:\n%s", stdout)
	}
}

// status --json is a top-level array machine callers already parse.
func TestStatusJSONRemainsATopLevelArray(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	addE2EBranch(t, repoDir, "develop", "develop-only.txt")
	writeRepoConfig(t, repoDir, "base_branch = \"develop\"\n")

	if _, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease"); code != 0 {
		t.Fatalf("get --lease failed: %s", stderr)
	}

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "status", "--json")
	if code != 0 {
		t.Fatalf("status --json failed (code %d): %s", code, stderr)
	}
	var statuses []statusJSONResult
	if err := json.Unmarshal([]byte(stdout), &statuses); err != nil {
		t.Fatalf("status --json is no longer a top-level array: %v\n%s", err, stdout)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 worktree, got %d: %s", len(statuses), stdout)
	}
	if strings.Contains(stdout, "base") && !strings.Contains(stdout, "\"base") {
		t.Errorf("unexpected human base line in JSON output:\n%s", stdout)
	}
}

// Exercises returnBaseBranch: `treehouse return <path>` must park the slot on
// the configured base, the same place `get` leaves one.
func TestReturnParksWorktreeOnConfiguredBaseBranch(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	addE2EBranch(t, repoDir, "develop", "develop-only.txt")
	writeRepoConfig(t, repoDir, "base_branch = \"develop\"\n")
	gitCmd(t, repoDir, "commit", "--allow-empty", "-m", "hotfix on main")
	gitCmd(t, repoDir, "push", "origin", "main")
	developTip := gitCmd(t, repoDir, "rev-parse", "develop")

	stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease")
	if code != 0 {
		t.Fatalf("get --lease failed: %s", stderr)
	}
	wtPath := strings.TrimSpace(stdout)

	if _, stderr, code = runTreehouse(t, repoDir, homeDir, nil, "return", wtPath); code != 0 {
		t.Fatalf("return failed (code %d): %s", code, stderr)
	}
	if got := gitCmd(t, wtPath, "rev-parse", "HEAD"); got != developTip {
		t.Errorf("returned slot HEAD = %s, want develop tip %s", got, developTip)
	}
}

// A pool driven only by --base, with no base_branch in config, must recycle its
// slot instead of building a new one per acquisition.
func TestGetBaseFlagRecyclesWithoutConfiguredBaseBranch(t *testing.T) {
	repoDir, homeDir := setupTestRepo(t)
	addE2EBranch(t, repoDir, "develop", "develop-only.txt")
	writeRepoConfig(t, repoDir, "max_trees = 2\n")
	gitCmd(t, repoDir, "commit", "--allow-empty", "-m", "hotfix on main")
	gitCmd(t, repoDir, "push", "origin", "main")

	var first string
	for i := 1; i <= 3; i++ {
		stdout, stderr, code := runTreehouse(t, repoDir, homeDir, nil, "get", "--lease", "--base", "develop")
		if code != 0 {
			t.Fatalf("acquire %d failed (code %d): %s", i, code, stderr)
		}
		wtPath := strings.TrimSpace(stdout)
		if i == 1 {
			first = wtPath
		} else if wtPath != first {
			t.Fatalf("acquire %d built a new slot %s instead of recycling %s", i, wtPath, first)
		}
		if _, stderr, code = runTreehouse(t, repoDir, homeDir, nil, "return", wtPath); code != 0 {
			t.Fatalf("return %d failed: %s", i, stderr)
		}
	}
}
