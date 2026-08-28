package pool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addBranch creates branch at the current HEAD of repoDir plus one commit that
// carries marker, then returns to main. The marker file makes it verifiable
// which branch a worktree was actually cut from.
func addBranch(t *testing.T, repoDir, branch, marker string) string {
	t.Helper()
	runGit(t, repoDir, "checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(repoDir, marker), []byte(marker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoDir, "add", marker)
	runGit(t, repoDir, "commit", "-m", "on "+branch)
	tip := gitOut(t, repoDir, "rev-parse", "HEAD")
	runGit(t, repoDir, "checkout", "main")
	return tip
}

func TestAcquire_UsesConfiguredBaseBranch(t *testing.T) {
	repoDir, poolDir := setupRepo(t)
	developTip := addBranch(t, repoDir, "develop", "develop-only.txt")

	wtPath, err := AcquireWithOptions(repoDir, poolDir, 1, nil, AcquireOptions{BaseBranch: "develop"})
	if err != nil {
		t.Fatalf("AcquireWithOptions failed: %v", err)
	}

	if got := gitOut(t, wtPath, "rev-parse", "HEAD"); got != developTip {
		t.Errorf("worktree HEAD = %s, want develop tip %s", got, developTip)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "develop-only.txt")); err != nil {
		t.Errorf("expected the worktree to be cut from develop: %v", err)
	}
}

// The default branch stays the default. A repository that has a develop branch
// but no configured base must behave exactly as it did before this option
// existed.
func TestAcquire_WithoutBaseBranchKeepsDefaultBranch(t *testing.T) {
	repoDir, poolDir := setupRepo(t)
	addBranch(t, repoDir, "develop", "develop-only.txt")
	mainTip := gitOut(t, repoDir, "rev-parse", "HEAD")

	wtPath, err := Acquire(repoDir, poolDir, 1, nil)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	if got := gitOut(t, wtPath, "rev-parse", "HEAD"); got != mainTip {
		t.Errorf("worktree HEAD = %s, want main tip %s", got, mainTip)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "develop-only.txt")); !os.IsNotExist(err) {
		t.Error("expected the worktree to be cut from main, not develop")
	}
}

// A base that cannot be resolved must stop the acquisition, not fall back to
// the inferred default. Falling back would hand out a worktree cut from the
// wrong branch and report success.
func TestAcquire_UnknownBaseBranchFailsClosedWithoutCreatingAWorktree(t *testing.T) {
	repoDir, poolDir := setupRepo(t)

	_, err := AcquireWithOptions(repoDir, poolDir, 1, nil, AcquireOptions{BaseBranch: "no-such-branch"})
	if err == nil {
		t.Fatal("expected an unresolvable base branch to fail closed")
	}
	if !strings.Contains(err.Error(), "no-such-branch") {
		t.Errorf("error %q does not name the requested branch", err)
	}

	state, readErr := ReadState(poolDir)
	if readErr == nil && len(state.Worktrees) != 0 {
		t.Errorf("expected no worktree to be created, got %d", len(state.Worktrees))
	}
	if _, err := os.Stat(filepath.Join(poolDir, "1")); err == nil {
		t.Error("expected no worktree directory to be left behind")
	}
}

// Pools predate the option: their slots were cut from the old default. Once a
// base branch is configured, the next acquire has to be able to recycle those
// slots onto it, or every existing pool would need a manual destroy.
func TestAcquire_RecyclesExistingSlotOntoNewBaseBranch(t *testing.T) {
	repoDir, poolDir := setupRepo(t)

	wtPath, err := Acquire(repoDir, poolDir, 1, nil)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	clearOwnerReservation(t, poolDir, wtPath)

	// develop is ahead of main, so the slot's HEAD is merged into it and the
	// slot holds nothing that would be lost.
	developTip := addBranch(t, repoDir, "develop", "develop-only.txt")

	reused, err := AcquireWithOptions(repoDir, poolDir, 1, nil, AcquireOptions{BaseBranch: "develop"})
	if err != nil {
		t.Fatalf("expected the existing slot to be recycled onto develop: %v", err)
	}
	if reused != wtPath {
		t.Fatalf("expected reuse of slot %s, got %s", wtPath, reused)
	}
	if got := gitOut(t, reused, "rev-parse", "HEAD"); got != developTip {
		t.Errorf("recycled worktree HEAD = %s, want develop tip %s", got, developTip)
	}
}

