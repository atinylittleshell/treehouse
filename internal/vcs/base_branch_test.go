package vcs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepoWithBranch(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "git", "init", "--initial-branch=main")
	mustRun(t, dir, "git", "config", "user.email", "test@test.com")
	mustRun(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "git", "add", ".")
	mustRun(t, dir, "git", "commit", "-m", "initial")
	if branch != "" {
		mustRun(t, dir, "git", "branch", branch)
	}
	return dir
}

func TestVerifyBaseBranchAcceptsExistingBranch(t *testing.T) {
	isolateUserConfig(t)
	dir := gitRepoWithBranch(t, "develop")

	if err := VerifyBaseBranch(dir, "develop"); err != nil {
		t.Fatalf("VerifyBaseBranch(develop) = %v, want nil", err)
	}
}

func TestVerifyBaseBranchRejectsMissingBranch(t *testing.T) {
	isolateUserConfig(t)
	dir := gitRepoWithBranch(t, "")

	err := VerifyBaseBranch(dir, "develop")
	if err == nil {
		t.Fatal("VerifyBaseBranch on a missing branch = nil, want an error")
	}
	for _, want := range []string{"develop", "origin/develop"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// git-only for now: under jj this must fail closed rather than silently fall
// back to the default bookmark.
func TestVerifyBaseBranchRefusesUnderJJBackend(t *testing.T) {
	isolateUserConfig(t)
	dir := fakeJJOnlyRepo(t)
	t.Setenv("TREEHOUSE_VCS", "jj")

	if got := backendFor(dir).Name(); got != "jj" {
		t.Fatalf("test fixture did not select the jj backend, got %q", got)
	}

	err := VerifyBaseBranch(dir, "develop")
	if err == nil {
		t.Fatal("VerifyBaseBranch under the jj backend = nil, want an error")
	}
	if !strings.Contains(err.Error(), "jj") {
		t.Errorf("error %q does not name the jj backend", err)
	}
}
