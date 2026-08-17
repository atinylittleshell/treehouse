package git

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func FindRepoRoot() (string, error) {
	return runGit("", "rev-parse", "--show-toplevel")
}

func FindRepoRootFrom(dir string) (string, error) {
	return runGit(dir, "rev-parse", "--show-toplevel")
}

// FindMainRepoRootFrom returns the main repository root for dir.
// For linked worktrees, it resolves the worktree root back to the owning
// repository.
func FindMainRepoRootFrom(dir string) (string, error) {
	repoRoot, err := FindRepoRootFrom(dir)
	if err != nil {
		return "", err
	}
	return mainRepoRoot(repoRoot), nil
}

func GetDefaultBranch(repoRoot string) (string, error) {
	mainRoot := mainRepoRoot(repoRoot)

	// Try remote HEAD first (most reliable when remote exists).
	if HasRemote(mainRoot, "origin") {
		if out, err := runGit(mainRoot, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
			if branch, ok := strings.CutPrefix(out, "refs/remotes/origin/"); ok && branch != "" {
				return branch, nil
			}
		}
	}

	return getLocalDefaultBranch(mainRoot)
}

func mainRepoRoot(repoRoot string) string {
	mainRoot := repoRoot
	if dir, err := runGit(repoRoot, "rev-parse", "--git-common-dir"); err == nil {
		if d, err2 := runGit(repoRoot, "rev-parse", "--path-format=absolute", "--git-common-dir"); err2 == nil {
			dir = d
		}
		if root, ok := repoRootFromCommonGitDir(dir); ok {
			mainRoot = root
		}
	}
	return mainRoot
}

func repoRootFromCommonGitDir(dir string) (string, bool) {
	cleaned := filepath.Clean(filepath.FromSlash(dir))
	if filepath.Base(cleaned) != ".git" {
		return "", false
	}
	return filepath.Dir(cleaned), true
}

func getLocalDefaultBranch(mainRoot string) (string, error) {
	if out, err := runGit(mainRoot, "symbolic-ref", "HEAD"); err == nil {
		if branch, ok := strings.CutPrefix(out, "refs/heads/"); ok && branch != "" {
			return branch, nil
		}
	}

	if out, err := runGit(mainRoot, "config", "init.defaultBranch"); err == nil && out != "" {
		return out, nil
	}

	return "", fmt.Errorf("cannot determine default branch: try running 'git fetch' or ensure you are on a branch")
}

