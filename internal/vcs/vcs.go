// Package vcs is the version-control seam for treehouse. Every VCS operation
// the rest of the codebase needs goes through this package, so the pool
// lifecycle, commands, and configuration stay backend-agnostic.
//
// Git is the default backend everywhere. The jj backend is strictly an
// explicit opt-in via the TREEHOUSE_VCS environment variable, the "vcs" key
// in the repository's treehouse.toml, or the "vcs" key in the user-level
// ~/.config/treehouse/config.toml, in that precedence order. The jj opt-in
// only takes effect where a .jj directory actually exists: in a repository
// without one, an ambient "jj" opt-in silently keeps the git backend, so a
// shell-wide TREEHOUSE_VCS=jj never breaks plain git repositories. Colocated
// repositories (both .jj and .git) stay on git worktrees without the opt-in,
// and a .jj-only repository without the opt-in simply keeps git's error
// behavior. Pooled jj workspaces are .jj-only trees that cannot carry
// an untracked config file, so they inherit the opt-in from their main
// repository root, located by reading the .jj/repo pointer — file
// inspection only; the decision still comes from explicit configuration.
package vcs

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/kunchenguid/treehouse/internal/vcs/gitvcs"
	"github.com/kunchenguid/treehouse/internal/vcs/jjvcs"
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
	// ResetWorktreeToRef resets worktreePath to an already resolved commit.
	// Callers that verified safety must pass the reset target and worktree
	// HEAD returned by IsWorktreeSafeToReset. The reset re-reads HEAD and,
	// when requireClean is set, re-checks dirtiness under the exclusive
	// lock before any destructive tree update, so concurrent uncommitted
	// work is not discarded. Refuse if HEAD changed, the lock cannot be
	// taken, or (when requireClean) the tree is dirty.
	ResetWorktreeToRef(worktreePath, ref, expectedHead string, requireClean bool) error
	// IsWorktreeSafeToReset reports whether worktreePath can be reset to
	// branch without discarding committed work. It returns the immutable
	// reset target and the worktree HEAD recorded at check time. Callers
	// must pass both to ResetWorktreeToRef. The check fails closed when
	// the target or HEAD cannot be resolved.
	IsWorktreeSafeToReset(worktreePath, branch string) (bool, string, string, error)
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

var (
	gitBackend Backend = gitvcs.New()
	jjBackend  Backend = jjvcs.New()
)

// backendFor selects the backend responsible for path (a repository root,
// worktree path, or any directory inside one). Git is the default
// everywhere; TREEHOUSE_VCS, a repo-root treehouse.toml "vcs" key, or a
// user-level config "vcs" key opts in to jj explicitly (see vcsOverride for
// the precedence). A jj opt-in applies only when the marker root actually
// has a .jj directory; otherwise it silently falls back to git. The opt-in
// is read at the path's marker root and, for a .jj-only tree (such as a
// pooled jj workspace, whose checkout cannot carry an untracked
// treehouse.toml), also at the main repository root that its .jj/repo
// pointer names. Backend choice always comes from that explicit
// configuration, never from the marker itself; paths outside any repository
// fall back to git so errors surface exactly as they always did.
func backendFor(path string) Backend {
	dir := path
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return gitBackend
		}
		dir = cwd
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	root, hasJJ, hasGit := findMarkerRoot(dir)
	if root == "" {
		return gitBackend
	}
	switch vcsOverride(root) {
	case "git":
		return gitBackend
	case "jj":
		if hasJJ {
			return jjBackend
		}
		return gitBackend
	}
	if hasJJ && !hasGit {
		// A workspace's own tree holds no untracked config; the opt-in, if
		// any, lives at the main repository root the .jj/repo pointer names.
		if mainRoot, err := jjvcs.MainRootFromWorkspaceRoot(root); err == nil && mainRoot != root {
			if vcsOverride(mainRoot) == "jj" {
				return jjBackend
			}
		}
	}
	return gitBackend
}

// findMarkerRoot walks up from dir and stops at the first level holding a VCS
// marker, reporting which markers exist there so the caller can tell a
// colocated repository (.jj and .git together) from a jj-only one.
func findMarkerRoot(dir string) (root string, hasJJ, hasGit bool) {
	for {
		if info, err := os.Stat(filepath.Join(dir, ".jj")); err == nil && info.IsDir() {
			hasJJ = true
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			hasGit = true
		}
		if hasJJ || hasGit {
			return dir, hasJJ, hasGit
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, false
		}
		dir = parent
	}
}

// vcsOverride returns a forced backend name ("git" or "jj") for repoRoot, or
// "" when selection should stay automatic. Precedence, highest first: the
// TREEHOUSE_VCS environment variable, the "vcs" key of the repository's
// treehouse.toml, the "vcs" key of the user-level
// ~/.config/treehouse/config.toml. The files are read directly here (rather
// than through internal/config) because config depends on this package.
func vcsOverride(repoRoot string) string {
	if v := normalizeVCSName(os.Getenv("TREEHOUSE_VCS")); v != "" {
		return v
	}
	if v := vcsFromFile(filepath.Join(repoRoot, "treehouse.toml")); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil {
		return vcsFromFile(filepath.Join(home, ".config", "treehouse", "config.toml"))
	}
	return ""
}

func vcsFromFile(path string) string {
	var cfg struct {
		VCS string `toml:"vcs"`
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return ""
	}
	return normalizeVCSName(cfg.VCS)
}

func normalizeVCSName(v string) string {
	switch v {
	case "git", "jj":
		return v
	}
	return ""
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
	return backendForRemoval(repoRoot, path).RemoveWorktree(repoRoot, path)
}

// RemoveCleanWorktree removes a worktree, refusing if it is not clean.
func RemoveCleanWorktree(repoRoot, path string) error {
	return backendForRemoval(repoRoot, path).RemoveCleanWorktree(repoRoot, path)
}

// backendForRemoval dispatches removal on what the worktree actually is (its
// own marker), not on the repository's configured backend. A pool can
// legitimately hold slots of both flavors after an opt-in change, and routing
// a git worktree through jj removal deletes its directory without
// deregistering it from .git/worktrees (the reverse direction errors and
// leaves the slot stranded). This is artifact-typed dispatch, not backend
// selection: creating worktrees still follows the explicit opt-in. A missing
// or empty path falls back to the repository's backend so error surfacing is
// unchanged.
func backendForRemoval(repoRoot, path string) Backend {
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return gitBackend
	}
	if info, err := os.Stat(filepath.Join(path, ".jj")); err == nil && info.IsDir() {
		return jjBackend
	}
	return backendFor(repoRoot)
}

// Fetch updates refs from origin when an origin remote exists.
func Fetch(repoRoot string) error { return backendFor(repoRoot).Fetch(repoRoot) }

// ResetWorktree returns a worktree to a pristine checkout of branch.
func ResetWorktree(worktreePath, branch string) error {
	return backendFor(worktreePath).ResetWorktree(worktreePath, branch)
}

// ResetWorktreeToRef resets worktreePath to an already resolved commit.
func ResetWorktreeToRef(worktreePath, ref, expectedHead string, requireClean bool) error {
	return backendFor(worktreePath).ResetWorktreeToRef(worktreePath, ref, expectedHead, requireClean)
}

// IsWorktreeSafeToReset reports whether worktreePath can be reset to branch
// without discarding committed work and returns the immutable reset target and
// the worktree HEAD recorded at check time.
func IsWorktreeSafeToReset(worktreePath, branch string) (bool, string, string, error) {
	return backendFor(worktreePath).IsWorktreeSafeToReset(worktreePath, branch)
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
