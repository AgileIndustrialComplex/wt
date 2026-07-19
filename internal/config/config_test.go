package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.DefaultAction != ActionPrompt || cfg.Keymap != KeymapVim || cfg.WorktreeRoot != "" {
		t.Fatalf("Default() = %+v, want prompt/vim/empty root", cfg)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg != Default() {
		t.Fatalf("Load() with no config file = %+v, want defaults", cfg)
	}
}

func TestLoadFillsPartialFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".config", "wt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `worktree_root = "~/worktrees"`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := filepath.Join(home, "worktrees")
	if cfg.WorktreeRoot != want {
		t.Fatalf("WorktreeRoot = %q, want %q (expanded)", cfg.WorktreeRoot, want)
	}
	if cfg.DefaultAction != ActionPrompt || cfg.Keymap != KeymapVim {
		t.Fatalf("unset fields not defaulted: %+v", cfg)
	}
}
