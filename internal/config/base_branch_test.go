package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_BaseBranchDefaultsToEmpty(t *testing.T) {
	setUserHome(t, t.TempDir())

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.BaseBranch != "" {
		t.Errorf("BaseBranch: got %q, want empty", cfg.BaseBranch)
	}
}

func TestLoad_BaseBranchFromRepoConfig(t *testing.T) {
	repoDir := t.TempDir()
	setUserHome(t, t.TempDir())

	cfgTOML := "max_trees = 4\nbase_branch = \"develop\"\n"
	if err := os.WriteFile(filepath.Join(repoDir, "treehouse.toml"), []byte(cfgTOML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.BaseBranch != "develop" {
		t.Errorf("BaseBranch: got %q, want develop", cfg.BaseBranch)
	}
}

func TestLoad_BaseBranchFromUserConfig(t *testing.T) {
	repoDir := t.TempDir()
	userHome := t.TempDir()
	setUserHome(t, userHome)

	configDir := filepath.Join(userHome, ".config", "treehouse")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("base_branch = \"develop\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.BaseBranch != "develop" {
		t.Errorf("BaseBranch: got %q, want develop", cfg.BaseBranch)
	}
}

func TestLoad_RepoBaseBranchOverridesUserConfig(t *testing.T) {
	repoDir := t.TempDir()
	userHome := t.TempDir()
	setUserHome(t, userHome)

	configDir := filepath.Join(userHome, ".config", "treehouse")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("base_branch = \"user-level\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "treehouse.toml"), []byte("base_branch = \"repo-level\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.BaseBranch != "repo-level" {
		t.Errorf("BaseBranch: got %q, want repo-level", cfg.BaseBranch)
	}
}
