package git

import (
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

func TestValidateSafeReturnStateChecksExactHeadAttachment(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	head := gitOutput(t, repoDir, "rev-parse", "HEAD")

	if err := ValidateSafeReturnState(repoDir, head, "refs/heads/main"); err != nil {
		t.Fatalf("attached local ref should validate: %v", err)
	}
	mustGit(t, repoDir, "update-ref", "refs/remotes/origin/main", head)
	if err := ValidateSafeReturnState(repoDir, head, "refs/remotes/origin/main"); err == nil ||
		!strings.Contains(err.Error(), "must be detached") {
		t.Fatalf("attached remote ref error = %v, want detached refusal", err)
	}

	mustGit(t, repoDir, "checkout", "--detach", head)
	if err := ValidateSafeReturnState(repoDir, head, "refs/remotes/origin/main"); err != nil {
		t.Fatalf("detached remote ref should validate: %v", err)
	}
	mustGit(t, repoDir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	if err := ValidateSafeReturnState(repoDir, head, "refs/remotes/origin/HEAD"); err == nil ||
		!strings.Contains(err.Error(), "must not be symbolic") {
		t.Fatalf("symbolic remote ref error = %v, want symbolic refusal", err)
	}
	if err := ValidateSafeReturnState(repoDir, head, "refs/heads/main"); err == nil ||
		!strings.Contains(err.Error(), "is not attached") {
		t.Fatalf("detached local ref error = %v, want attached refusal", err)
	}
}

func TestValidateSafeReturnStateRejectsUnsafeInputsAndOperations(t *testing.T) {
	repoDir := t.TempDir()
	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	head := gitOutput(t, repoDir, "rev-parse", "HEAD")

	for _, tc := range []struct {
		name string
		head string
		ref  string
		want string
	}{
		{name: "short head", head: head[:12], ref: "refs/heads/main", want: "full commit object ID"},
		{name: "non-hex head", head: strings.Repeat("z", len(head)), ref: "refs/heads/main", want: "full commit object ID"},
		{name: "tag ref", head: head, ref: "refs/tags/main", want: "safe return ref must be under"},
		{name: "remote other than origin", head: head, ref: "refs/remotes/upstream/main", want: "safe return ref must be under"},
		{name: "malformed ref", head: head, ref: "refs/heads/../main", want: "invalid safe return ref"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSafeReturnState(repoDir, tc.head, tc.ref)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateSafeReturnState error = %v, want %q", err, tc.want)
			}
		})
	}

	for _, operation := range []struct {
		path string
		want string
		dir  bool
	}{
		{path: "MERGE_HEAD", want: "merge"},
		{path: "rebase-apply", want: "rebase", dir: true},
		{path: "rebase-merge", want: "rebase", dir: true},
		{path: "CHERRY_PICK_HEAD", want: "cherry-pick"},
		{path: "REVERT_HEAD", want: "revert"},
		{path: "BISECT_LOG", want: "bisect"},
		{path: "sequencer", want: "sequencer", dir: true},
	} {
		t.Run(operation.want+" operation", func(t *testing.T) {
			path := gitOutput(t, repoDir, "rev-parse", "--git-path", operation.path)
			if !filepath.IsAbs(path) {
				path = filepath.Join(repoDir, path)
			}
			if operation.dir {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte(head+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = os.RemoveAll(path)
			})

			err := ValidateSafeReturnState(repoDir, head, "refs/heads/main")
			if err == nil || !strings.Contains(err.Error(), operation.want+" in progress") {
				t.Fatalf("operation error = %v, want %s refusal", err, operation.want)
			}
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
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