func HasRemote(repoRoot, name string) bool {
	out, err := runGit(repoRoot, "remote")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

func GetRemoteURL(repoRoot string) (string, error) {
	return runGit(repoRoot, "remote", "get-url", "origin")
}

func refExists(repoRoot, ref string) bool {
	_, err := runGit(repoRoot, "rev-parse", "--verify", ref)
	return err == nil
}

// branchRef returns whichever of the local branch or remote-tracking branch is
// further ahead. If they have diverged (neither is an ancestor of the other),
// it prefers origin. Falls back to whichever ref exists.
func branchRef(repoRoot, branch string) string {
	local := "refs/heads/" + branch
	remote := remoteTrackingRef("origin", branch)
	hasLocal := refExists(repoRoot, local)
	hasRemote := refExists(repoRoot, remote)

	switch {
	case hasLocal && hasRemote:
		// If local is ancestor of remote, remote is ahead (or equal).
		if isAncestor(repoRoot, local, remote) {
			return remote
		}
		// Otherwise local is ahead or they diverged; prefer local when
		// it's strictly ahead, prefer remote on divergence.
		if isAncestor(repoRoot, remote, local) {
			return branch
		}
		return remote
	case hasLocal:
		return branch
	default:
		return remote
	}
}

func remoteTrackingRef(remote, branch string) string {
	return "refs/remotes/" + remote + "/" + branch
}

// isAncestor returns true if ref a is an ancestor of (or equal to) ref b.
func isAncestor(repoRoot, a, b string) bool {
	_, err := runGit(repoRoot, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

func AddWorktree(repoRoot, path, branch string) error {
	_, err := runGit(repoRoot, "worktree", "add", "--detach", path, branchRef(repoRoot, branch))
	return err
}

func RemoveWorktree(repoRoot, path string) error {
	_, err := runGit(repoRoot, "worktree", "remove", "--force", path)
	return err
}

// RemoveCleanWorktree removes a clean git worktree without forcing deletion.
func RemoveCleanWorktree(repoRoot, path string) error {
	_, err := runGit(repoRoot, "worktree", "remove", path)
	return err
}

func Fetch(repoRoot string) error {
	if !HasRemote(repoRoot, "origin") {
		return nil
	}
	_, err := runGit(repoRoot, "fetch", "origin")
	return err
}

func ResetWorktree(worktreePath, branch string) error {
	repoRoot, err := runGit(worktreePath, "rev-parse", "--show-toplevel")
	if err != nil {
		repoRoot = worktreePath
	}
	ref := branchRef(repoRoot, branch)
	if _, err := runGit(worktreePath, "checkout", "--detach", "--force", ref); err != nil {
		return err
	}
	if _, err := runGit(worktreePath, "reset", "--hard", ref); err != nil {
		return err
	}
	_, err = runGit(worktreePath, "clean", "-fd")
	return err
}

func DetachWorktree(worktreePath string) error {
	_, err := runGit(worktreePath, "checkout", "--detach")
	return err
}

type SafeReturnIdentity string

func ValidateSafeReturnState(worktreePath, expectedHead, expectedRef string) error {
	_, err := ValidateSafeReturnStateWithIdentity(worktreePath, expectedHead, expectedRef)
	return err
}

func ValidateSafeReturnStateWithIdentity(
	worktreePath, expectedHead, expectedRef string,
) (SafeReturnIdentity, error) {
	if err := validateCommitOID(expectedHead); err != nil {
		return "", err
	}
	if err := validateSafeReturnRef(expectedRef); err != nil {
		return "", err
	}

	repository, err := openSafeGitRepository(worktreePath)
	if err != nil {
		return "", err
	}
	head, err := repository.run("rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", err
	}
	if head != expectedHead {
		return "", fmt.Errorf("worktree HEAD changed: expected %s, got %s", expectedHead, head)
	}

	if _, symbolic, err := repository.symbolicRef(expectedRef); err != nil {
		return "", err
	} else if symbolic {
		return "", fmt.Errorf("safe return ref must not be symbolic: %s", expectedRef)
	}
	refHead, err := repository.run("rev-parse", "--verify", expectedRef+"^{commit}")
	if err != nil {
		return "", err
	}
	if refHead != expectedHead {
		return "", fmt.Errorf("ref %s changed: expected %s, got %s", expectedRef, expectedHead, refHead)
	}

	symbolicHead, attached, err := repository.symbolicRef("HEAD")
	if err != nil {
		return "", err
	}
	switch {
	case strings.HasPrefix(expectedRef, "refs/heads/") && attached && symbolicHead != expectedRef:
		return "", fmt.Errorf("worktree HEAD is not attached to %s", expectedRef)
	case strings.HasPrefix(expectedRef, "refs/remotes/origin/") && attached:
		return "", fmt.Errorf("worktree HEAD must be detached for remote ref %s", expectedRef)
	}

	identity, err := validateSafeRepository(repository)
	if err != nil {
		return "", err
	}
	return identity, nil
}

func validateSafeRepositoryState(worktreePath string) error {
	root, err := openSafeGitRepository(worktreePath)
	if err != nil {
		return err
	}
	_, err = validateSafeRepository(root)
	return err
}

func validateSafeRepository(root safeGitRepository) (SafeReturnIdentity, error) {
	repositories := []safeGitRepository{root}
	submodules, err := initializedSubmoduleRepositories(root)
	if err != nil {
		return "", err
	}
	repositories = append(repositories, submodules...)
	identityParts := make([]string, 0, len(repositories)*2)
	for _, repository := range repositories {
		identityParts = append(identityParts, repository.worktreePath, repository.gitDir)
		label := "worktree"
		if repository.worktreePath != root.worktreePath {
			relative, err := filepath.Rel(root.worktreePath, repository.worktreePath)
			if err != nil {
				return "", err
			}
			label = "submodule " + relative
		}
		operation, err := operationInProgress(repository)
		if err != nil {
			return "", err
		}
		if operation != "" {
			return "", fmt.Errorf("%s has %s in progress", label, operation)
		}
		unsafeFlags, err := hasUnsafeIndexFlags(repository)
		if err != nil {
			return "", err
		}
		if unsafeFlags {
			return "", fmt.Errorf("%s has assume-unchanged or skip-worktree index flags", label)
		}
		dirty, err := repository.isDirty()
		if err != nil {
			return "", err
		}
		if dirty {
			return "", fmt.Errorf("%s has uncommitted changes", label)
		}
	}
	return SafeReturnIdentity(strings.Join(identityParts, "\x00")), nil
}

func OperationInProgress(worktreePath string) (string, error) {
	repository, err := openSafeGitRepository(worktreePath)
	if err != nil {
		return "", err
	}
	return operationInProgress(repository)
}

func operationInProgress(repository safeGitRepository) (string, error) {
	operations := []struct {
		path string
		name string
	}{
		{path: "MERGE_HEAD", name: "merge"},
		{path: "rebase-apply", name: "rebase"},
		{path: "rebase-merge", name: "rebase"},
		{path: "CHERRY_PICK_HEAD", name: "cherry-pick"},
		{path: "REVERT_HEAD", name: "revert"},
		{path: "BISECT_LOG", name: "bisect"},
		{path: "sequencer", name: "sequencer"},
	}
	for _, operation := range operations {
		path, err := repository.run("rev-parse", "--git-path", operation.path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(repository.worktreePath, path)
		}
		if _, err := os.Stat(path); err == nil {
			return operation.name, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", nil
}

func validateCommitOID(value string) error {
	if len(value) != 40 && len(value) != 64 {
		return fmt.Errorf("expected HEAD must be a full commit object ID")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("expected HEAD must be a full commit object ID")
	}
	return nil
}

func validateSafeReturnRef(ref string) error {
	if !strings.HasPrefix(ref, "refs/heads/") && !strings.HasPrefix(ref, "refs/remotes/origin/") {
		return fmt.Errorf("safe return ref must be under refs/heads/ or refs/remotes/origin/")
	}
	if ref == "refs/heads/" || ref == "refs/remotes/origin/" {
		return fmt.Errorf("safe return ref must name a branch")
	}
	if _, err := runGit("", "check-ref-format", ref); err != nil {
		return fmt.Errorf("invalid safe return ref %q", ref)
	}
	return nil
}

func (repository safeGitRepository) symbolicRef(ref string) (string, bool, error) {
	cmd := repository.command("symbolic-ref", "-q", ref)
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return "", false, nil
	}
	return "", false, err
}

// DefaultBranchMergeRef returns the fully qualified ref used for merge safety checks.
// Repositories with origin use the current remote default tracking ref and fail
// closed if that local tracking ref does not match remote HEAD; local-only
// repositories use the local default branch ref.
func DefaultBranchMergeRef(repoRoot string) (string, error) {
	if HasRemote(repoRoot, "origin") {
		branch, sha, err := remoteDefaultBranch(repoRoot, "origin")
		if err != nil {
			return "", err
		}
		ref := remoteTrackingRef("origin", branch)
		localSHA, err := refCommit(repoRoot, ref)
		if err != nil {
			return "", fmt.Errorf("%s is unavailable", ref)
		}
		if localSHA != sha {
			return "", fmt.Errorf("%s is stale: expected %s, got %s", ref, sha, localSHA)
		}
		return ref, nil
	}

	branch, err := GetDefaultBranch(repoRoot)
	if err != nil {
		return "", err
	}
	ref := "refs/heads/" + branch
	if _, err := refCommit(repoRoot, ref); err != nil {
		return "", fmt.Errorf("%s is unavailable", ref)
	}
	return ref, nil
}

func refCommit(repoRoot, ref string) (string, error) {
	return runGit(repoRoot, "rev-parse", "--verify", ref+"^{commit}")
}

func remoteDefaultBranch(repoRoot, remote string) (string, string, error) {
	out, err := runGit(repoRoot, "ls-remote", "--symref", remote, "HEAD")
	if err != nil {
		return "", "", err
	}
	var branch string
	var sha string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "ref:" && fields[2] == "HEAD" {
			if value, ok := strings.CutPrefix(fields[1], "refs/heads/"); ok {
				branch = value
			}
			continue
		}
		if len(fields) == 2 && fields[1] == "HEAD" {
			sha = fields[0]
		}
	}
	if branch == "" {
		return "", "", fmt.Errorf("cannot determine %s default branch", remote)
	}
	if sha == "" {
		return "", "", fmt.Errorf("cannot determine %s default branch commit", remote)
	}
	return branch, sha, nil
}

// IsHeadMergedIntoDefault reports whether HEAD is merged into DefaultBranchMergeRef.
func IsHeadMergedIntoDefault(repoRoot, worktreePath string) (bool, string, error) {
	ref, err := DefaultBranchMergeRef(repoRoot)
	if err != nil {
		return false, "", err
	}

	merged, err := IsHeadMergedIntoRef(worktreePath, ref)
	return merged, ref, err
}

// IsHeadMergedIntoRef reports whether worktreePath's HEAD is an ancestor of ref.
func IsHeadMergedIntoRef(worktreePath, ref string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", "HEAD", ref)
	cmd.Dir = worktreePath
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor HEAD %s: %s", ref, strings.TrimSpace(string(out)))
}

// IsDirty reports tracked or untracked changes, ignoring status.showUntrackedFiles.
func IsDirty(worktreePath string) (bool, error) {
	out, err := runGit(worktreePath, "status", "--porcelain", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func hasUnsafeIndexFlags(repository safeGitRepository) (bool, error) {
	cmd := repository.command("ls-files", "-v", "-z")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return false, err
	}
	reader := bufio.NewReader(stdout)
	for {
		record, readErr := reader.ReadString(0)
		if record != "" {
			flag := record[0]
			if flag == 'S' || (flag >= 'a' && flag <= 'z') {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return true, nil
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return false, readErr
		}
	}
	if err := cmd.Wait(); err != nil {
		return false, fmt.Errorf("git ls-files -v -z: %s", strings.TrimSpace(stderr.String()))
	}
	return false, nil
}

func initializedSubmoduleRepositories(repository safeGitRepository) ([]safeGitRepository, error) {
	out, err := repository.runRaw("ls-files", "--stage", "-z")
	if err != nil {
		return nil, err
	}
	var result []safeGitRepository
	for _, rawRecord := range bytes.Split(out, []byte{0}) {
		if len(rawRecord) == 0 {
			continue
		}
		metadata, rawPath, ok := bytes.Cut(rawRecord, []byte{'\t'})
		if !ok || !strings.HasPrefix(string(metadata), "160000 ") {
			continue
		}
		relative := filepath.Clean(filepath.FromSlash(string(rawPath)))
		if relative == "." || filepath.IsAbs(relative) ||
			relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("invalid submodule path %q", rawPath)
		}
		path := filepath.Join(repository.worktreePath, relative)
		if _, err := os.Stat(filepath.Join(path, ".git")); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		submodule, err := openSafeGitRepository(path)
		if err != nil {
			return nil, err
		}
		result = append(result, submodule)
		nested, err := initializedSubmoduleRepositories(submodule)
		if err != nil {
			return nil, err
		}
		result = append(result, nested...)
	}
	return result, nil
}

func ShortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:3])
}

func runGit(dir string, args ...string) (string, error) {
	out, err := runGitRaw(dir, args...)
	return strings.TrimSpace(string(out)), err
}

func runGitRaw(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

type safeGitRepository struct {
	worktreePath string
	gitDir       string
}

func openSafeGitRepository(worktreePath string) (safeGitRepository, error) {
	canonicalWorktree, err := canonicalPath(worktreePath)
	if err != nil {
		return safeGitRepository{}, err
	}
	worktreeInfo, err := os.Stat(canonicalWorktree)
	if err != nil {
		return safeGitRepository{}, err
	}
	if !worktreeInfo.IsDir() {
		return safeGitRepository{}, fmt.Errorf("safe Git worktree is not a directory: %s", canonicalWorktree)
	}

	marker := filepath.Join(canonicalWorktree, ".git")
	markerInfo, err := os.Lstat(marker)
	if err != nil {
		return safeGitRepository{}, fmt.Errorf("inspect safe Git marker: %w", err)
	}
	var gitDir string
	switch {
	case markerInfo.IsDir():
		gitDir = marker
	case markerInfo.Mode().IsRegular():
		if markerInfo.Size() > 4096 {
			return safeGitRepository{}, fmt.Errorf("safe Git marker is too large")
		}
		content, err := os.ReadFile(marker)
		if err != nil {
			return safeGitRepository{}, err
		}
		value := strings.TrimSpace(string(content))
		if strings.ContainsAny(value, "\r\n") {
			return safeGitRepository{}, fmt.Errorf("safe Git marker has invalid content")
		}
		value, ok := strings.CutPrefix(value, "gitdir: ")
		if !ok || value == "" {
			return safeGitRepository{}, fmt.Errorf("safe Git marker has invalid content")
		}
		if filepath.IsAbs(value) {
			gitDir = value
		} else {
			gitDir = filepath.Join(canonicalWorktree, value)
		}
	default:
		return safeGitRepository{}, fmt.Errorf("safe Git marker must be a directory or regular file")
	}
	canonicalGitDir, err := canonicalPath(gitDir)
	if err != nil {
		return safeGitRepository{}, err
	}
	gitDirInfo, err := os.Stat(canonicalGitDir)
	if err != nil {
		return safeGitRepository{}, err
	}
	if !gitDirInfo.IsDir() {
		return safeGitRepository{}, fmt.Errorf("safe Git directory is not a directory: %s", canonicalGitDir)
	}

	repository := safeGitRepository{
		worktreePath: canonicalWorktree,
		gitDir:       canonicalGitDir,
	}
	if markerInfo.Mode().IsRegular() {
		if err := repository.validateExternalGitDirOwner(marker); err != nil {
			return safeGitRepository{}, err
		}
	}
	topLevel, err := repository.run("rev-parse", "--show-toplevel")
	if err != nil {
		return safeGitRepository{}, err
	}
	canonicalTopLevel, err := canonicalPath(topLevel)
	if err != nil {
		return safeGitRepository{}, err
	}
	if canonicalTopLevel != canonicalWorktree {
		return safeGitRepository{}, fmt.Errorf(
			"safe Git worktree root mismatch: expected %s, got %s",
			canonicalWorktree,
			canonicalTopLevel,
		)
	}
	absoluteGitDir, err := repository.run("rev-parse", "--absolute-git-dir")
	if err != nil {
		return safeGitRepository{}, err
	}
	canonicalReportedGitDir, err := canonicalPath(absoluteGitDir)
	if err != nil {
		return safeGitRepository{}, err
	}
	if canonicalReportedGitDir != canonicalGitDir {
		return safeGitRepository{}, fmt.Errorf(
			"safe Git directory mismatch: expected %s, got %s",
			canonicalGitDir,
			canonicalReportedGitDir,
		)
	}
	return repository, nil
}

func (repository safeGitRepository) validateExternalGitDirOwner(marker string) error {
	backlink := filepath.Join(repository.gitDir, "gitdir")
	backlinkInfo, err := os.Lstat(backlink)
	switch {
	case err == nil:
		if !backlinkInfo.Mode().IsRegular() || backlinkInfo.Size() > 4096 {
			return fmt.Errorf("safe Git backlink must be a small regular file")
		}
		content, err := os.ReadFile(backlink)
		if err != nil {
			return err
		}
		value := strings.TrimSpace(string(content))
		if value == "" || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("safe Git backlink has invalid content")
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(repository.gitDir, value)
		}
		canonicalBacklink, err := canonicalPath(value)
		if err != nil {
			return err
		}
		canonicalMarker, err := canonicalPath(marker)
		if err != nil {
			return err
		}
		if canonicalBacklink != canonicalMarker {
			return fmt.Errorf(
				"safe Git backlink mismatch: expected %s, got %s",
				canonicalMarker,
				canonicalBacklink,
			)
		}
		return nil
	case !os.IsNotExist(err):
		return err
	}

	cmd := repository.command("config", "--local", "--no-includes", "--get", "core.worktree")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("safe external Git directory has no worktree owner")
	}
	value := strings.TrimSpace(string(out))
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("safe Git core.worktree has invalid content")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(repository.gitDir, value)
	}
	canonicalOwner, err := canonicalPath(value)
	if err != nil {
		return err
	}
	if canonicalOwner != repository.worktreePath {
		return fmt.Errorf(
			"safe Git worktree owner mismatch: expected %s, got %s",
			repository.worktreePath,
			canonicalOwner,
		)
	}
	return nil
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

