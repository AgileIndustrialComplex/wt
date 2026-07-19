package action

import (
	"fmt"
	"reflect"
	"strings"
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

func TestDeleteWorktreesWithRemovesWorktreeThenDeletesBranch(t *testing.T) {
	run, calls := recordingRunner()
	items := []gitdata.Item{
		{Branch: "feature/login", Path: "/repo/proj-login"},
		{Branch: "release/2.1", Path: "/repo/proj-release"},
	}

	if err := deleteWorktreesWith(run, items); err != nil {
		t.Fatalf("deleteWorktreesWith() error = %v", err)
	}
	want := [][]string{
		{"worktree", "remove", "/repo/proj-login"},
		{"branch", "-d", "feature/login"},
		{"worktree", "remove", "/repo/proj-release"},
		{"branch", "-d", "release/2.1"},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
	}
}

func TestDeleteWorktreesWithSkipsBranchDeleteOnWorktreeRemoveFailure(t *testing.T) {
	run := func(args ...string) error {
		if args[0] == "worktree" {
			return fmt.Errorf("worktree remove failed")
		}
		t.Fatalf("unexpected git call: %v", args)
		return nil
	}
	item := gitdata.Item{Branch: "feature/login", Path: "/repo/proj-login"}

	err := deleteWorktreesWith(run, []gitdata.Item{item})
	if err == nil {
		t.Fatal("deleteWorktreesWith() error = nil, want error propagated from worktree remove")
	}
}

func TestDeleteWorktreesWithContinuesAfterFailureAndCombinesErrors(t *testing.T) {
	run := func(args ...string) error {
		if args[0] == "worktree" && args[2] == "/repo/proj-login" {
			return fmt.Errorf("boom")
		}
		return nil
	}
	items := []gitdata.Item{
		{Branch: "feature/login", Path: "/repo/proj-login"},
		{Branch: "release/2.1", Path: "/repo/proj-release"},
	}

	err := deleteWorktreesWith(run, items)
	if err == nil {
		t.Fatal("deleteWorktreesWith() error = nil, want combined error from the failing item")
	}
	if !strings.Contains(err.Error(), "feature/login") {
		t.Fatalf("error = %v, want it to mention feature/login", err)
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
