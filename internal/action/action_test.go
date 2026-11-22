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

func TestDeleteWorktreesWithForcesDeleteOfUnmergedBranch(t *testing.T) {
	run, calls := recordingRunner()
	items := []gitdata.Item{
		{Branch: "feature/wip", Path: "/repo/proj-wip", Unmerged: true},
		{Branch: "release/2.1", Path: "/repo/proj-release"},
	}

	if err := deleteWorktreesWith(run, items); err != nil {
		t.Fatalf("deleteWorktreesWith() error = %v", err)
	}
	want := [][]string{
		{"worktree", "remove", "/repo/proj-wip"},
		{"branch", "-D", "feature/wip"},
		{"worktree", "remove", "/repo/proj-release"},
		{"branch", "-d", "release/2.1"},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
	}
}

func TestDeleteWorktreesWithForcesRemoveOfDirtyWorktree(t *testing.T) {
	run, calls := recordingRunner()
	items := []gitdata.Item{
		{Branch: "feature/wip", Path: "/repo/proj-wip", Dirty: true},
		{Branch: "release/2.1", Path: "/repo/proj-release"},
	}

	if err := deleteWorktreesWith(run, items); err != nil {
		t.Fatalf("deleteWorktreesWith() error = %v", err)
	}
	want := [][]string{
		{"worktree", "remove", "--force", "/repo/proj-wip"},
		{"branch", "-d", "feature/wip"},
		{"worktree", "remove", "/repo/proj-release"},
		{"branch", "-d", "release/2.1"},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
	}
}

