// Package config loads the optional wt configuration file.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultAction values for what happens when a selected branch has no worktree.
const (
	ActionPrompt   = "prompt"
	ActionSwitch   = "switch"
	ActionWorktree = "worktree"
)

// Keymap hint values. All key bindings (vim, emacs, arrows) are always
// active regardless of this setting; it only affects displayed hint text.
const (
	KeymapVim        = "vim"
	KeymapEmacs      = "emacs"
	KeymapArrowsOnly = "arrows-only"
)

// Config holds the contents of ~/.config/wt/config.toml.
type Config struct {
	WorktreeRoot  string `toml:"worktree_root"`
	DefaultAction string `toml:"default_action"`
	Keymap        string `toml:"keymap"`
}

// Default returns the configuration used when no config file is present.
func Default() Config {
	return Config{
		WorktreeRoot:  "",
		DefaultAction: ActionPrompt,
		Keymap:        KeymapVim,
	}
}

// Path returns the location of the config file: ~/.config/wt/config.toml.
func Path() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".config", "wt", "config.toml"), nil
}

// Load reads the config file if present, filling in defaults for any field
// left unset. A missing file is not an error.
func Load() (Config, error) {
	cfg := Default()

	path, err := Path()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, err
	}
	if cfg.DefaultAction == "" {
		cfg.DefaultAction = ActionPrompt
	}
	if cfg.Keymap == "" {
		cfg.Keymap = KeymapVim
	}
	if cfg.WorktreeRoot != "" {
		cfg.WorktreeRoot = expandHome(cfg.WorktreeRoot)
	}
	return cfg, nil
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		if p == "~" {
			return home
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
