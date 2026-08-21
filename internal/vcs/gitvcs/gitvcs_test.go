package gitvcs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestIsWorktreeSafeToReset(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	wtPath := filepath.Join(base, "worktree")

	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "worktree", "add", "--detach", wtPath, "main")

	// At base: resetting discards nothing, so it is safe.
	safe, _, _, err := IsWorktreeSafeToReset(wtPath, "main")
	if err != nil {
		t.Fatalf("IsWorktreeSafeToReset at base: %v", err)
	}
	if !safe {
		t.Fatal("expected a worktree at base to be safe to reset")
	}

	// Ahead of base: resetting would discard the commit, so it is not safe.
	if err := os.WriteFile(filepath.Join(wtPath, "unlanded.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, wtPath, "add", "unlanded.txt")
	mustGit(t, wtPath, "commit", "-m", "unlanded work")

	safe, _, _, err = IsWorktreeSafeToReset(wtPath, "main")
	if err != nil {
		t.Fatalf("IsWorktreeSafeToReset ahead of base: %v", err)
	}
	if safe {
		t.Fatal("expected a worktree ahead of base to be refused")
	}
}

func TestResetWorktreeUsesCommitVerifiedBySafetyCheck(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	wtPath := filepath.Join(base, "worktree")

	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "worktree", "add", "--detach", wtPath, "main")

	safe, resetRef, head, err := IsWorktreeSafeToReset(wtPath, "main")
	if err != nil {
		t.Fatalf("IsWorktreeSafeToReset: %v", err)
	}
	if !safe {
		t.Fatal("expected worktree at base to be safe to reset")
	}

	if err := os.WriteFile(filepath.Join(repoDir, "advanced.txt"), []byte("new base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", "advanced.txt")
	mustGit(t, repoDir, "commit", "-m", "advance main")

	if err := ResetWorktreeToRef(wtPath, resetRef, head, true); err != nil {
		t.Fatalf("ResetWorktreeToRef: %v", err)
	}
	got, err := runGit(wtPath, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("resolve reset HEAD: %v", err)
	}
	if got != resetRef {
		t.Fatalf("reset targeted %s, want verified commit %s", got, resetRef)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "README.md")); err != nil {
		t.Fatalf("expected committed work to remain: %v", err)
	}
}

func TestResetWorktreeToRefRefusesWhenHeadChanged(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	wtPath := filepath.Join(base, "worktree")

	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "worktree", "add", "--detach", wtPath, "main")

	safe, resetRef, head, err := IsWorktreeSafeToReset(wtPath, "main")
	if err != nil {
		t.Fatalf("IsWorktreeSafeToReset: %v", err)
	}
	if !safe {
		t.Fatal("expected worktree at base to be safe to reset")
	}

	// Concurrent committed work after the safety check: HEAD is no longer the
	// one whose ancestry was verified, so the reset must refuse.
	if err := os.WriteFile(filepath.Join(wtPath, "unlanded.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, wtPath, "add", "unlanded.txt")
	mustGit(t, wtPath, "commit", "-m", "unlanded after check")
	changed, err := runGit(wtPath, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("resolve changed HEAD: %v", err)
	}
	if changed == head {
		t.Fatal("expected HEAD to change after the concurrent commit")
	}

	if err := ResetWorktreeToRef(wtPath, resetRef, head, true); err == nil {
		t.Fatal("expected ResetWorktreeToRef to refuse after HEAD changed")
	}
	got, err := runGit(wtPath, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("resolve preserved HEAD: %v", err)
	}
	if got != changed {
		t.Fatalf("expected unlanded HEAD %s preserved, got %s", changed, got)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "unlanded.txt")); err != nil {
		t.Fatalf("expected concurrent commit preserved on disk: %v", err)
	}
}