func TestDeleteWorktreesWithBranchWithoutWorktreeSkipsWorktreeRemove(t *testing.T) {
	run, calls := recordingRunner()
	item := gitdata.Item{Branch: "bugfix/api-timeout"}

	if err := deleteWorktreesWith(run, []gitdata.Item{item}); err != nil {
		t.Fatalf("deleteWorktreesWith() error = %v", err)
	}
	want := [][]string{{"branch", "-d", "bugfix/api-timeout"}}
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

func TestDeleteWorktreesWithRejectsCurrentWorktree(t *testing.T) {
	run, calls := recordingRunner()
	item := gitdata.Item{Branch: "main", Path: "/repo/proj", IsCurrent: true}

	err := deleteWorktreesWith(run, []gitdata.Item{item})
	if err == nil || !strings.Contains(err.Error(), "current worktree") {
		t.Fatalf("deleteWorktreesWith() error = %v, want current worktree error", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("git calls = %v, want none", *calls)
	}
}

func TestDeleteWorktreesWithForcesRemoveOfLockedWorktreeTwice(t *testing.T) {
	run, calls := recordingRunner()
	item := gitdata.Item{Branch: "release/2.1", Path: "/repo/proj-release", Locked: true}

	if err := deleteWorktreesWith(run, []gitdata.Item{item}); err != nil {
		t.Fatalf("deleteWorktreesWith() error = %v", err)
	}
	want := [][]string{
		{"worktree", "remove", "--force", "--force", "/repo/proj-release"},
		{"branch", "-d", "release/2.1"},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
	}
}

func TestDeleteWorktreesWithLockedAndDirtyUsesOnlyDoubleForce(t *testing.T) {
	run, calls := recordingRunner()
	item := gitdata.Item{Branch: "release/2.1", Path: "/repo/proj-release", Locked: true, Dirty: true}

	if err := deleteWorktreesWith(run, []gitdata.Item{item}); err != nil {
		t.Fatalf("deleteWorktreesWith() error = %v", err)
	}
	want := [][]string{
		{"worktree", "remove", "--force", "--force", "/repo/proj-release"},
		{"branch", "-d", "release/2.1"},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
	}
}

func TestDeleteWorktreesWithDetachedSkipsBranchDelete(t *testing.T) {
	run, calls := recordingRunner()
	item := gitdata.Item{Branch: "(detached)", Path: "/repo/proj-scratch", Detached: true}

	if err := deleteWorktreesWith(run, []gitdata.Item{item}); err != nil {
		t.Fatalf("deleteWorktreesWith() error = %v", err)
	}
	want := [][]string{{"worktree", "remove", "/repo/proj-scratch"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
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

func prFakeRunner(responses map[string]string) (outputRunnerFunc, *[][]string) {
	var calls [][]string
	run := func(args ...string) (string, error) {
		calls = append(calls, append([]string{}, args...))
		key := strings.Join(args, " ")
		if out, ok := responses[key]; ok {
			return out, nil
		}
		if args[0] == "branch" || args[0] == "-C" {
			return "", nil
		}
		return "", fmt.Errorf("unexpected git call: %v", args)
	}
	return run, &calls
}

func prBehindResponses() map[string]string {
	return map[string]string{
		"fetch origin feature/x":                          "",
		"rev-parse --verify --quiet refs/heads/feature/x": "aaa",
		"rev-parse --verify --quiet origin/feature/x":     "bbb",
		"rev-list --count origin/feature/x..feature/x":    "0",
		"rev-list --count feature/x..origin/feature/x":    "2",
	}
}

func TestPreparePRBranchFastForwardsPlainBranchAfterConsent(t *testing.T) {
	run, calls := prFakeRunner(prBehindResponses())
	confirmed := 0

	ok, err := preparePRBranchWith(run, "feature/x", "", func() bool { confirmed++; return true })
	if err != nil {
		t.Fatalf("preparePRBranchWith() error = %v", err)
	}
	if !ok {
		t.Fatal("preparePRBranchWith() = false, want ready to switch")
	}
	if confirmed != 1 {
		t.Fatalf("confirm called %d times, want 1", confirmed)
	}
	want := [][]string{
		{"fetch", "origin", "feature/x"},
		{"rev-parse", "--verify", "--quiet", "refs/heads/feature/x"},
		{"rev-parse", "--verify", "--quiet", "origin/feature/x"},
		{"rev-list", "--count", "origin/feature/x..feature/x"},
		{"rev-list", "--count", "feature/x..origin/feature/x"},
		{"branch", "-f", "feature/x", "origin/feature/x"},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
	}
}

func TestPreparePRBranchFastForwardsWorktreeWithMerge(t *testing.T) {
	run, calls := prFakeRunner(prBehindResponses())
	ok, err := preparePRBranchWith(run, "feature/x", "/repo/proj-x", func() bool { return true })
	if err != nil || !ok {
		t.Fatalf("preparePRBranchWith() = %v, %v, want true, nil", ok, err)
	}
	wantLast := []string{"-C", "/repo/proj-x", "merge", "--ff-only", "origin/feature/x"}
	if got := (*calls)[len(*calls)-1]; !reflect.DeepEqual(got, wantLast) {
		t.Fatalf("last git call = %v, want %v", got, wantLast)
	}
}

func TestPreparePRBranchDeclinedDoesNotMutate(t *testing.T) {
	run, calls := prFakeRunner(prBehindResponses())
	ok, err := preparePRBranchWith(run, "feature/x", "", func() bool { return false })
	if err != nil {
		t.Fatalf("preparePRBranchWith() error = %v", err)
	}
	if ok {
		t.Fatal("preparePRBranchWith() = true after decline, want false")
	}
	for _, call := range *calls {
		if call[0] == "branch" || call[0] == "merge" || call[0] == "-C" {
			t.Fatalf("unexpected mutation after decline: %v", call)
		}
	}
}

func TestPreparePRBranchRejectsDivergedBranchWithoutAsking(t *testing.T) {
	responses := prBehindResponses()
	responses["rev-list --count origin/feature/x..feature/x"] = "1"
	run, _ := prFakeRunner(responses)
	asked := false

	ok, err := preparePRBranchWith(run, "feature/x", "", func() bool { asked = true; return true })
	if err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("preparePRBranchWith() error = %v, want diverged error", err)
	}
	if ok || asked {
		t.Fatalf("preparePRBranchWith() = %v, confirm asked = %v, want false/false", ok, asked)
	}
}

func TestPreparePRBranchCreatesMissingBranchFromRemoteTip(t *testing.T) {
	responses := prBehindResponses()
	delete(responses, "rev-parse --verify --quiet refs/heads/feature/x")
	run, calls := prFakeRunner(responses)
	asked := false

	ok, err := preparePRBranchWith(run, "feature/x", "", func() bool { asked = true; return true })
	if err != nil || !ok {
		t.Fatalf("preparePRBranchWith() = %v, %v, want true, nil", ok, err)
	}
	if asked {
		t.Fatal("missing branch should be created without asking")
	}
	wantLast := []string{"branch", "feature/x", "origin/feature/x"}
	if got := (*calls)[len(*calls)-1]; !reflect.DeepEqual(got, wantLast) {
		t.Fatalf("last git call = %v, want %v", got, wantLast)
	}
}

func TestPreparePRBranchUpToDateRunsNoMutation(t *testing.T) {
	responses := prBehindResponses()
	responses["rev-parse --verify --quiet origin/feature/x"] = "aaa"
	run, calls := prFakeRunner(responses)

	ok, err := preparePRBranchWith(run, "feature/x", "", func() bool { return true })
	if err != nil || !ok {
		t.Fatalf("preparePRBranchWith() = %v, %v, want true, nil", ok, err)
	}
	if len(*calls) != 3 {
		t.Fatalf("git calls = %v, want only fetch and two rev-parse checks", *calls)
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

func TestForceUpdateWithFetchesOnceThenResetsWorktreeAndMovesPlainBranch(t *testing.T) {
	run, calls := recordingRunner()
	items := []gitdata.Item{
		{Branch: "feature/login", Path: "/repo/proj-login"},
		{Branch: "bugfix/api-timeout"},
	}

	if err := forceUpdateWith(run, items); err != nil {
		t.Fatalf("forceUpdateWith() error = %v", err)
	}
	want := [][]string{
		{"fetch", "origin"},
		{"-C", "/repo/proj-login", "reset", "--hard", "origin/feature/login"},
		{"branch", "-f", "bugfix/api-timeout", "origin/bugfix/api-timeout"},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("git calls = %v, want %v", *calls, want)
	}
}

func TestForceUpdateWithDetachedOnlyRunsNoGit(t *testing.T) {
	run, calls := recordingRunner()
	item := gitdata.Item{Branch: "(detached)", Path: "/repo/proj-scratch", Detached: true}

	if err := forceUpdateWith(run, []gitdata.Item{item}); err != nil {
		t.Fatalf("forceUpdateWith() error = %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("git calls = %v, want none for a detached worktree", *calls)
	}
}

func TestForceUpdateWithFetchFailureStopsBeforeAnyReset(t *testing.T) {
	run := func(args ...string) error {
		if args[0] == "fetch" {
			return fmt.Errorf("no remote")
		}
		t.Fatalf("unexpected git call after failed fetch: %v", args)
		return nil
	}
	item := gitdata.Item{Branch: "feature/login", Path: "/repo/proj-login"}

	err := forceUpdateWith(run, []gitdata.Item{item})
	if err == nil || !strings.Contains(err.Error(), "fetch origin") {
		t.Fatalf("forceUpdateWith() error = %v, want fetch origin error", err)
	}
}

func TestForceUpdateWithContinuesAfterFailureAndCombinesErrors(t *testing.T) {
	run := func(args ...string) error {
		if args[len(args)-1] == "origin/feature/login" {
			return fmt.Errorf("boom")
		}
		return nil
	}
	items := []gitdata.Item{
		{Branch: "feature/login", Path: "/repo/proj-login"},
		{Branch: "bugfix/api-timeout"},
	}

	err := forceUpdateWith(run, items)
	if err == nil {
		t.Fatal("forceUpdateWith() error = nil, want combined error from the failing item")
	}
	if !strings.Contains(err.Error(), "feature/login") {
		t.Fatalf("error = %v, want it to mention feature/login", err)
	}
}
