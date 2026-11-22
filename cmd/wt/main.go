// Command wt is an interactive picker for switching between Git branches
// and worktrees.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AgileIndustrialComplex/wt/internal/action"
	"github.com/AgileIndustrialComplex/wt/internal/config"
	"github.com/AgileIndustrialComplex/wt/internal/gitdata"
	"github.com/AgileIndustrialComplex/wt/internal/github"
	"github.com/AgileIndustrialComplex/wt/internal/ui"
)

var version = "dev"

// subcommand returns the first non-flag argument and its position in args,
// skipping flags that take a separate value. The shell wrapper prepends
// --path-only, so a subcommand may not be the first argument.
func subcommand(args []string) (string, int) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			return arg, i
		}
		if (arg == "--worktree-root" || arg == "-worktree-root") && i+1 < len(args) {
			i++
		}
	}
	return "", -1
}

// One reader for all prompts: a fresh bufio.Reader would buffer and discard
// answers meant for the next prompt in the same invocation.
var stdinReader = bufio.NewReader(os.Stdin)

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

	if cmd, cmdIdx := subcommand(os.Args[1:]); cmd == "pr" {
		// The shell wrapper prepends --path-only, so "pr" is not necessarily
		// the first argument. Pass every other argument to the PR flag set.
		prArgs := append(append([]string{}, os.Args[1:cmdIdx+1]...), os.Args[cmdIdx+2:]...)
		prFlags := flag.NewFlagSet("pr", flag.ExitOnError)
		prPathOnly := prFlags.Bool("path-only", false, "print the chosen destination path, no cd (for scripting)")
		prWorktreeRoot := prFlags.String("worktree-root", "", "base directory to propose for new worktrees")
		prNoColor := prFlags.Bool("no-color", false, "disable colored output")
		// flag stops at the first positional arg, so move the PR number
		// after any flags: both `wt pr 123 --no-color` and
		// `wt pr --no-color 123` must work.
		var flagArgs, positional []string
		args := prArgs
		for i := 0; i < len(args); i++ {
			arg := args[i]
			if !strings.HasPrefix(arg, "-") {
				positional = append(positional, arg)
				continue
			}
			flagArgs = append(flagArgs, arg)
			if (arg == "--worktree-root" || arg == "-worktree-root") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		}
		_ = prFlags.Parse(append(flagArgs, positional...))
		if prFlags.NArg() > 1 {
			fmt.Fprintln(os.Stderr, "usage: wt pr [number] [--no-color] [--worktree-root DIR] [--path-only]")
			os.Exit(1)
		}
		number := ""
		if prFlags.NArg() == 1 {
			number = prFlags.Arg(0)
		}
		if err := runPR(number, *prPathOnly, *prWorktreeRoot, *prNoColor); err != nil {
			fmt.Fprintln(os.Stderr, "wt:", err)
			os.Exit(1)
		}
		return
	}

	pathOnly := flag.Bool("path-only", false, "print the chosen destination path, no cd (for scripting)")
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
		if errors.Is(err, gitdata.ErrNotAGitRepo) {
			return fmt.Errorf("%w; run `wt init bash|zsh|fish` to install shell integration, then cd into a repository", err)
		}
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

	if len(result.Delete) > 0 {
		return applyDelete(result.Delete)
	}
	if len(result.ForceUpdate) > 0 {
		return applyForceUpdate(result.ForceUpdate)
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

// runPR switches to the branch of a pull request. With a number it goes
// straight there; otherwise it lists open PRs and lets the picker choose.
// Before switching, a local branch that is strictly behind origin/<branch>
// is fast-forwarded only after an explicit yes.
func runPR(number string, pathOnly bool, worktreeRootFlag string, noColor bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	worktreeRoot := cfg.WorktreeRoot
	if worktreeRootFlag != "" {
		worktreeRoot = worktreeRootFlag
	}

	branch := number
	if number == "" {
		prs, err := github.OpenPRs()
		if err != nil {
			return err
		}
		if len(prs) == 0 {
			fmt.Fprintln(os.Stderr, "wt: no open pull requests")
			return nil
		}
		items := make([]gitdata.Item, 0, len(prs))
		for _, pr := range prs {
			items = append(items, gitdata.Item{
				Branch:  pr.Branch,
				Display: fmt.Sprintf("#%d  %s  (%s)", pr.Number, pr.Title, pr.Branch),
			})
		}
		top, err := gitdata.Toplevel()
		if err != nil {
			return err
		}
		m := ui.NewPR(items, top, worktreeRoot, noColor, cfg.DefaultAction, cfg.Keymap)
		program := tea.NewProgram(m, tea.WithOutput(os.Stderr))
		final, err := program.Run()
		if err != nil {
			return err
		}
		result := final.(ui.Model).Result()
		if result.Cancelled || result.Item.Branch == "" {
			return nil
		}
		branch = result.Item.Branch
	} else {
		pr, err := github.Get(number)
		if err != nil {
			return err
		}
		branch = pr.Branch
	}

	local, err := itemForBranch(branch)
	if err != nil {
		return err
	}
	proceed, err := action.PreparePRBranch(branch, local.Path, func() bool { return askFastForward(branch) })
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}

	local, err = itemForBranch(branch)
	if err != nil {
		return err
	}
	dest, err := switchPRBranch(local, worktreeRoot, cfg.DefaultAction)
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

// itemForBranch returns the picker item for branch, or a bare item when the
// branch does not exist locally yet.
func itemForBranch(branch string) (gitdata.Item, error) {
	items, err := gitdata.Collect()
	if err != nil {
		return gitdata.Item{}, err
	}
	for _, item := range items {
		if item.Branch == branch {
			return item, nil
		}
	}
	return gitdata.Item{Branch: branch}, nil
}

// readAnswer consumes one line from the shared stdin reader, so several
// prompts in one invocation each get their own answer.
func readAnswer() string {
	line, err := stdinReader.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(line))
}

func askFastForward(branch string) bool {
	fmt.Fprintf(os.Stderr, "Fast-forward %s to origin/%s? [y/N] ", branch, branch)
	answer := readAnswer()
	return answer == "y" || answer == "yes"
}

func switchPRBranch(item gitdata.Item, worktreeRoot, defaultAction string) (string, error) {
	if item.HasWorktree() {
		return action.Switch(item)
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		return "", err
	}
	path := ui.ProposeWorktreePath(top, worktreeRoot, item.Branch)
	switch defaultAction {
	case config.ActionSwitch:
		return action.Switch(item)
	case config.ActionWorktree:
		return action.NewWorktree(path, item.Branch)
	default:
		fmt.Fprintf(os.Stderr, "'%s' has no worktree. [s]witch here  [w]orktree at %s  [Esc] cancel: ", item.Branch, path)
		switch readAnswer() {
		case "s":
			return action.Switch(item)
		case "w":
			return action.NewWorktree(path, item.Branch)
		default:
			return "", nil
		}
	}
}

// applyDelete removes each marked branch (and its worktree, if it has one),
// reporting success to stderr on completion. It returns any error(s) action.
// DeleteWorktrees encountered so main exits non-zero, but every item is
// still attempted regardless of earlier failures.
func applyDelete(items []gitdata.Item) error {
	err := action.DeleteWorktrees(items)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wt: deleted %d branch(es)\n", len(items))
	return nil
}

// applyForceUpdate resets each confirmed branch to its upstream tip,
// reporting success to stderr on completion.
func applyForceUpdate(items []gitdata.Item) error {
	if err := action.ForceUpdate(items); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wt: force-updated %d branch(es)\n", len(items))
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