func TestResetWorktreeToRefRefusesWhenHeadLockHeld(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	wtPath := filepath.Join(base, "worktree")

	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "worktree", "add", "--detach", wtPath, "main")

	safe, resetRef, head, err := IsWorktreeSafeToReset(wtPath, "main")
	if err != nil {
		t.Fatalf("IsWorktreeSafeToReset: %v", err)
	}
	if !safe {
		t.Fatal("expected worktree at base to be safe to reset")
	}

	headPath, err := gitPath(wtPath, "HEAD")
	if err != nil {
		t.Fatalf("resolve HEAD path: %v", err)
	}
	lockPath := headPath + ".lock"
	lf, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		t.Fatalf("create HEAD.lock: %v", err)
	}
	defer func() {
		_ = lf.Close()
		_ = os.Remove(lockPath)
	}()

	if err := ResetWorktreeToRef(wtPath, resetRef, head, true); err == nil {
		t.Fatal("expected ResetWorktreeToRef to refuse when git HEAD.lock is held")
	}
	got, err := runGit(wtPath, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("resolve preserved HEAD: %v", err)
	}
	if got != head {
		t.Fatalf("expected HEAD %s preserved under HEAD.lock, got %s", head, got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "commit", "--allow-empty", "-m", "concurrent")
	cmd.Dir = wtPath
	if err := cmd.Run(); err == nil {
		t.Fatal("expected git commit to honor an existing HEAD.lock")
	}
	if got, err := runGit(wtPath, "rev-parse", "HEAD"); err != nil {
		t.Fatalf("resolve HEAD after concurrent commit: %v", err)
	} else if got != head {
		t.Fatalf("expected git commit not to move HEAD while locked, got %s", got)
	}
}

func TestResetWorktreeToRefRefusesWhenDirtyAfterSafetyCheck(t *testing.T) {
	cases := []struct {
		name  string
		dirty func(t *testing.T, wtPath string) (keepPath, keepContents string)
	}{
		{
			name: "untracked file",
			dirty: func(t *testing.T, wtPath string) (string, string) {
				path := filepath.Join(wtPath, "scratch.txt")
				if err := os.WriteFile(path, []byte("keep\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return path, "keep\n"
			},
		},
		{
			name: "tracked modification",
			dirty: func(t *testing.T, wtPath string) (string, string) {
				path := filepath.Join(wtPath, "README.md")
				if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return path, "changed\n"
			},
		},
		{
			name: "index update",
			dirty: func(t *testing.T, wtPath string) (string, string) {
				path := filepath.Join(wtPath, "staged.txt")
				if err := os.WriteFile(path, []byte("keep\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				mustGit(t, wtPath, "add", "staged.txt")
				return path, "keep\n"
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wtPath, resetRef, head := setupSafeResetWorktree(t)
			keepPath, keepContents := tc.dirty(t, wtPath)

			err := ResetWorktreeToRef(wtPath, resetRef, head, true)
			if err == nil {
				t.Fatal("expected ResetWorktreeToRef to refuse after the tree became dirty")
			}
			if !strings.Contains(err.Error(), "became dirty after safety check") {
				t.Fatalf("expected dirty-after-check error, got %v", err)
			}

			got, err := runGit(wtPath, "rev-parse", "HEAD")
			if err != nil {
				t.Fatalf("resolve preserved HEAD: %v", err)
			}
			if got != head {
				t.Fatalf("expected HEAD %s preserved, got %s", head, got)
			}
			contents, err := os.ReadFile(keepPath)
			if err != nil {
				t.Fatalf("expected concurrent uncommitted work preserved on disk: %v", err)
			}
			if string(contents) != keepContents {
				t.Fatalf("expected %q preserved, got %q", keepContents, contents)
			}
		})
	}
}

func TestResetWorktreeToRefDiscardsDirtyWithoutRequireClean(t *testing.T) {
	wtPath, resetRef, head := setupSafeResetWorktree(t)
	scratch := filepath.Join(wtPath, "scratch.txt")
	if err := os.WriteFile(scratch, []byte("discard\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ResetWorktreeToRef(wtPath, resetRef, head, false); err != nil {
		t.Fatalf("ResetWorktreeToRef without requireClean: %v", err)
	}
	got, err := runGit(wtPath, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("resolve reset HEAD: %v", err)
	}
	if got != resetRef {
		t.Fatalf("reset targeted %s, want %s", got, resetRef)
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatal("expected return-path reset to discard uncommitted work")
	}
}

func setupSafeResetWorktree(t *testing.T) (wtPath, resetRef, head string) {
	t.Helper()
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	wtPath = filepath.Join(base, "worktree")

	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "worktree", "add", "--detach", wtPath, "main")

	safe, resetRef, head, err := IsWorktreeSafeToReset(wtPath, "main")
	if err != nil {
		t.Fatalf("IsWorktreeSafeToReset: %v", err)
	}
	if !safe {
		t.Fatal("expected worktree at base to be safe to reset")
	}
	return wtPath, resetRef, head
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
