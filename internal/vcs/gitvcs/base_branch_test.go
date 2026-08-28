package gitvcs

import (
	"os"
	"path/filepath"
	"testing"
)

// setupBaseBranchRepo builds a repo with a bare origin holding a branch that
// exists only on the remote, so the local/remote halves of BranchExists are
// exercised independently.
func setupBaseBranchRepo(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	base, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}

	bareDir := filepath.Join(base, "remote.git")
	repoDir := filepath.Join(base, "repo")

	mustGit(t, "", "init", "--bare", "--initial-branch=main", bareDir)
	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	mustGit(t, repoDir, "remote", "add", "origin", bareDir)
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "push", "-u", "origin", "main")

	// remote-only: pushed, then the local branch is deleted so only
	// refs/remotes/origin/remote-only remains.
	mustGit(t, repoDir, "checkout", "-b", "remote-only")
	mustGit(t, repoDir, "commit", "--allow-empty", "-m", "remote only")
	mustGit(t, repoDir, "push", "origin", "remote-only")
	mustGit(t, repoDir, "checkout", "main")
	mustGit(t, repoDir, "branch", "-D", "remote-only")

	// local-only: never pushed.
	mustGit(t, repoDir, "branch", "local-only")

	return repoDir
}

func TestBranchExists(t *testing.T) {
	repoDir := setupBaseBranchRepo(t)

	// BranchExists must accept exactly what branchRef can resolve, otherwise
	// verification and the reset that follows it could disagree.
	for _, branch := range []string{"main", "local-only", "remote-only"} {
		if !BranchExists(repoDir, branch) {
			t.Errorf("BranchExists(%q) = false, want true", branch)
		}
	}
}

func TestBranchExistsReportsMissingBranch(t *testing.T) {
	repoDir := setupBaseBranchRepo(t)

	if BranchExists(repoDir, "no-such-branch") {
		t.Error("BranchExists(\"no-such-branch\") = true, want false")
	}
}

// A tag or a commit SHA resolves as a git ref but is not a branch. Accepting
// one would let a worktree be pinned to a ref that never advances, which the
// recycle guard's "HEAD merged into the base" question is not written for.
func TestBranchExistsRejectsNonBranchRefs(t *testing.T) {
	repoDir := setupBaseBranchRepo(t)
	mustGit(t, repoDir, "tag", "v1.0.0")

	head := gitOutput(t, repoDir, "rev-parse", "HEAD")
	for _, ref := range []string{"v1.0.0", head, "origin/main", "HEAD"} {
		if BranchExists(repoDir, ref) {
			t.Errorf("BranchExists(%q) = true, want false: only branch names are accepted", ref)
		}
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGit(dir, args...)
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return out
}
