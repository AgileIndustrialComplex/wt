// Package gitdata collects and merges branch and worktree information by
// shelling out to the system git binary, never reimplementing git logic.
package gitdata

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotAGitRepo is returned by Collect and Toplevel when the current
// directory is outside any Git repository.
var ErrNotAGitRepo = errors.New("not a git repository (or any of the parent directories)")

// Item represents a single row in the picker: a local branch, optionally
// paired with the worktree it is checked out in.
type Item struct {
	Branch    string
	Path      string // "" if the branch has no worktree (including no main checkout)
	IsCurrent bool
	Locked    bool
	Detached  bool
	Unmerged  bool // true if the branch is not fully merged into HEAD (git branch -d would refuse it)
	Dirty     bool // true if the worktree has modified or untracked files (git worktree remove would refuse it)
}

// HasWorktree reports whether the branch is checked out anywhere.
func (i Item) HasWorktree() bool {
	return i.Path != ""
}

type worktreeInfo struct {
	path     string
	branch   string // short branch name, "" if detached
	locked   bool
	detached bool
}

// Collect runs git branch and git worktree list against the repository
// containing the current working directory and returns the merged item list.
func Collect() ([]Item, error) {
	return collect(runGit)
}

// Toplevel returns the top-level directory of the repository containing the
// current working directory.
func Toplevel() (string, error) {
	return toplevel(runGit)
}

func toplevel(run runnerFunc) (string, error) {
	out, err := run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return normalizePath(strings.TrimSpace(out)), nil
}

type runnerFunc func(args ...string) (string, error)

func runGit(args ...string) (string, error) {
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
		if strings.Contains(msg, "not a git repository") {
			return "", ErrNotAGitRepo
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out.String(), nil
}

func collect(run runnerFunc) ([]Item, error) {
	wtOut, err := run("worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	worktrees := parseWorktreePorcelain(wtOut)

	byBranch := make(map[string]worktreeInfo, len(worktrees))
	for _, w := range worktrees {
		if w.branch != "" {
			byBranch[w.branch] = w
		}
	}

	brOut, err := run("branch", "--list", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	branches := parseLines(brOut)

	merged := make(map[string]bool, len(branches))
	if len(branches) > 0 {
		mergedOut, err := run("branch", "--format=%(refname:short)", "--merged")
		if err != nil {
			return nil, err
		}
		for _, b := range parseLines(mergedOut) {
			merged[b] = true
		}
	}

	top, err := toplevel(run)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(branches)+len(worktrees))
	for _, b := range branches {
		item := Item{Branch: b, Unmerged: !merged[b]}
		if w, ok := byBranch[b]; ok {
			item.Path = w.path
			item.Locked = w.locked
			item.IsCurrent = normalizePath(w.path) == top
			dirty, err := isDirty(run, w.path)
			if err != nil {
				return nil, err
			}
			item.Dirty = dirty
		}
		items = append(items, item)
	}
	for _, w := range worktrees {
		if !w.detached {
			continue
		}
		dirty, err := isDirty(run, w.path)
		if err != nil {
			return nil, err
		}
		items = append(items, Item{
			Branch:    "(detached)",
			Path:      w.path,
			IsCurrent: normalizePath(w.path) == top,
			Locked:    w.locked,
			Detached:  true,
			Dirty:     dirty,
		})
	}
	for i := range items {
		if items[i].IsCurrent {
			current := items[i]
			items = append(items[:i], items[i+1:]...)
			items = append([]Item{current}, items...)
			break
		}
	}
	return items, nil
}

// parseWorktreePorcelain parses the output of `git worktree list --porcelain -z`.
// Entries are separated by empty records; each entry has a `worktree <path>`
// line followed by either `branch <ref>` or `detached`, and may include a
// bare `locked` or `locked <reason>` line and a bare `prunable <reason>` line.
func parseWorktreePorcelain(out string) []worktreeInfo {
	var result []worktreeInfo
	var cur *worktreeInfo

	flush := func() {
		if cur != nil {
			result = append(result, *cur)
			cur = nil
		}
	}

	for _, line := range strings.Split(out, "\x00") {
		if line == "" {
			flush()
			continue
		}
		key, rest, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			cur = &worktreeInfo{path: rest}
		case "branch":
			if cur != nil {
				cur.branch = strings.TrimPrefix(rest, "refs/heads/")
			}
		case "detached":
			if cur != nil {
				cur.detached = true
			}
		case "locked":
			if cur != nil {
				cur.locked = true
			}
		}
	}
	flush()
	return result
}

func parseLines(out string) []string {
	var result []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func normalizePath(p string) string {
	return filepath.Clean(p)
}

// isDirty reports whether the worktree at path has modified or untracked
// files — the same condition that makes `git worktree remove` (without
// --force) refuse to delete it.
func isDirty(run runnerFunc, path string) (bool, error) {
	out, err := run("-C", path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}
