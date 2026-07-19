package gitdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRunGitUsesStableLocale(t *testing.T) {
	dir := t.TempDir()
	git := filepath.Join(dir, "git")
	script := "#!/bin/sh\nif [ \"$LC_ALL\" = C ]; then\n  echo 'fatal: not a git repository' >&2\nelse\n  echo 'fatal: kein Git-Repository' >&2\nfi\nexit 128\n"
	if err := os.WriteFile(git, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LC_ALL", "de_DE.UTF-8")

	_, err := runGit("worktree", "list")
	if !errors.Is(err, ErrNotAGitRepo) {
		t.Fatalf("runGit() error = %v, want ErrNotAGitRepo", err)
	}
}

func TestParseWorktreePorcelain(t *testing.T) {
	out := strings.Join([]string{
		"worktree /repo/proj",
		"HEAD abc123",
		"branch refs/heads/main",
		"",
		"worktree /repo/proj-login",
		"HEAD def456",
		"branch refs/heads/feature/login",
		"",
		"worktree /repo/proj-release",
		"HEAD ghi789",
		"branch refs/heads/release/2.1",
		"locked",
		"",
		"worktree /repo/proj-scratch",
		"HEAD jkl012",
		"detached",
		"prunable gitdir file points to non-existent location",
		"",
	}, "\x00")

	got := parseWorktreePorcelain(out)
	want := []worktreeInfo{
		{path: "/repo/proj", branch: "main"},
		{path: "/repo/proj-login", branch: "feature/login"},
		{path: "/repo/proj-release", branch: "release/2.1", locked: true},
		{path: "/repo/proj-scratch", detached: true, prunable: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseWorktreePorcelain() = %#v, want %#v", got, want)
	}
}

func TestParseWorktreePorcelainNoTrailingBlank(t *testing.T) {
	out := "worktree /repo/proj\x00HEAD abc123\x00branch refs/heads/main"
	got := parseWorktreePorcelain(out)
	want := []worktreeInfo{{path: "/repo/proj", branch: "main"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseWorktreePorcelain() = %#v, want %#v", got, want)
	}
}

func TestParseLines(t *testing.T) {
	got := parseLines("main\nfeature/login\n\nrelease/2.1\n")
	want := []string{"main", "feature/login", "release/2.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLines() = %#v, want %#v", got, want)
	}
}

// fakeRunner returns canned output per full git invocation, keyed by the
// space-joined args (e.g. "branch --format=%(refname:short) --merged").
type fakeRunner map[string]string

func (f fakeRunner) run(args ...string) (string, error) {
	out, ok := f[strings.Join(args, " ")]
	if !ok {
		return "", fmt.Errorf("unexpected git subcommand: %v", args)
	}
	return out, nil
}

func TestCollectMergesBranchesAndWorktrees(t *testing.T) {
	fr := fakeRunner{
		"worktree list --porcelain -z": strings.Join([]string{
			"worktree /repo/proj",
			"HEAD abc123",
			"branch refs/heads/main",
			"",
			"worktree /repo/proj-login",
			"HEAD def456",
			"branch refs/heads/feature/login",
			"",
			"worktree /repo/proj-release",
			"HEAD ghi789",
			"branch refs/heads/release/2.1",
			"locked",
			"",
			"worktree /repo/proj-scratch",
			"HEAD jkl012",
			"detached",
			"",
		}, "\x00"),
		"branch --list --format=%(refname:short)":                        "main\nfeature/login\nbugfix/api-timeout\nrelease/2.1\n",
		"branch --format=%(refname:short) --merged":                      "main\nbugfix/api-timeout\n",
		"rev-parse --show-toplevel":                                      "/repo/proj\n",
		"-C /repo/proj status --porcelain --untracked-files=all":         "",
		"-C /repo/proj-login status --porcelain --untracked-files=all":   "",
		"-C /repo/proj-release status --porcelain --untracked-files=all": "",
		"-C /repo/proj-scratch status --porcelain --untracked-files=all": "",
	}

	items, err := collect(fr.run)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}

	want := []Item{
		{Branch: "main", Path: "/repo/proj", IsCurrent: true},
		{Branch: "feature/login", Path: "/repo/proj-login", Unmerged: true},
		{Branch: "bugfix/api-timeout"},
		{Branch: "release/2.1", Path: "/repo/proj-release", Locked: true, Unmerged: true},
		{Branch: "(detached)", Path: "/repo/proj-scratch", Detached: true},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("collect() = %#v, want %#v", items, want)
	}
}

func TestCollectMovesCurrentItemFirst(t *testing.T) {
	fr := fakeRunner{
		"worktree list --porcelain -z":                           "worktree /repo/proj\x00HEAD abc123\x00branch refs/heads/z-current\x00",
		"branch --list --format=%(refname:short)":                "a-first\nz-current\n",
		"branch --format=%(refname:short) --merged":              "a-first\nz-current\n",
		"rev-parse --show-toplevel":                              "/repo/proj\n",
		"-C /repo/proj status --porcelain --untracked-files=all": "",
	}
	items, err := collect(fr.run)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Branch != "z-current" || !items[0].IsCurrent {
		t.Fatalf("first item = %+v, want current branch", items[0])
	}
}

func TestCollectMovesCurrentFirstPreservingRemainingOrder(t *testing.T) {
	fr := fakeRunner{
		"worktree list --porcelain -z": strings.Join([]string{
			"worktree /repo/proj-current",
			"HEAD abc123",
			"branch refs/heads/z-current",
			"",
			"worktree /repo/proj-scratch",
			"HEAD def456",
			"detached",
			"",
		}, "\x00"),
		"branch --list --format=%(refname:short)":                        "a-first\nb-middle\nz-current\n",
		"branch --format=%(refname:short) --merged":                      "a-first\nb-middle\nz-current\n",
		"rev-parse --show-toplevel":                                      "/repo/proj-current\n",
		"-C /repo/proj-current status --porcelain --untracked-files=all": "",
		"-C /repo/proj-scratch status --porcelain --untracked-files=all": "",
	}

	items, err := collect(fr.run)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}

	want := []Item{
		{Branch: "z-current", Path: "/repo/proj-current", IsCurrent: true},
		{Branch: "a-first"},
		{Branch: "b-middle"},
		{Branch: "(detached)", Path: "/repo/proj-scratch", Detached: true},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("collect() = %#v, want %#v", items, want)
	}
}

