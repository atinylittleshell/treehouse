package gitvcs

import (
	"os"
	"path/filepath"
	"testing"
)

// setupBaseBranchRepo builds a repo whose origin holds a remote-only branch,
// so both halves of BranchExists are exercised.
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

// A tag or SHA resolves as a ref but is not a branch: the recycle guard's
// "merged into the base" question assumes a base that advances.
func TestBranchExistsRejectsNonBranchRefs(t *testing.T) {
	repoDir := setupBaseBranchRepo(t)
	// main needs a parent, or main^ and main~1 would not resolve for any
	// implementation and the assertion would hold vacuously.
	mustGit(t, repoDir, "commit", "--allow-empty", "-m", "second")
	mustGit(t, repoDir, "tag", "v1.0.0")

	head := gitOutput(t, repoDir, "rev-parse", "HEAD")
	// The trailing forms are revision EXPRESSIONS: rev-parse --verify resolves
	// them against a real branch, so a prefix check alone would pin the base to
	// a commit that never advances.
	for _, ref := range []string{"v1.0.0", head, "origin/main", "HEAD", "main^", "main~1", "main@{0}"} {
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

// git rev-parse ranks refs/tags/<name> above refs/heads/<name>, so a bare name
// would resolve a base branch to a same-named tag.
func TestBranchRefPrefersBranchOverSameNamedTag(t *testing.T) {
	repoDir := setupBaseBranchRepo(t)
	mustGit(t, repoDir, "branch", "release-1.0")
	branchTip := gitOutput(t, repoDir, "rev-parse", "refs/heads/release-1.0")
	mustGit(t, repoDir, "commit", "--allow-empty", "-m", "after the branch")
	mustGit(t, repoDir, "tag", "release-1.0")
	tagTip := gitOutput(t, repoDir, "rev-parse", "refs/tags/release-1.0")
	if branchTip == tagTip {
		t.Fatal("fixture needs the tag and the branch at different commits")
	}

	got, err := refCommit(repoDir, branchRef(repoDir, "release-1.0"))
	if err != nil {
		t.Fatalf("resolving the base failed: %v", err)
	}
	if got != branchTip {
		t.Errorf("base resolved to %s, want branch %s (tag is %s)", got, branchTip, tagTip)
	}
}

func TestAddWorktreeCutsFromBranchNotSameNamedTag(t *testing.T) {
	repoDir := setupBaseBranchRepo(t)
	mustGit(t, repoDir, "branch", "release-1.0")
	branchTip := gitOutput(t, repoDir, "rev-parse", "refs/heads/release-1.0")
	mustGit(t, repoDir, "commit", "--allow-empty", "-m", "after the branch")
	mustGit(t, repoDir, "tag", "release-1.0")

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := AddWorktree(repoDir, wtPath, "release-1.0"); err != nil {
		t.Fatalf("AddWorktree failed: %v", err)
	}
	if got := gitOutput(t, wtPath, "rev-parse", "HEAD"); got != branchTip {
		t.Errorf("worktree HEAD = %s, want branch tip %s", got, branchTip)
	}
}

// git clone writes refs/remotes/origin/HEAD, so the fully qualified prefixes
// alone accept "HEAD" as a base branch. The push-based fixture above never
// creates that ref, which is why it cannot prove the rejection.
func TestBranchExistsRejectsHeadInAClonedRepository(t *testing.T) {
	origin := setupBaseBranchRepo(t)
	clone := filepath.Join(t.TempDir(), "clone")
	mustGit(t, "", "clone", origin, clone)

	if _, err := runGit(clone, "rev-parse", "--verify", "refs/remotes/origin/HEAD"); err != nil {
		t.Skipf("this git does not create refs/remotes/origin/HEAD on clone: %v", err)
	}
	if BranchExists(clone, "HEAD") {
		t.Error("BranchExists(\"HEAD\") = true, want false: only branch names are accepted")
	}
}

func TestBranchMergeRefResolvesToTheBranchRef(t *testing.T) {
	repoDir := setupBaseBranchRepo(t)

	if got := BranchMergeRef(repoDir, "local-only"); got != "refs/heads/local-only" {
		t.Errorf("BranchMergeRef(\"local-only\") = %q, want refs/heads/local-only", got)
	}
	if got := BranchMergeRef(repoDir, "remote-only"); got != "refs/remotes/origin/remote-only" {
		t.Errorf("BranchMergeRef(\"remote-only\") = %q, want refs/remotes/origin/remote-only", got)
	}
	for _, name := range []string{"", "no-such-branch", "HEAD"} {
		if got := BranchMergeRef(repoDir, name); got != "" {
			t.Errorf("BranchMergeRef(%q) = %q, want \"\"", name, got)
		}
	}
}