// Changing the base must not turn the unlanded-work guard off. A slot holding
// commits that are not in the new base is not disposable just because the base
// changed.
func TestAcquire_SkipsSlotHoldingWorkNotMergedIntoNewBaseBranch(t *testing.T) {
	repoDir, poolDir := setupRepo(t)

	wtPath, err := Acquire(repoDir, poolDir, 1, nil)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "unlanded.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wtPath, "add", "unlanded.txt")
	runGit(t, wtPath, "commit", "-m", "committed but unlanded")
	head := gitOut(t, wtPath, "rev-parse", "HEAD")
	clearOwnerReservation(t, poolDir, wtPath)

	// develop is a sibling of main and does not contain the slot's commit.
	addBranch(t, repoDir, "develop", "develop-only.txt")

	if _, err := AcquireWithOptions(repoDir, poolDir, 1, nil, AcquireOptions{BaseBranch: "develop"}); err == nil {
		t.Fatal("expected acquire to fail closed rather than reset unlanded work onto the new base")
	}
	if got := gitOut(t, wtPath, "rev-parse", "HEAD"); got != head {
		t.Fatalf("expected unlanded HEAD %s preserved, got %s", head, got)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "unlanded.txt")); err != nil {
		t.Fatalf("expected unlanded commit preserved on disk: %v", err)
	}
}

// A branch that exists only on origin is the common case for base_branch: the
// person configuring it may never have checked that branch out locally.
func TestAcquire_ResolvesBaseBranchThatExistsOnlyOnOrigin(t *testing.T) {
	repoDir, poolDir := setupRepo(t)
	developTip := addBranch(t, repoDir, "develop", "develop-only.txt")
	runGit(t, repoDir, "push", "origin", "develop")
	runGit(t, repoDir, "branch", "-D", "develop")

	wtPath, err := AcquireWithOptions(repoDir, poolDir, 1, nil, AcquireOptions{BaseBranch: "develop"})
	if err != nil {
		t.Fatalf("AcquireWithOptions failed for a remote-only base branch: %v", err)
	}
	if got := gitOut(t, wtPath, "rev-parse", "HEAD"); got != developTip {
		t.Errorf("worktree HEAD = %s, want origin/develop tip %s", got, developTip)
	}
}

func TestAcquireLeaseInfo_ReportsResolvedBaseBranch(t *testing.T) {
	repoDir, poolDir := setupRepo(t)
	addBranch(t, repoDir, "develop", "develop-only.txt")

	lease, err := AcquireLeaseInfoWithOptions(repoDir, poolDir, 1, nil, "holder", AcquireOptions{BaseBranch: "develop"})
	if err != nil {
		t.Fatalf("AcquireLeaseInfoWithOptions failed: %v", err)
	}
	if lease.BaseBranch != "develop" {
		t.Errorf("lease BaseBranch = %q, want develop", lease.BaseBranch)
	}
}

// With no base configured the reported base is the inferred default, so a
// caller always learns which branch it actually got rather than an empty
// field it has to interpret.
func TestAcquireLeaseInfo_ReportsInferredDefaultBranch(t *testing.T) {
	repoDir, poolDir := setupRepo(t)

	lease, err := AcquireLeaseInfo(repoDir, poolDir, 1, nil, "holder")
	if err != nil {
		t.Fatalf("AcquireLeaseInfo failed: %v", err)
	}
	if lease.BaseBranch != "main" {
		t.Errorf("lease BaseBranch = %q, want main", lease.BaseBranch)
	}
}
