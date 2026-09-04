package config

import (
	"os"
	"path/filepath"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func TestLoad_UniqueLeafDefaultsToOff(t *testing.T) {
	setUserHome(t, t.TempDir())

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.UniqueLeaf {
		t.Error("UniqueLeaf: got true, want false by default")
	}
}

func TestLoad_UniqueLeafFromRepoConfig(t *testing.T) {
	repoDir := t.TempDir()
	setUserHome(t, t.TempDir())

	cfgTOML := "max_trees = 4\nunique_leaf = true\n"
	if err := os.WriteFile(filepath.Join(repoDir, "treehouse.toml"), []byte(cfgTOML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !cfg.UniqueLeaf {
		t.Error("UniqueLeaf: got false, want true")
	}
}

func TestLoad_UniqueLeafFromUserConfig(t *testing.T) {
	repoDir := t.TempDir()
	userHome := t.TempDir()
	setUserHome(t, userHome)

	configDir := filepath.Join(userHome, ".config", "treehouse")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("unique_leaf = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !cfg.UniqueLeaf {
		t.Error("UniqueLeaf: got false, want true")
	}
}

func TestLoad_RepoUniqueLeafOverridesUserConfig(t *testing.T) {
	repoDir := t.TempDir()
	userHome := t.TempDir()
	setUserHome(t, userHome)

	configDir := filepath.Join(userHome, ".config", "treehouse")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("unique_leaf = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "treehouse.toml"), []byte("unique_leaf = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(repoDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.UniqueLeaf {
		t.Error("UniqueLeaf: got true, want the repo-level false to win")
	}
}

func TestResolveUniqueLeaf_Precedence(t *testing.T) {
	cfg := Config{UniqueLeaf: true}

	t.Run("flag wins over env and config", func(t *testing.T) {
		t.Setenv(UniqueLeafEnvVar, "1")
		if got := ResolveUniqueLeaf(boolPtr(false), cfg); got {
			t.Error("expected the flag to win, got true")
		}
	})

	t.Run("env wins over config when the flag is unset", func(t *testing.T) {
		t.Setenv(UniqueLeafEnvVar, "0")
		if got := ResolveUniqueLeaf(nil, cfg); got {
			t.Error("expected the env var to win, got true")
		}
	})

	t.Run("config used when flag and env are unset", func(t *testing.T) {
		t.Setenv(UniqueLeafEnvVar, "")
		if got := ResolveUniqueLeaf(nil, cfg); !got {
			t.Error("expected the config value, got false")
		}
	})

	t.Run("off by default when nothing is set", func(t *testing.T) {
		t.Setenv(UniqueLeafEnvVar, "")
		if got := ResolveUniqueLeaf(nil, Config{}); got {
			t.Error("expected unique leaves to be off by default, got true")
		}
	})
}

func TestResolveUniqueLeaf_EnvEnablesWithoutConfig(t *testing.T) {
	t.Setenv(UniqueLeafEnvVar, "true")

	if got := ResolveUniqueLeaf(nil, Config{}); !got {
		t.Error("expected the env var alone to enable unique leaves, got false")
	}
}

func TestResolveUniqueLeaf_UnparseableEnvFallsThroughToConfig(t *testing.T) {
	// A bool has no "empty" spelling, so a value that is not a bool at all is
	// treated as unset rather than as an accidental opt-in or opt-out.
	t.Setenv(UniqueLeafEnvVar, "yes-please")

	if got := ResolveUniqueLeaf(nil, Config{UniqueLeaf: true}); !got {
		t.Error("expected an unparseable env var to fall through to config, got false")
	}
	if got := ResolveUniqueLeaf(nil, Config{}); got {
		t.Error("expected an unparseable env var to leave the default off, got true")
	}
}
