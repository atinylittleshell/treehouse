package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoRootFromCommonGitDirHandlesForwardSlashPath(t *testing.T) {
	root, ok := repoRootFromCommonGitDir("C:/Users/runner/AppData/Local/Temp/repo/.git")
	if !ok {
		t.Fatal("expected .git common dir to resolve to a repo root")
	}

	want := filepath.Clean(filepath.FromSlash("C:/Users/runner/AppData/Local/Temp/repo"))
	if root != want {
		t.Fatalf("expected repo root %q, got %q", want, root)
	}
}

func TestGetDefaultBranchFromDetachedLinkedWorktreeUsesMainRepoHead(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	wtPath := filepath.Join(base, "worktree")

	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	mustGit(t, repoDir, "config", "init.defaultBranch", "wrong")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "worktree", "add", "--detach", wtPath, "main")

	branch, err := GetDefaultBranch(wtPath)
	if err != nil {
		t.Fatalf("GetDefaultBranch failed: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected default branch main from main repo HEAD, got %q", branch)
	}
}

func TestFindMainRepoRootFromLinkedWorktree(t *testing.T) {
	base := t.TempDir()
	base, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	repoDir := filepath.Join(base, "repo")
	wtPath := filepath.Join(base, "worktree")

	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "worktree", "add", "--detach", wtPath, "main")

	mainRoot, err := FindMainRepoRootFrom(wtPath)
	if err != nil {
		t.Fatalf("FindMainRepoRootFrom failed: %v", err)
	}
	if mainRoot != repoDir {
		t.Fatalf("expected main repo root %s, got %s", repoDir, mainRoot)
	}
}

func TestWithHEADLockBlocksConcurrentHEADChange(t *testing.T) {
	repoDir := t.TempDir()
	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "checkout", "--detach")
	headBefore := strings.TrimSpace(mustGitOutput(t, repoDir, "rev-parse", "HEAD"))

	err := WithHEADLock(repoDir, func() error {
		cmd := exec.Command("git", "update-ref", "HEAD", headBefore, headBefore)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("concurrent HEAD update succeeded while lock held; output: %s", out)
		}
		if got := strings.TrimSpace(mustGitOutput(t, repoDir, "rev-parse", "HEAD")); got != headBefore {
			t.Fatalf("HEAD changed under lock to %s, want %s", got, headBefore)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithHEADLock failed: %v", err)
	}

	mustGit(t, repoDir, "update-ref", "HEAD", headBefore, headBefore)
}