func TestCollectMarksUnmergedBranches(t *testing.T) {
	fr := fakeRunner{
		"worktree list --porcelain -z":              "",
		"branch --list --format=%(refname:short)":   "main\nfeature/wip\n",
		"branch --format=%(refname:short) --merged": "main\n",
		"rev-parse --show-toplevel":                 "/repo/proj\n",
	}

	items, err := collect(fr.run)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}

	want := []Item{
		{Branch: "main"},
		{Branch: "feature/wip", Unmerged: true},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("collect() = %#v, want %#v", items, want)
	}
}

func TestCollectMarksDirtyWorktrees(t *testing.T) {
	fr := fakeRunner{
		"worktree list --porcelain -z": strings.Join([]string{
			"worktree /repo/proj",
			"HEAD abc123",
			"branch refs/heads/main",
			"",
			"worktree /repo/proj-login",
			"HEAD def456",
			"branch refs/heads/feature/login",
			"",
		}, "\x00"),
		"branch --list --format=%(refname:short)":                      "main\nfeature/login\n",
		"branch --format=%(refname:short) --merged":                    "main\nfeature/login\n",
		"rev-parse --show-toplevel":                                    "/repo/proj\n",
		"-C /repo/proj status --porcelain --untracked-files=all":       "",
		"-C /repo/proj-login status --porcelain --untracked-files=all": " M tracked.go\n?? scratch.txt\n",
	}

	items, err := collect(fr.run)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}

	want := []Item{
		{Branch: "main", Path: "/repo/proj", IsCurrent: true},
		{Branch: "feature/login", Path: "/repo/proj-login", Dirty: true},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("collect() = %#v, want %#v", items, want)
	}
}

func TestCollectPropagatesStatusErrors(t *testing.T) {
	fr := fakeRunner{
		"worktree list --porcelain -z":              "worktree /repo/proj\x00HEAD abc123\x00branch refs/heads/main\x00",
		"branch --list --format=%(refname:short)":   "main\n",
		"branch --format=%(refname:short) --merged": "main\n",
		"rev-parse --show-toplevel":                 "/repo/proj\n",
		// Status response deliberately omitted to produce an unexpected-subcommand error.
	}
	if _, err := collect(fr.run); err == nil {
		t.Fatal("collect() error = nil, want error when git status fails")
	}
}

func TestCollectSkipsStatusForPrunableWorktrees(t *testing.T) {
	fr := fakeRunner{
		"worktree list --porcelain -z": strings.Join([]string{
			"worktree /repo/proj",
			"HEAD abc123",
			"branch refs/heads/main",
			"",
			"worktree /repo/missing",
			"HEAD def456",
			"branch refs/heads/stale",
			"prunable gitdir file points to non-existent location",
			"",
		}, "\x00"),
		"branch --list --format=%(refname:short)":                "main\nstale\n",
		"branch --format=%(refname:short) --merged":              "main\nstale\n",
		"rev-parse --show-toplevel":                              "/repo/proj\n",
		"-C /repo/proj status --porcelain --untracked-files=all": "",
	}

	items, err := collect(fr.run)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	if len(items) != 2 || items[1].Path != "/repo/missing" || items[1].Dirty {
		t.Fatalf("collect() = %#v, want clean stale worktree without status probe", items)
	}
}

func TestCollectSkipsMergedQueryWhenThereAreNoBranches(t *testing.T) {
	fr := fakeRunner{
		"worktree list --porcelain -z":            "",
		"branch --list --format=%(refname:short)": "",
		"rev-parse --show-toplevel":               "/repo/proj\n",
	}

	items, err := collect(fr.run)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("collect() = %#v, want no items", items)
	}
}

func TestParseWorktreePorcelainPreservesSpecialPath(t *testing.T) {
	out := "worktree /repo/café folder\\name\x00HEAD abc123\x00detached\x00\x00"
	got := parseWorktreePorcelain(out)
	want := []worktreeInfo{{path: "/repo/café folder\\name", detached: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseWorktreePorcelain() = %#v, want %#v", got, want)
	}
}

func TestCollectPropagatesGitErrors(t *testing.T) {
	fr := fakeRunner{} // every subcommand is "unexpected" -> errors immediately
	if _, err := collect(fr.run); err == nil {
		t.Fatal("collect() error = nil, want error when git worktree list fails")
	}
}

func TestItemHasWorktree(t *testing.T) {
	if (Item{Branch: "main"}).HasWorktree() {
		t.Error("HasWorktree() = true for item with no path, want false")
	}
	if !(Item{Branch: "main", Path: "/repo/proj"}).HasWorktree() {
		t.Error("HasWorktree() = false for item with path, want true")
	}
}
