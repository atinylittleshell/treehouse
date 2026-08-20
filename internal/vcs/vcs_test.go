package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
	dir := t.TempDir()
	mustRun(t, dir, "jj", "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "treehouse.toml"), []byte("vcs = \"jj\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := backendFor(dir).Name(); got != "jj" {
		t.Fatalf("expected jj after explicit opt-in, got %q", got)
	}
}

func TestBackendForWorkspaceInheritsMainRootOptIn(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
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
	if err := os.WriteFile(filepath.Join(repoDir, "treehouse.toml"), []byte("vcs = \"jj\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := backendFor(wsDir).Name(); got != "jj" {
		t.Fatalf("expected workspace to inherit the main root opt-in, got %q", got)
	}
}

func TestBackendForColocatedRepoDefaultsToGit(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "jj", "git", "init", "--colocate")
	if got := backendFor(dir).Name(); got != "git" {
		t.Fatalf("expected git backend for colocated repo without opt-in, got %q", got)
	}
}

func TestBackendForOutsideAnyRepoFallsBackToGit(t *testing.T) {
	if got := backendFor(t.TempDir()).Name(); got != "git" {
		t.Fatalf("expected git fallback outside repositories, got %q", got)
	}
}

func TestBackendForEnvOverride(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
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
	dir := t.TempDir()
	mustRun(t, dir, "jj", "git", "init", "--colocate")
	if err := os.WriteFile(filepath.Join(dir, "treehouse.toml"), []byte("vcs = \"jj\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := backendFor(dir).Name(); got != "jj" {
		t.Fatalf("expected vcs = jj in treehouse.toml to opt the colocated repo in to jj, got %q", got)
	}
}

func TestBackendForNestedPathFindsRootOptIn(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	isolateJJConfig(t)
	dir := t.TempDir()
	mustRun(t, dir, "jj", "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "treehouse.toml"), []byte("vcs = \"jj\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := backendFor(nested).Name(); got != "jj" {
		t.Fatalf("expected nested path to find the marker root's opt-in, got %q", got)
	}
}
