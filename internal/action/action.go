// Package action executes the single git operation implied by a user's
// selection. It never touches repository state beyond calling the real git
// binary.
package action

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

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

type outputRunnerFunc func(args ...string) (string, error)

func runGitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out.String(), nil
}

// PreparePRBranch fetches a PR head branch from origin and makes the local
// branch ready to switch to. A local branch strictly behind origin/<branch>
// is fast-forwarded only when confirm returns true; in that case a false
// return with a nil error means the user declined. A local branch that has
// diverged is rejected — only a fast-forward is allowed here. A missing
// local branch is created from the remote tip without asking. path is the
// branch's worktree, or "" when it has none.
func PreparePRBranch(branch, path string, confirm func() bool) (bool, error) {
	return preparePRBranchWith(runGitOutput, branch, path, confirm)
}

func preparePRBranchWith(run outputRunnerFunc, branch, path string, confirm func() bool) (bool, error) {
	if _, err := run("fetch", "origin", branch); err != nil {
		return false, fmt.Errorf("fetch origin %s: %w", branch, err)
	}
	ref := "origin/" + branch
	local, err := run("rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil {
		if _, err := run("branch", branch, ref); err != nil {
			return false, fmt.Errorf("create %s from %s: %w", branch, ref, err)
		}
		return true, nil
	}
	remote, err := run("rev-parse", "--verify", "--quiet", ref)
	if err != nil {
		return false, fmt.Errorf("branch %s: no %s: %w", branch, ref, err)
	}
	if strings.TrimSpace(local) == strings.TrimSpace(remote) {
		return true, nil
	}
	ahead, err := run("rev-list", "--count", ref+".."+branch)
	if err != nil {
		return false, err
	}
	behind, err := run("rev-list", "--count", branch+".."+ref)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(ahead) != "0" {
		return false, fmt.Errorf("branch %s has diverged from %s; cannot fast-forward", branch, ref)
	}
	if strings.TrimSpace(behind) == "0" {
		return true, nil
	}
	if !confirm() {
		return false, nil
	}
	if path != "" {
		if _, err := run("-C", path, "merge", "--ff-only", ref); err != nil {
			return false, fmt.Errorf("fast-forward %s: %w", branch, err)
		}
	} else {
		if _, err := run("branch", "-f", branch, ref); err != nil {
			return false, fmt.Errorf("fast-forward %s: %w", branch, err)
		}
	}
	return true, nil
}

// ForceUpdate resets each branch to its upstream tip. It runs a single
// `git fetch origin`, then `git reset --hard origin/<branch>` for branches
// with a worktree and `git branch -f <branch> origin/<branch>` for plain
// branches. Callers must only pass items after the user has confirmed the
// action (see internal/ui's modeConfirmForceUpdate) — this function performs
// no confirmation of its own. Detached worktrees have no branch and are
// skipped. Every item is attempted even if earlier ones fail; all failures
// are combined into a single returned error.
func ForceUpdate(items []gitdata.Item) error {
	return forceUpdateWith(runGit, items)
}

func forceUpdateWith(run runnerFunc, items []gitdata.Item) error {
	var targets []gitdata.Item
	for _, item := range items {
		if !item.Detached {
			targets = append(targets, item)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	if err := run("fetch", "origin"); err != nil {
		return fmt.Errorf("fetch origin: %w", err)
	}
	var errs []error
	for _, item := range targets {
		ref := "origin/" + item.Branch
		if item.HasWorktree() {
			if err := run("-C", item.Path, "reset", "--hard", ref); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", item.Branch, err))
			}
			continue
		}
		if err := run("branch", "-f", item.Branch, ref); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", item.Branch, err))
		}
	}
	return errors.Join(errs...)
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
