package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// isolateUserConfig points the user home at a fresh temp directory so backend
// selection never sees the developer's real ~/.config/treehouse/config.toml,
// and clears any ambient TREEHOUSE_VCS exported by the developer's shell.
// It returns the new home so tests can plant a user-level config there.
func isolateUserConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	t.Setenv("TREEHOUSE_VCS", "")
	return home
}

// writeVCSKey writes a config file at path containing only a vcs key.
func writeVCSKey(t *testing.T, path, vcs string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("vcs = \""+vcs+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func userConfigPath(home string) string {
	return filepath.Join(home, ".config", "treehouse", "config.toml")
}

// fakeJJOnlyRepo lays down a bare .jj marker directory. Backend selection is
// pure file inspection, so these tests need no jj binary.
func fakeJJOnlyRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".jj", "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// isolateJJConfig points JJ_CONFIG at a minimal config so tests are hermetic
// against the developer's own jj configuration (e.g. git.colocate = true
// would silently colocate every repo these tests create).
func isolateJJConfig(t *testing.T) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "jjconfig.toml")
	contents := "[user]\nname = \"Treehouse Tests\"\nemail = \"treehouse-tests@example.com\"\n\n[git]\n# jj colocates new repos by default; tests opt out so \"jj-only\" fixtures\n# really are jj-only, and colocated fixtures say --colocate explicitly.\ncolocate = false\n"
	if err := os.WriteFile(cfg, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JJ_CONFIG", cfg)
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func TestBackendForGitRepo(t *testing.T) {
	isolateUserConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "git", "init")
	if got := backendFor(dir).Name(); got != "git" {
		t.Fatalf("expected git backend, got %q", got)
	}
}

func TestBackendForJJOnlyRepoDefaultsToGitWithoutOptIn(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
	isolateUserConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "jj", "git", "init")
	if got := backendFor(dir).Name(); got != "git" {
		t.Fatalf("expected git default for a .jj-only repo without opt-in, got %q", got)
	}
}

func TestBackendForJJOnlyRepoWithOptIn(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
	isolateUserConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "jj", "git", "init")
	writeVCSKey(t, filepath.Join(dir, "treehouse.toml"), "jj")
	if got := backendFor(dir).Name(); got != "jj" {
		t.Fatalf("expected jj after explicit opt-in, got %q", got)
	}
}

func TestBackendForWorkspaceInheritsMainRootOptIn(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
	isolateUserConfig(t)
	base := t.TempDir()
	repoDir := filepath.Join(base, "repo")
	wsDir := filepath.Join(base, "ws")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRun(t, repoDir, "jj", "git", "init")
	mustRun(t, repoDir, "jj", "workspace", "add", "--name", "pool", wsDir)

	// Without the main root's opt-in, the workspace stays on the git default.
	if got := backendFor(wsDir).Name(); got != "git" {
		t.Fatalf("expected git default for a workspace without opt-in, got %q", got)
	}

	// The opt-in lives at the main repository root; the workspace cannot
	// carry an untracked treehouse.toml, so it must inherit through the
	// .jj/repo pointer.
	writeVCSKey(t, filepath.Join(repoDir, "treehouse.toml"), "jj")
	if got := backendFor(wsDir).Name(); got != "jj" {
		t.Fatalf("expected workspace to inherit the main root opt-in, got %q", got)
	}
}

func TestBackendForColocatedRepoDefaultsToGit(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
	isolateUserConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "jj", "git", "init", "--colocate")
	if got := backendFor(dir).Name(); got != "git" {
		t.Fatalf("expected git backend for colocated repo without opt-in, got %q", got)
	}
}

func TestBackendForOutsideAnyRepoFallsBackToGit(t *testing.T) {
	isolateUserConfig(t)
	if got := backendFor(t.TempDir()).Name(); got != "git" {
		t.Fatalf("expected git fallback outside repositories, got %q", got)
	}
}

func TestBackendForEnvOverride(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
	isolateUserConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "jj", "git", "init", "--colocate")

	t.Setenv("TREEHOUSE_VCS", "jj")
	if got := backendFor(dir).Name(); got != "jj" {
		t.Fatalf("expected TREEHOUSE_VCS=jj to opt the colocated repo in to jj, got %q", got)
	}
}

func TestBackendForConfigOverride(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
	isolateUserConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "jj", "git", "init", "--colocate")
	writeVCSKey(t, filepath.Join(dir, "treehouse.toml"), "jj")
	if got := backendFor(dir).Name(); got != "jj" {
		t.Fatalf("expected vcs = jj in treehouse.toml to opt the colocated repo in to jj, got %q", got)
	}
}

func TestBackendForNestedPathFindsRootOptIn(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
	isolateUserConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "jj", "git", "init")
	writeVCSKey(t, filepath.Join(dir, "treehouse.toml"), "jj")
	nested := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := backendFor(nested).Name(); got != "jj" {
		t.Fatalf("expected nested path to find the marker root's opt-in, got %q", got)
	}
}

func TestBackendForEnvOptInIgnoredInPlainGitRepo(t *testing.T) {
	isolateUserConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "git", "init")

	t.Setenv("TREEHOUSE_VCS", "jj")
	if got := backendFor(dir).Name(); got != "git" {
		t.Fatalf("expected TREEHOUSE_VCS=jj to be ignored in a repo without .jj, got %q", got)
	}
}

func TestBackendForConfigOptInIgnoredInPlainGitRepo(t *testing.T) {
	isolateUserConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "git", "init")

	writeVCSKey(t, filepath.Join(dir, "treehouse.toml"), "jj")
	if got := backendFor(dir).Name(); got != "git" {
		t.Fatalf("expected vcs = jj to be ignored in a repo without .jj, got %q", got)
	}
}

func TestBackendForUserConfigOptIn(t *testing.T) {
	home := isolateUserConfig(t)
	dir := fakeJJOnlyRepo(t)

	if got := backendFor(dir).Name(); got != "git" {
		t.Fatalf("expected git default before the user-level opt-in, got %q", got)
	}
	writeVCSKey(t, userConfigPath(home), "jj")
	if got := backendFor(dir).Name(); got != "jj" {
		t.Fatalf("expected user-level vcs = jj to opt the jj repo in, got %q", got)
	}
}

func TestBackendForUserConfigOptInIgnoredInPlainGitRepo(t *testing.T) {
	home := isolateUserConfig(t)
	writeVCSKey(t, userConfigPath(home), "jj")

	dir := t.TempDir()
	mustRun(t, dir, "git", "init")
	if got := backendFor(dir).Name(); got != "git" {
		t.Fatalf("expected user-level vcs = jj to be ignored in a repo without .jj, got %q", got)
	}
}

func TestBackendForRepoConfigBeatsUserConfig(t *testing.T) {
	home := isolateUserConfig(t)
	writeVCSKey(t, userConfigPath(home), "jj")

	dir := fakeJJOnlyRepo(t)
	writeVCSKey(t, filepath.Join(dir, "treehouse.toml"), "git")
	if got := backendFor(dir).Name(); got != "git" {
		t.Fatalf("expected repo-level vcs = git to beat the user-level opt-in, got %q", got)
	}
}

func TestBackendForEnvBeatsRepoConfig(t *testing.T) {
	isolateUserConfig(t)
	dir := fakeJJOnlyRepo(t)
	writeVCSKey(t, filepath.Join(dir, "treehouse.toml"), "git")

	t.Setenv("TREEHOUSE_VCS", "jj")
	if got := backendFor(dir).Name(); got != "jj" {
		t.Fatalf("expected TREEHOUSE_VCS=jj to beat the repo-level vcs = git, got %q", got)
	}
}
