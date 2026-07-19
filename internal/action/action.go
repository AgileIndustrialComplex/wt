// Package action executes the single git operation implied by a user's
// selection. It never touches repository state beyond calling the real git
// binary.
package action

import (
	"os"
	"os/exec"

	"github.com/AgileIndustrialComplex/wt/internal/gitdata"
)

type runnerFunc func(args ...string) error

func runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// Switch performs the operation for a selected item. If the item already
// has a worktree, no git command runs: the worktree path is returned for the
// shell wrapper to cd into. Otherwise the branch is checked out in place via
// `git switch` and "" is returned, since the current directory should not
// change.
func Switch(item gitdata.Item) (path string, err error) {
	return switchWith(runGit, item)
}

func switchWith(run runnerFunc, item gitdata.Item) (string, error) {
	if item.HasWorktree() {
		return item.Path, nil
	}
	if err := run("switch", item.Branch); err != nil {
		return "", err
	}
	return "", nil
}

// NewWorktree creates a worktree for branch at path via `git worktree add`
// and returns path for the shell wrapper to cd into.
func NewWorktree(path, branch string) (string, error) {
	return newWorktreeWith(runGit, path, branch)
}

func newWorktreeWith(run runnerFunc, path, branch string) (string, error) {
	if err := run("worktree", "add", path, branch); err != nil {
		return "", err
	}
	return path, nil
}
