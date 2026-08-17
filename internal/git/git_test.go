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

func TestIsDirtyFindsSubmoduleChangesHiddenByConfig(t *testing.T) {
	base := t.TempDir()
	childRepo := filepath.Join(base, "child")
	mustGit(t, "", "init", "--initial-branch=main", childRepo)
	mustGit(t, childRepo, "config", "user.email", "test@test.com")
	mustGit(t, childRepo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(childRepo, "tracked.txt"), []byte("clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, childRepo, "add", ".")
	mustGit(t, childRepo, "commit", "-m", "initial")

	repoDir := filepath.Join(base, "repo")
	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	mustGit(t, repoDir, "-c", "protocol.file.allow=always", "submodule", "add", childRepo, "child")
	mustGit(t, repoDir, "commit", "-am", "add child")
	mustGit(t, repoDir, "config", "submodule.child.ignore", "all")

	if err := os.WriteFile(filepath.Join(repoDir, "child", "tracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := IsDirty(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("dirty submodule hidden by config was reported clean")
	}
}

func TestSafeRepositoryStateIgnoresGitRepositoryEnvironment(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	decoy := filepath.Join(base, "decoy")
	for _, repo := range []string{target, decoy} {
		mustGit(t, "", "init", "--initial-branch=main", repo)
		mustGit(t, repo, "config", "user.email", "test@test.com")
		mustGit(t, repo, "config", "user.name", "Test")
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("clean\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit(t, repo, "add", ".")
		mustGit(t, repo, "commit", "-m", "initial")
	}
	if err := os.WriteFile(filepath.Join(target, "tracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)
	if err := validateSafeRepositoryState(target); err == nil ||
		!strings.Contains(err.Error(), "worktree has uncommitted changes") {
		t.Fatalf("safe state with redirected Git environment = %v, want dirty refusal", err)
	}
}

func TestSafeRepositoryStateDoesNotDiscoverParentRepository(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "parent")
	target := filepath.Join(parent, "target")
	for _, repo := range []string{parent, target} {
		mustGit(t, "", "init", "--initial-branch=main", repo)
		mustGit(t, repo, "config", "user.email", "test@test.com")
		mustGit(t, repo, "config", "user.name", "Test")
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("clean\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit(t, repo, "add", ".")
		mustGit(t, repo, "commit", "-m", "initial")
	}
	if err := os.RemoveAll(filepath.Join(target, ".git")); err != nil {
		t.Fatal(err)
	}

	if err := validateSafeRepositoryState(target); err == nil ||
		!strings.Contains(err.Error(), "inspect safe Git marker") {
		t.Fatalf("safe state without target .git marker = %v, want refusal", err)
	}
}

func TestSafeReturnRejectsCopiedLinkedWorktreeMarker(t *testing.T) {
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	first := filepath.Join(base, "first")
	second := filepath.Join(base, "second")
	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repoDir, "tracked.txt"), []byte("clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", ".")
	mustGit(t, repoDir, "commit", "-m", "initial")
	mustGit(t, repoDir, "worktree", "add", "--detach", first, "main")
	mustGit(t, repoDir, "worktree", "add", "--detach", second, "main")
	head, err := runGit(first, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	secondMarker, err := os.ReadFile(filepath.Join(second, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, ".git"), secondMarker, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSafeReturnState(first, head, "refs/heads/main"); err == nil ||
		!strings.Contains(err.Error(), "safe Git backlink mismatch") {
		t.Fatalf("safe return with copied worktree marker = %v, want backlink refusal", err)
	}
}

func TestSanitizeGitEnvironment(t *testing.T) {
	got := sanitizeGitEnvironment([]string{
		"PATH=/usr/bin",
		"GIT_DIR=/tmp/decoy",
		"git_work_tree=/tmp/decoy",
		"GIT_CONFIG_KEY_0=core.worktree",
		"GIT_CONFIG_VALUE_0=/tmp/decoy",
		"GIT_SSH_COMMAND=ssh -i key",
	})
	joined := strings.Join(got, "\n")
	for _, blocked := range []string{"GIT_DIR=", "git_work_tree=", "GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_"} {
		if strings.Contains(joined, blocked) {
			t.Fatalf("sanitized environment retained %q: %v", blocked, got)
		}
	}
	for _, retained := range []string{"PATH=/usr/bin", "GIT_SSH_COMMAND=ssh -i key"} {
		if !strings.Contains(joined, retained) {
			t.Fatalf("sanitized environment removed %q: %v", retained, got)
		}
	}
}

func TestSafeRepositoryStateRecursesIntoSubmodules(t *testing.T) {
	base := t.TempDir()
	deepRepo := filepath.Join(base, "deep-source")
	mustGit(t, "", "init", "--initial-branch=main", deepRepo)
	mustGit(t, deepRepo, "config", "user.email", "test@test.com")
	mustGit(t, deepRepo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(deepRepo, "tracked.txt"), []byte("clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, deepRepo, "add", ".")
	mustGit(t, deepRepo, "commit", "-m", "initial")

	childRepo := filepath.Join(base, "child-source")
	mustGit(t, "", "init", "--initial-branch=main", childRepo)
	mustGit(t, childRepo, "config", "user.email", "test@test.com")
	mustGit(t, childRepo, "config", "user.name", "Test")
	mustGit(t, childRepo, "-c", "protocol.file.allow=always", "submodule", "add", deepRepo, "nested")
	mustGit(t, childRepo, "commit", "-am", "add nested child")

	repoDir := filepath.Join(base, "repo")
	mustGit(t, "", "init", "--initial-branch=main", repoDir)
	mustGit(t, repoDir, "config", "user.email", "test@test.com")
	mustGit(t, repoDir, "config", "user.name", "Test")
	mustGit(t, repoDir, "-c", "protocol.file.allow=always", "submodule", "add", childRepo, "child")
	mustGit(t, repoDir, "commit", "-am", "add child")
	mustGit(t, repoDir, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")
	childLabel := "submodule child"
	deepPath := filepath.Join(repoDir, "child", "nested")
	deepLabel := "submodule " + filepath.Join("child", "nested")

	mustGit(t, repoDir, "config", "submodule.child.ignore", "all")
	mustGit(t, filepath.Join(repoDir, "child"), "config", "submodule.nested.ignore", "all")
	if err := os.WriteFile(filepath.Join(deepPath, "tracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateSafeRepositoryState(repoDir); err == nil ||
		!strings.Contains(err.Error(), childLabel+" has uncommitted changes") {
		t.Fatalf("nested dirty state error = %v, want uncommitted change refusal", err)
	}
	mustGit(t, deepPath, "checkout", "--", "tracked.txt")

	mustGit(t, deepPath, "update-index", "--assume-unchanged", "tracked.txt")
	if err := os.WriteFile(filepath.Join(deepPath, "tracked.txt"), []byte("hidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateSafeRepositoryState(repoDir); err == nil ||
		!strings.Contains(err.Error(), deepLabel+" has assume-unchanged") {
		t.Fatalf("hidden submodule state error = %v, want index flag refusal", err)
	}

	mustGit(t, deepPath, "update-index", "--no-assume-unchanged", "tracked.txt")
	mustGit(t, deepPath, "checkout", "--", "tracked.txt")
	mergeHead := gitOutput(t, deepPath, "rev-parse", "--git-path", "MERGE_HEAD")
	if !filepath.IsAbs(mergeHead) {
		mergeHead = filepath.Join(deepPath, mergeHead)
	}
	if err := os.WriteFile(mergeHead, []byte(gitOutput(t, deepPath, "rev-parse", "HEAD")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateSafeRepositoryState(repoDir); err == nil ||
		!strings.Contains(err.Error(), deepLabel+" has merge in progress") {
		t.Fatalf("submodule operation error = %v, want merge refusal", err)
	}
}

func TestSafeReturnRejectsUnsafeIndexFlagsWithoutChangingIsDirty(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag string
	}{
		{name: "assume unchanged", flag: "--assume-unchanged"},
		{name: "skip worktree", flag: "--skip-worktree"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoDir := t.TempDir()
			mustGit(t, "", "init", "--initial-branch=main", repoDir)
			mustGit(t, repoDir, "config", "user.email", "test@test.com")
			mustGit(t, repoDir, "config", "user.name", "Test")
			trackedPath := filepath.Join(repoDir, "tracked.txt")
			if err := os.WriteFile(trackedPath, []byte("clean\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			mustGit(t, repoDir, "add", ".")
			mustGit(t, repoDir, "commit", "-m", "initial")
			mustGit(t, repoDir, "update-index", tc.flag, "tracked.txt")

			dirty, err := IsDirty(repoDir)
			if err != nil {
				t.Fatal(err)
			}
			if dirty {
				t.Fatalf("clean file with %s index flag was reported dirty", tc.flag)
			}
			if err := validateSafeRepositoryState(repoDir); err == nil ||
				!strings.Contains(err.Error(), "index flags") {
				t.Fatalf("safe validation error = %v, want index flag refusal", err)
			}

			if err := os.WriteFile(trackedPath, []byte("dirty\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := validateSafeRepositoryState(repoDir); err == nil ||
				!strings.Contains(err.Error(), "index flags") {
				t.Fatalf("safe validation after modification = %v, want index flag refusal", err)
			}
		})
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
	mustGit(t, repoDir, "branch", "other", head)
	mustGit(t, repoDir, "checkout", "other")
	if err := ValidateSafeReturnState(repoDir, head, "refs/heads/main"); err == nil ||
		!strings.Contains(err.Error(), "is not attached") {
		t.Fatalf("mismatched attached local ref error = %v, want attachment refusal", err)
	}
	mustGit(t, repoDir, "checkout", "main")
	mustGit(t, repoDir, "checkout", "--detach", head)
	if err := ValidateSafeReturnState(repoDir, head, "refs/heads/main"); err != nil {
		t.Fatalf("detached local ref should validate: %v", err)
	}
	mustGit(t, repoDir, "checkout", "main")

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
	if err := ValidateSafeReturnState(repoDir, head, "refs/heads/main"); err != nil {
		t.Fatalf("detached local ref should remain valid with origin refs: %v", err)
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