func (repository safeGitRepository) command(args ...string) *exec.Cmd {
	commandArgs := []string{
		"--git-dir=" + repository.gitDir,
		"--work-tree=" + repository.worktreePath,
	}
	commandArgs = append(commandArgs, args...)
	cmd := exec.Command("git", commandArgs...)
	cmd.Dir = repository.worktreePath
	cmd.Env = sanitizeGitEnvironment(os.Environ())
	return cmd
}

func (repository safeGitRepository) run(args ...string) (string, error) {
	out, err := repository.runRaw(args...)
	return strings.TrimSpace(string(out)), err
}

func (repository safeGitRepository) runRaw(args ...string) ([]byte, error) {
	out, err := repository.command(args...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

func (repository safeGitRepository) isDirty() (bool, error) {
	out, err := repository.run("status", "--porcelain", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func sanitizeGitEnvironment(environment []string) []string {
	blocked := map[string]bool{
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
		"GIT_CEILING_DIRECTORIES":          true,
		"GIT_COMMON_DIR":                   true,
		"GIT_CONFIG":                       true,
		"GIT_CONFIG_COUNT":                 true,
		"GIT_CONFIG_GLOBAL":                true,
		"GIT_CONFIG_NOSYSTEM":              true,
		"GIT_CONFIG_PARAMETERS":            true,
		"GIT_CONFIG_SYSTEM":                true,
		"GIT_DIR":                          true,
		"GIT_DISCOVERY_ACROSS_FILESYSTEM":  true,
		"GIT_INDEX_FILE":                   true,
		"GIT_NAMESPACE":                    true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_QUARANTINE_PATH":              true,
		"GIT_REPLACE_REF_BASE":             true,
		"GIT_SHALLOW_FILE":                 true,
		"GIT_WORK_TREE":                    true,
	}
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		upperName := strings.ToUpper(name)
		if blocked[upperName] ||
			strings.HasPrefix(upperName, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(upperName, "GIT_CONFIG_VALUE_") {
			continue
		}
		result = append(result, entry)
	}
	return result
}
