// Package gitdata collects and merges branch and worktree information by
// shelling out to the system git binary, never reimplementing git logic.
package gitdata

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Item represents a single row in the picker: a local branch, optionally
// paired with the worktree it is checked out in.
type Item struct {
	Branch    string
	Path      string // "" if the branch has no worktree (including no main checkout)
	IsCurrent bool
	Locked    bool
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

func collect(run runnerFunc) ([]Item, error) {
	wtOut, err := run("worktree", "list", "--porcelain")
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

	top, err := toplevel(run)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(branches))
	for _, b := range branches {
		item := Item{Branch: b}
		if w, ok := byBranch[b]; ok {
			item.Path = w.path
			item.Locked = w.locked
			item.IsCurrent = normalizePath(w.path) == top
		}
		items = append(items, item)
	}
	return items, nil
}

// parseWorktreePorcelain parses the output of `git worktree list --porcelain`.
// Entries are separated by blank lines; each entry has a `worktree <path>`
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

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
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
