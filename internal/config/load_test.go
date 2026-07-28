package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FindsNearestAncestorConfig(t *testing.T) {
	home := t.TempDir()
	setUserHome(t, home)

	contextDir := filepath.Join(home, "Workspace", "personal")
	repoDir := filepath.Join(contextDir, "projects", "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeConfig(t, contextDir, "max_trees = 8\n")
	writeConfig(t, filepath.Dir(repoDir), "max_trees = 4\n")

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MaxTrees != 4 {
		t.Fatalf("MaxTrees: got %d, want 4", cfg.MaxTrees)
	}
}

func TestLoad_RepoConfigOverridesAncestorConfig(t *testing.T) {
	home := t.TempDir()
	setUserHome(t, home)

	contextDir := filepath.Join(home, "Workspace", "personal")
	repoDir := filepath.Join(contextDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeConfig(t, contextDir, "max_trees = 8\nroot = \"./\"\n")
	writeConfig(t, repoDir, "max_trees = 2\n")

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MaxTrees != 2 {
		t.Fatalf("MaxTrees: got %d, want 2", cfg.MaxTrees)
	}
	if cfg.Root != "" {
		t.Fatalf("Root: got %q, want empty repo override", cfg.Root)
	}
}

func TestLoad_DoesNotDiscoverConfigAtHome(t *testing.T) {
	home := t.TempDir()
	setUserHome(t, home)

	repoDir := filepath.Join(home, "Workspace", "personal", "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, home, "max_trees = 3\n")

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MaxTrees != DefaultConfig().MaxTrees {
		t.Fatalf("MaxTrees: got %d, want default %d", cfg.MaxTrees, DefaultConfig().MaxTrees)
	}
}

func TestLoad_FindsAncestorConfigOutsideHome(t *testing.T) {
	setUserHome(t, t.TempDir())

	contextDir := t.TempDir()
	repoDir := filepath.Join(contextDir, "projects", "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, contextDir, "max_trees = 6\n")

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MaxTrees != 6 {
		t.Fatalf("MaxTrees: got %d, want 6", cfg.MaxTrees)
	}
}

func TestLoad_ResolvesRelativeRootFromConfigDirectory(t *testing.T) {
	home := t.TempDir()
	setUserHome(t, home)

	contextDir := filepath.Join(home, "Workspace", "personal")
	repoDir := filepath.Join(contextDir, "projects", "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, contextDir, "root = \"./\"\n")

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	poolRoot, err := ResolvePoolRoot(repoDir, cfg.Root)
	if err != nil {
		t.Fatalf("ResolvePoolRoot failed: %v", err)
	}

	want := filepath.Join(contextDir, ".treehouse")
	if poolRoot != want {
		t.Fatalf("pool root: got %q, want %q", poolRoot, want)
	}
}

func TestLoad_MalformedNearestConfigDoesNotFallBack(t *testing.T) {
	home := t.TempDir()
	setUserHome(t, home)

	contextDir := filepath.Join(home, "Workspace", "personal")
	projectDir := filepath.Join(contextDir, "projects")
	repoDir := filepath.Join(projectDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, contextDir, "max_trees = 8\n")
	writeConfig(t, projectDir, "max_trees = [\n")

	if _, err := Load(repoDir); err == nil {
		t.Fatal("expected malformed nearest config to fail")
	}
}

func TestLoad_IgnoresAncestorHooksAndKeepsUserHooks(t *testing.T) {
	home := t.TempDir()
	setUserHome(t, home)

	contextDir := filepath.Join(home, "Workspace", "personal")
	repoDir := filepath.Join(contextDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, contextDir, `
max_trees = 4

[hooks]
post_create = ["echo unsafe"]
`)

	userConfigDir := filepath.Join(home, ".config", "treehouse")
	if err := os.MkdirAll(userConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(userConfigDir, "config.toml"),
		[]byte("[hooks]\npost_create = [\"echo safe\"]\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Hooks.PostCreate) != 1 || cfg.Hooks.PostCreate[0] != "echo safe" {
		t.Fatalf("PostCreate: got %v, want user-level hook", cfg.Hooks.PostCreate)
	}
}

func writeConfig(t *testing.T, dir string, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "treehouse.toml"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
