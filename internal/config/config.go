package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/kunchenguid/treehouse/internal/git"
)

type Config struct {
	MaxTrees int    `toml:"max_trees"`
	Root     string `toml:"root"`
	Hooks    Hooks  `toml:"hooks,omitempty"`
}

type Hooks struct {
	PostCreate []string `toml:"post_create,omitempty"`
	PreDestroy []string `toml:"pre_destroy,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		MaxTrees: 16,
	}
}

func Load(repoRoot string) (Config, error) {
	cfg := DefaultConfig()

	repoPath, hasRepoConfig, err := findRepoConfig(repoRoot)
	if err != nil {
		return cfg, err
	}
	if hasRepoConfig {
		if _, err := toml.DecodeFile(repoPath, &cfg); err != nil {
			return cfg, err
		}
		cfg.Hooks = Hooks{}
		cfg.Root = resolveRootFrom(cfg.Root, filepath.Dir(repoPath))
	}

	userCfg, hasUserConfig, err := loadUser()
	if err != nil {
		return cfg, err
	}
	if hasUserConfig {
		if !hasRepoConfig {
			cfg = userCfg
		} else {
			cfg.Hooks = userCfg.Hooks
		}
	}

	return cfg, nil
}

func findRepoConfig(repoRoot string) (string, bool, error) {
	dir, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", false, err
	}
	dir = filepath.Clean(dir)

	home := ""
	if userHome, err := os.UserHomeDir(); err == nil {
		if absoluteHome, err := filepath.Abs(userHome); err == nil {
			home = filepath.Clean(absoluteHome)
		}
	}

	for {
		if dir == home {
			return "", false, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}

		path := filepath.Join(dir, "treehouse.toml")
		if _, err := os.Stat(path); err == nil {
			return path, true, nil
		} else if !os.IsNotExist(err) {
			return "", false, err
		}

		dir = parent
	}
}

func resolveRootFrom(root string, dir string) string {
	if root == "" {
		return ""
	}

	expanded := os.ExpandEnv(root)
	if filepath.IsAbs(expanded) {
		return expanded
	}
	return filepath.Join(dir, expanded)
}

// LoadGlobal returns the default configuration merged with user-level config.
// It intentionally ignores repo-level config because callers may run without a
// repository context.
func LoadGlobal() (Config, error) {
	cfg := DefaultConfig()
	userCfg, hasUserConfig, err := loadUser()
	if err != nil {
		return cfg, err
	}
	if hasUserConfig {
		cfg = userCfg
	}
	return cfg, nil
}

func loadUser() (Config, bool, error) {
	cfg := DefaultConfig()
	if home, err := os.UserHomeDir(); err == nil {
		userPath := filepath.Join(home, ".config", "treehouse", "config.toml")
		if _, err := os.Stat(userPath); err == nil {
			if _, err := toml.DecodeFile(userPath, &cfg); err != nil {
				return cfg, false, err
			}
			return cfg, true, nil
		}
	}

	return cfg, false, nil
}

func ResolvePoolDir(repoRoot string, root string) (string, error) {
	// Use remote URL for the hash when available; fall back to the
	// absolute repo path for purely-local repositories.
	hashInput, err := git.GetRemoteURL(repoRoot)
	if err != nil {
		hashInput = repoRoot
	}

	repoName := filepath.Base(repoRoot)
	shortHash := git.ShortHash(hashInput)
	poolName := repoName + "-" + shortHash

	poolRoot, err := ResolvePoolRoot(repoRoot, root)
	if err != nil {
		return "", err
	}
	return filepath.Join(poolRoot, poolName), nil
}

// ResolvePoolRoot resolves the directory that contains per-repository pools.
// Relative roots require repoRoot because they are resolved from the repository
// root.
func ResolvePoolRoot(repoRoot string, root string) (string, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".treehouse"), nil
	}

	expanded := os.ExpandEnv(root)
	if !filepath.IsAbs(expanded) {
		if repoRoot == "" {
			return "", fmt.Errorf("relative treehouse root %q requires a repository", root)
		}
		expanded = filepath.Join(repoRoot, expanded)
	}
	return filepath.Join(expanded, ".treehouse"), nil
}