func TestWithHEADLockBlocksConcurrentAttachedCommit(t *testing.T) {
	repoDir := t.TempDir()
	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	headBefore := strings.TrimSpace(mustGitOutput(t, repoDir, "rev-parse", "HEAD"))

	err := WithHEADLock(repoDir, func() error {
		cmd := exec.Command("git", "commit", "--allow-empty", "-m", "concurrent")
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("concurrent attached commit succeeded while lock held; output: %s", out)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithHEADLock failed: %v", err)
	}
	if got := strings.TrimSpace(mustGitOutput(t, repoDir, "rev-parse", "HEAD")); got != headBefore {
		t.Fatalf("attached HEAD changed under lock to %s, want %s", got, headBefore)
	}
}

func TestWithHEADLockRejectsReftable(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo")
	cmd := exec.Command("git", "init", "--ref-format=reftable", "--initial-branch=main", repoDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("Git does not support reftable test repositories: %v\n%s", err, out)
	}

	called := false
	err := WithHEADLock(repoDir, func() error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrUnsupportedRefStorage) {
		t.Fatalf("WithHEADLock on reftable = %v, want ErrUnsupportedRefStorage", err)
	}
	if called {
		t.Fatal("WithHEADLock called protected operation for unsupported ref storage")
	}
}

func TestRemoveCleanWorktreeRejectsDirtyWorktree(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	wtPath := filepath.Join(base, "worktree")

	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "worktree", "add", "--detach", wtPath, "main")

	dirtyPath := filepath.Join(wtPath, "uncommitted.txt")
	if err := os.WriteFile(dirtyPath, []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveCleanWorktree(repoDir, wtPath); err == nil {
		t.Fatal("expected clean worktree removal to reject dirty worktree")
	}
	if _, err := os.Stat(dirtyPath); err != nil {
		t.Fatalf("expected dirty worktree to remain: %v", err)
	}
}

func TestIsHeadMergedIntoRef(t *testing.T) {
	tests := []struct {
		name                   string
		ordinaryMerge          bool
		squashMerge            bool
		laterUnrelated         bool
		targetFeatureContent   string
		emptyFeatureCommit     bool
		revertedFeatureContent bool
		wantMerged             bool
	}{
		{name: "ordinary ancestry merge", ordinaryMerge: true, wantMerged: true},
		{name: "squash merge", squashMerge: true, wantMerged: true},
		{name: "squash merge followed by unrelated target commit", squashMerge: true, laterUnrelated: true, wantMerged: true},
		{name: "squash merge missing final feature content", squashMerge: true, targetFeatureContent: "one\n", wantMerged: false},
		{name: "unique unmerged content", wantMerged: false},
		{name: "empty feature commit", emptyFeatureCommit: true, wantMerged: false},
		{name: "feature content fully reverted", revertedFeatureContent: true, wantMerged: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoDir := t.TempDir()
			mustGit(t, "", "init", "--initial-branch=main", repoDir)
			mustGit(t, repoDir, "config", "user.email", "test@test.com")
			mustGit(t, repoDir, "config", "user.name", "Test")

			if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			mustGit(t, repoDir, "add", ".")
			mustGit(t, repoDir, "commit", "-m", "initial")
			mustGit(t, repoDir, "checkout", "-b", "feature")

			switch {
			case tt.emptyFeatureCommit:
				mustGit(t, repoDir, "commit", "--allow-empty", "-m", "empty feature commit")
			case tt.revertedFeatureContent:
				if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("feature\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				mustGit(t, repoDir, "commit", "-am", "feature change")
				if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				mustGit(t, repoDir, "commit", "-am", "revert feature change")
			default:
				if err := os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("one\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				mustGit(t, repoDir, "add", "feature.txt")
				mustGit(t, repoDir, "commit", "-m", "feature one")
				if err := os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				mustGit(t, repoDir, "commit", "-am", "feature two")
			}

			mustGit(t, repoDir, "checkout", "main")
			switch {
			case tt.ordinaryMerge:
				mustGit(t, repoDir, "merge", "--no-ff", "feature", "-m", "merge feature")
			case tt.squashMerge:
				mustGit(t, repoDir, "merge", "--squash", "feature")
				if tt.targetFeatureContent != "" {
					if err := os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte(tt.targetFeatureContent), 0o644); err != nil {
						t.Fatal(err)
					}
					mustGit(t, repoDir, "add", "feature.txt")
				}
				mustGit(t, repoDir, "commit", "-m", "squash feature")
			}
			if tt.laterUnrelated {
				if err := os.WriteFile(filepath.Join(repoDir, "unrelated.txt"), []byte("later\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				mustGit(t, repoDir, "add", "unrelated.txt")
				mustGit(t, repoDir, "commit", "-m", "unrelated target change")
			}
			mustGit(t, repoDir, "checkout", "feature")

			merged, err := IsHeadMergedIntoRef(repoDir, "refs/heads/main")
			if err != nil {
				t.Fatalf("IsHeadMergedIntoRef failed: %v", err)
			}
			if merged != tt.wantMerged {
				t.Fatalf("expected merged=%t, got %t", tt.wantMerged, merged)
			}
		})
	}
}

func TestIsHeadMergedIntoRefFailsClosedWhenTargetCannotBeVerified(t *testing.T) {
	repoDir := t.TempDir()
	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")

	if _, err := IsHeadMergedIntoRef(repoDir, "refs/heads/missing"); err == nil {
		t.Fatal("expected merge verification error for missing target ref")
	}
}

func TestIsHeadReferenced(t *testing.T) {
	repoDir := t.TempDir()
	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")

	referenced, err := IsHeadReferenced(repoDir)
	if err != nil || !referenced {
		t.Fatalf("branch HEAD referenced=%t err=%v, want true", referenced, err)
	}

	mustGit(t, repoDir, "checkout", "--detach")
	if err := os.WriteFile(filepath.Join(repoDir, "detached.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", "detached.txt")
	mustGit(t, repoDir, "commit", "-m", "detached work")
	referenced, err = IsHeadReferenced(repoDir)
	if err != nil {
		t.Fatalf("IsHeadReferenced failed: %v", err)
	}
	if referenced {
		t.Fatal("detached commit unexpectedly has a durable ref")
	}

	mustGit(t, repoDir, "tag", "preserved")
	referenced, err = IsHeadReferenced(repoDir)
	if err != nil || !referenced {
		t.Fatalf("tagged HEAD referenced=%t err=%v, want true", referenced, err)
	}
	head := strings.TrimSpace(mustGitOutput(t, repoDir, "rev-parse", "HEAD"))
	ref, err := PreserveHEAD(repoDir, "lease-identity")
	if err != nil {
		t.Fatalf("PreserveHEAD failed: %v", err)
	}
	if ref != GuardedRecoveryRef("lease-identity") {
		t.Fatalf("PreserveHEAD ref = %q, want %q", ref, GuardedRecoveryRef("lease-identity"))
	}
	if preserved := strings.TrimSpace(mustGitOutput(t, repoDir, "rev-parse", ref)); preserved != head {
		t.Fatalf("recovery ref points to %s, want %s", preserved, head)
	}
	if _, err := PreserveHEAD(repoDir, "lease-identity"); err != nil {
		t.Fatalf("idempotent PreserveHEAD failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoDir, "later.txt"), []byte("later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", "later.txt")
	mustGit(t, repoDir, "commit", "-m", "later detached work")
	if _, err := PreserveHEAD(repoDir, "lease-identity"); err == nil || !strings.Contains(err.Error(), "already protects different HEAD") {
		t.Fatalf("PreserveHEAD changed target error = %v, want fail-closed mismatch", err)
	}
	if preserved := strings.TrimSpace(mustGitOutput(t, repoDir, "rev-parse", ref)); preserved != head {
		t.Fatalf("failed retry overwrote recovery ref with %s, want %s", preserved, head)
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func mustGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
