package action

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/AgileIndustrialComplex/wt/internal/gitdata"
)

func recordingRunner() (runnerFunc, *[][]string) {
	var calls [][]string
	return func(args ...string) error {
		calls = append(calls, append([]string{}, args...))
		return nil
	}, &calls
}

func TestSwitchWithExistingWorktreeReturnsPathAndRunsNoGit(t *testing.T) {
	run, calls := recordingRunner()
	item := gitdata.Item{Branch: "feature/login", Path: "/repo/proj-login"}

	path, err := switchWith(run, item)
	if err != nil {
		t.Fatalf("switchWith() error = %v", err)
	}
	if path != "/repo/proj-login" {
		t.Fatalf("path = %q, want worktree path", path)
	}
	if len(*calls) != 0 {
		t.Fatalf("git calls = %v, want none (no mutation for existing worktree)", *calls)
	}
}

func TestSwitchWithNoWorktreeRunsGitSwitch(t *testing.T) {
	run, calls := recordingRunner()
	item := gitdata.Item{Branch: "bugfix/api-timeout"}

	path, err := switchWith(run, item)
	if err != nil {
		t.Fatalf("switchWith() error = %v", err)
	}
	if path != "" {
		t.Fatalf("path = %q, want empty (in-place switch)", path)
	}
	want := [][]string{{"switch", "bugfix/api-timeout"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
	}
}

func TestSwitchWithPropagatesGitError(t *testing.T) {
	run := func(args ...string) error { return fmt.Errorf("boom") }
	_, err := switchWith(run, gitdata.Item{Branch: "main"})
	if err == nil {
		t.Fatal("switchWith() error = nil, want error propagated from git")
	}
}

func TestNewWorktreeWithRunsGitWorktreeAdd(t *testing.T) {
	run, calls := recordingRunner()
	path, err := newWorktreeWith(run, "/repo/proj-login", "feature/login")
	if err != nil {
		t.Fatalf("newWorktreeWith() error = %v", err)
	}
	if path != "/repo/proj-login" {
		t.Fatalf("path = %q, want /repo/proj-login", path)
	}
	want := [][]string{{"worktree", "add", "/repo/proj-login", "feature/login"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
	}
}

func TestRemoveWorktreesWithRunsGitWorktreeRemoveForEach(t *testing.T) {
	run, calls := recordingRunner()
	items := []gitdata.Item{
		{Branch: "feature/login", Path: "/repo/proj-login"},
		{Branch: "bugfix/api-timeout", Path: "/repo/proj-api-timeout"},
	}
	removed, err := removeWorktreesWith(run, items)
	if err != nil {
		t.Fatalf("removeWorktreesWith() error = %v", err)
	}
	if !reflect.DeepEqual(removed, items) {
		t.Fatalf("removed = %v, want %v", removed, items)
	}
	want := [][]string{
		{"worktree", "remove", "/repo/proj-login"},
		{"worktree", "remove", "/repo/proj-api-timeout"},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
	}
}

func TestRemoveWorktreesWithStopsAtFirstError(t *testing.T) {
	var calls [][]string
	run := func(args ...string) error {
		calls = append(calls, append([]string{}, args...))
		return fmt.Errorf("boom")
	}
	items := []gitdata.Item{
		{Branch: "feature/login", Path: "/repo/proj-login"},
		{Branch: "bugfix/api-timeout", Path: "/repo/proj-api-timeout"},
	}
	removed, err := removeWorktreesWith(run, items)
	if err == nil {
		t.Fatal("removeWorktreesWith() error = nil, want error propagated from git")
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want none", removed)
	}
	if len(calls) != 1 {
		t.Fatalf("git calls = %v, want exactly one call before stopping", calls)
	}
}

func TestRemoveWorktreesWithReturnsItemsRemovedBeforeFailure(t *testing.T) {
	var calls int
	run := func(args ...string) error {
		calls++
		if calls == 2 {
			return fmt.Errorf("boom")
		}
		return nil
	}
	items := []gitdata.Item{
		{Branch: "feature/login", Path: "/repo/proj-login"},
		{Branch: "bugfix/api-timeout", Path: "/repo/proj-api-timeout"},
	}
	removed, err := removeWorktreesWith(run, items)
	if err == nil {
		t.Fatal("removeWorktreesWith() error = nil, want error propagated from git")
	}
	if !reflect.DeepEqual(removed, items[:1]) {
		t.Fatalf("removed = %v, want %v", removed, items[:1])
	}
}
