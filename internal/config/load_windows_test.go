//go:build windows

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_HomeBoundaryIsCaseInsensitive(t *testing.T) {
	home := t.TempDir()
	setUserHome(t, home)

	repoDir := filepath.Join(home, "Workspace", "personal", "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, home, "max_trees = 3\n")

	cfg, err := Load(strings.ToUpper(repoDir))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MaxTrees != DefaultConfig().MaxTrees {
		t.Fatalf("MaxTrees: got %d, want default %d", cfg.MaxTrees, DefaultConfig().MaxTrees)
	}
}
