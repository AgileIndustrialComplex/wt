// Command wt is an interactive picker for switching between Git branches
// and worktrees.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AgileIndustrialComplex/wt/internal/action"
	"github.com/AgileIndustrialComplex/wt/internal/config"
	"github.com/AgileIndustrialComplex/wt/internal/gitdata"
	"github.com/AgileIndustrialComplex/wt/internal/ui"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: wt init bash|zsh|fish")
			os.Exit(1)
		}
		if err := runInit(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "wt:", err)
			os.Exit(1)
		}
		return
	}

	pathOnly := flag.Bool("path-only", false, "print the chosen path or branch, no cd (for scripting)")
	worktreeRoot := flag.String("worktree-root", "", "base directory to propose for new worktrees")
	noColor := flag.Bool("no-color", false, "disable colored output")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("wt", version)
		return
	}

	if err := run(*pathOnly, *worktreeRoot, *noColor); err != nil {
		fmt.Fprintln(os.Stderr, "wt:", err)
		os.Exit(1)
	}
}

func run(pathOnly bool, worktreeRootFlag string, noColor bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	worktreeRoot := cfg.WorktreeRoot
	if worktreeRootFlag != "" {
		worktreeRoot = worktreeRootFlag
	}

	items, err := gitdata.Collect()
	if err != nil {
		return err
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		return err
	}

	m := ui.NewConfigured(items, top, worktreeRoot, noColor, cfg.DefaultAction, cfg.Keymap)
	// Render to stderr, like fzf, so stdout stays clean for the final path
	// when a caller (e.g. the shell wrapper) captures it via $(...).
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return err
	}
	result := final.(ui.Model).Result()

	if result.Cancelled {
		return nil
	}

	dest, err := applyResult(result)
	if err != nil {
		return err
	}

	if pathOnly {
		fmt.Println(dest)
		return nil
	}
	if dest != "" {
		fmt.Fprintf(os.Stderr, "wt: run 'cd %s', or use shell integration (see `wt init`) to change directory automatically.\n", dest)
	}
	return nil
}

func applyResult(result ui.Result) (string, error) {
	if result.Item.HasWorktree() {
		return action.Switch(result.Item)
	}
	switch result.Resolution {
	case config.ActionWorktree:
		return action.NewWorktree(result.NewWorktreePath, result.Item.Branch)
	default:
		return action.Switch(result.Item)
	}
}
