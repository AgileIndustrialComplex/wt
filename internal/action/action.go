// Package action executes the single git operation implied by a user's
// selection. It never touches repository state beyond calling the real git
// binary.
package action

import (
	"errors"
	"fmt"
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

// DeleteWorktrees removes each item's worktree, if it has one, via
// `git worktree remove`, or `git worktree remove --force` when item.Dirty is
// set, or `git worktree remove --force --force` when item.Locked is set (a
// single --force is not enough for git to remove a locked worktree), then
// deletes the branch via `git branch -d`, or `git branch -D` (force) when
// item.Unmerged is set. Callers must only pass Dirty, Locked, or Unmerged
// items after the user has given explicit, phrase-typed consent (see
// internal/ui's modeConfirmForceDelete) — this function performs no
// confirmation of its own. Items without a worktree skip straight to the
// branch delete. Every item is attempted even if earlier ones fail; all
// failures are combined into a single returned error.
func DeleteWorktrees(items []gitdata.Item) error {
	return deleteWorktreesWith(runGit, items)
}

func deleteWorktreesWith(run runnerFunc, items []gitdata.Item) error {
	var errs []error
	for _, item := range items {
		if item.IsCurrent {
			errs = append(errs, fmt.Errorf("%s: cannot delete the current worktree", item.Branch))
			continue
		}
		if item.HasWorktree() {
			args := []string{"worktree", "remove"}
			switch {
			case item.Locked:
				args = append(args, "--force", "--force")
			case item.Dirty:
				args = append(args, "--force")
			}
			args = append(args, item.Path)
			if err := run(args...); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", item.Branch, err))
				continue
			}
		}
		if item.Detached {
			continue
		}
		deleteFlag := "-d"
		if item.Unmerged {
			deleteFlag = "-D"
		}
		if err := run("branch", deleteFlag, item.Branch); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", item.Branch, err))
		}
	}
	return errors.Join(errs...)
}
