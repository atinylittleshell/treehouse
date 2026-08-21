package process

import (
	"os"
	"path/filepath"
	"testing"
)

// gopsutil returns an empty cwd (with no error) for processes whose working
// directory cannot be read - notably Windows system processes. Before the guard,
// filepath.Abs("") resolved to the caller's own directory, so such a process was
// wrongly matched whenever the caller ran from inside the worktree, which made
// return/destroy fail with unkillable phantoms like "System (4)". An empty cwd
// must never be treated as inside the worktree.
func TestCwdWithinWorktree_EmptyCwdNeverMatches(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Use the caller's own directory as the worktree so the pre-fix
	// filepath.Abs("") == wd path would have matched.
	if cwdWithinWorktree(resolvePath(wd), "") {
		t.Fatal("a process with an empty cwd must not be treated as inside the worktree")
	}
}

func TestCwdWithinWorktree_MatchesRootAndDescendant(t *testing.T) {
	root := resolvePath(t.TempDir())
	child := filepath.Join(root, "sub", "dir")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	if !cwdWithinWorktree(root, root) {
		t.Error("the worktree root itself should match")
	}
	if !cwdWithinWorktree(root, child) {
		t.Error("a descendant of the worktree should match")
	}
	if cwdWithinWorktree(root, filepath.Dir(root)) {
		t.Error("the worktree's parent should not match")
	}
}
