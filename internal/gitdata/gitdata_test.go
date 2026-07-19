package gitdata

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

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
		"",
	}, "\x00")

	got := parseWorktreePorcelain(out)
	want := []worktreeInfo{
		{path: "/repo/proj", branch: "main"},
		{path: "/repo/proj-login", branch: "feature/login"},
		{path: "/repo/proj-release", branch: "release/2.1", locked: true},
		{path: "/repo/proj-scratch", detached: true},
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

// fakeRunner returns canned output per git subcommand, keyed by the first arg.
type fakeRunner map[string]string

func (f fakeRunner) run(args ...string) (string, error) {
	out, ok := f[args[0]]
	if !ok {
		return "", fmt.Errorf("unexpected git subcommand: %v", args)
	}
	return out, nil
}

func TestCollectMergesBranchesAndWorktrees(t *testing.T) {
	fr := fakeRunner{
		"worktree": strings.Join([]string{
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
		"branch":    "main\nfeature/login\nbugfix/api-timeout\nrelease/2.1\n",
		"rev-parse": "/repo/proj\n",
	}

	items, err := collect(fr.run)
	if err != nil {
		t.Fatalf("collect() error = %v", err)
	}

	want := []Item{
		{Branch: "main", Path: "/repo/proj", IsCurrent: true},
		{Branch: "feature/login", Path: "/repo/proj-login"},
		{Branch: "bugfix/api-timeout"},
		{Branch: "release/2.1", Path: "/repo/proj-release", Locked: true},
		{Branch: "(detached)", Path: "/repo/proj-scratch", Detached: true},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("collect() = %#v, want %#v", items, want)
	}
}

func TestCollectMovesCurrentItemFirst(t *testing.T) {
	fr := fakeRunner{
		"worktree":  "worktree /repo/proj\x00HEAD abc123\x00branch refs/heads/z-current\x00",
		"branch":    "a-first\nz-current\n",
		"rev-parse": "/repo/proj\n",
	}
	items, err := collect(fr.run)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Branch != "z-current" || !items[0].IsCurrent {
		t.Fatalf("first item = %+v, want current branch", items[0])
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
