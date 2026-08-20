// Package vcs is the version-control seam for treehouse. Every VCS operation
// the rest of the codebase needs goes through this package, so the pool
// lifecycle, commands, and configuration stay backend-agnostic.
//
// Today git is the only backend, selected unconditionally, which keeps this
// package a pure refactor of the previous internal/git call sites. Additional
// backends plug in by implementing Backend and extending backendFor.
package vcs

import (
	"crypto/sha256"
	"fmt"

	"github.com/kunchenguid/treehouse/internal/vcs/gitvcs"
)

// Backend is the set of version-control operations treehouse's lifecycle
// needs. Path-taking methods accept either a repository root or a worktree
// path exactly as the underlying VCS command would.
type Backend interface {
	// Name identifies the backend (e.g. "git").
	Name() string
	// FindRepoRootFrom returns the root of the repository or worktree
	// containing dir. An empty dir means the current working directory.
	FindRepoRootFrom(dir string) (string, error)
	// FindMainRepoRootFrom resolves dir to the main repository root, mapping
	// linked worktrees back to the repository that owns them.
	FindMainRepoRootFrom(dir string) (string, error)
	// GetDefaultBranch returns the repository's default branch name.
	GetDefaultBranch(repoRoot string) (string, error)
	// CommonGitDir returns the shared git metadata directory used for
	// repo-local, untracked exclusions (info/exclude). Backends without a
	// usable git dir return an error and callers degrade gracefully.
	CommonGitDir(dir string) (string, error)
	// HasRemote reports whether the named remote exists.
	HasRemote(repoRoot, name string) bool
	// GetRemoteURL returns the origin remote URL.
	GetRemoteURL(repoRoot string) (string, error)
	// AddWorktree creates a new worktree at path based on branch.
	AddWorktree(repoRoot, path, branch string) error
	// PruneWorktrees clears bookkeeping for worktrees whose directories no
	// longer exist. It never touches live worktrees or their data.
	PruneWorktrees(repoRoot string) error
	// RemoveWorktree removes a worktree even if it has local changes.
	RemoveWorktree(repoRoot, path string) error
	// RemoveCleanWorktree removes a worktree, refusing if it is not clean.
	RemoveCleanWorktree(repoRoot, path string) error
	// Fetch updates refs from origin when an origin remote exists.
	Fetch(repoRoot string) error
	// ResetWorktree returns a worktree to a pristine checkout of branch,
	// discarding local modifications.
	ResetWorktree(worktreePath, branch string) error
	// DetachWorktree releases any branch the worktree has checked out so
	// pooled worktrees never hold branch names.
	DetachWorktree(worktreePath string) error
	// DefaultBranchMergeRef returns the fully qualified ref merge-safety
	// checks compare against, failing closed when it cannot be verified.
	DefaultBranchMergeRef(repoRoot string) (string, error)
	// IsHeadMergedIntoRef reports whether the worktree's current head is
	// merged into ref.
	IsHeadMergedIntoRef(worktreePath, ref string) (bool, error)
	// IsDirty reports tracked or untracked local changes.
	IsDirty(worktreePath string) (bool, error)
}

var gitBackend Backend = gitvcs.New()

// backendFor selects the backend responsible for path (a repository root,
// worktree path, or any directory inside one). Git is currently the only
// backend, so selection is unconditional; future backends hook in here.
func backendFor(path string) Backend {
	_ = path
	return gitBackend
}

// FindRepoRoot returns the repository or worktree root for the current
// working directory.
func FindRepoRoot() (string, error) { return backendFor("").FindRepoRootFrom("") }

// FindRepoRootFrom returns the repository or worktree root containing dir.
func FindRepoRootFrom(dir string) (string, error) { return backendFor(dir).FindRepoRootFrom(dir) }

// FindMainRepoRoot returns the main repository root for the current working
// directory, resolving linked worktrees back to their owning repository.
func FindMainRepoRoot() (string, error) { return backendFor("").FindMainRepoRootFrom("") }

// FindMainRepoRootFrom returns the main repository root for dir.
func FindMainRepoRootFrom(dir string) (string, error) {
	return backendFor(dir).FindMainRepoRootFrom(dir)
}

// GetDefaultBranch returns the repository's default branch name.
func GetDefaultBranch(repoRoot string) (string, error) {
	return backendFor(repoRoot).GetDefaultBranch(repoRoot)
}

// CommonGitDir returns the shared git metadata directory for the repo
// containing dir.
func CommonGitDir(dir string) (string, error) { return backendFor(dir).CommonGitDir(dir) }

// HasRemote reports whether the named remote exists.
func HasRemote(repoRoot, name string) bool { return backendFor(repoRoot).HasRemote(repoRoot, name) }

// GetRemoteURL returns the origin remote URL.
func GetRemoteURL(repoRoot string) (string, error) {
	return backendFor(repoRoot).GetRemoteURL(repoRoot)
}

// AddWorktree creates a new worktree at path based on branch.
func AddWorktree(repoRoot, path, branch string) error {
	return backendFor(repoRoot).AddWorktree(repoRoot, path, branch)
}

// PruneWorktrees clears bookkeeping for worktrees whose directories no longer
// exist.
func PruneWorktrees(repoRoot string) error { return backendFor(repoRoot).PruneWorktrees(repoRoot) }

// RemoveWorktree removes a worktree even if it has local changes.
func RemoveWorktree(repoRoot, path string) error {
	return backendFor(repoRoot).RemoveWorktree(repoRoot, path)
}

// RemoveCleanWorktree removes a worktree, refusing if it is not clean.
func RemoveCleanWorktree(repoRoot, path string) error {
	return backendFor(repoRoot).RemoveCleanWorktree(repoRoot, path)
}

// Fetch updates refs from origin when an origin remote exists.
func Fetch(repoRoot string) error { return backendFor(repoRoot).Fetch(repoRoot) }

// ResetWorktree returns a worktree to a pristine checkout of branch.
func ResetWorktree(worktreePath, branch string) error {
	return backendFor(worktreePath).ResetWorktree(worktreePath, branch)
}

// DetachWorktree releases any branch the worktree has checked out.
func DetachWorktree(worktreePath string) error {
	return backendFor(worktreePath).DetachWorktree(worktreePath)
}

// DefaultBranchMergeRef returns the fully qualified ref merge-safety checks
// compare against.
func DefaultBranchMergeRef(repoRoot string) (string, error) {
	return backendFor(repoRoot).DefaultBranchMergeRef(repoRoot)
}

// IsHeadMergedIntoRef reports whether the worktree's current head is merged
// into ref.
func IsHeadMergedIntoRef(worktreePath, ref string) (bool, error) {
	return backendFor(worktreePath).IsHeadMergedIntoRef(worktreePath, ref)
}

// IsDirty reports tracked or untracked local changes in the worktree.
func IsDirty(worktreePath string) (bool, error) {
	return backendFor(worktreePath).IsDirty(worktreePath)
}

// ShortHash returns a short stable hash of s, used for pool directory naming.
// It is VCS-independent.
func ShortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:3])
}
