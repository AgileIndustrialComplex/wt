// Integration tests that exercise gitdata, the interactive picker, and
// action execution together against real temporary git repositories. The
// interactive picker's key handling is driven through teatest, which runs
// the real bubbletea Program headlessly (no controlling terminal required);
// its pure key-handling logic is additionally covered directly in
// internal/ui's unit tests.
package wt_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/AgileIndustrialComplex/wt/internal/action"
	"github.com/AgileIndustrialComplex/wt/internal/gitdata"
	"github.com/AgileIndustrialComplex/wt/internal/ui"
)

// chdir switches the process working directory to dir for the duration of
// the test and restores it afterward.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatal(err)
		}
	})
}

func initRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "wt-test@example.com")
	runGit(t, dir, "config", "user.name", "wt test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	runGit(t, dir, "commit", "--allow-empty", "-m", "initial commit")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// enter sends Enter and s/w characters through the picker to reach a
// terminal (quitting) state, then returns the result.
func drivePicker(t *testing.T, m ui.Model, keys ...tea.KeyMsg) ui.Result {
	t.Helper()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	for _, k := range keys {
		tm.Send(k)
	}
	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second))
	return final.(ui.Model).Result()
}

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

var (
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyDown  = keyRune('j')
)

// applyResult mirrors cmd/wt's dispatch: pick the git operation implied by
// the picker's result and execute it for real.
func applyResult(t *testing.T, result ui.Result) string {
	t.Helper()
	if result.Cancelled {
		t.Fatal("picker was cancelled, want a selection")
	}
	if len(result.Delete) > 0 {
		if err := action.DeleteWorktrees(result.Delete); err != nil {
			t.Fatalf("action.DeleteWorktrees: %v", err)
		}
		return ""
	}
	if len(result.ForceUpdate) > 0 {
		if err := action.ForceUpdate(result.ForceUpdate); err != nil {
			t.Fatalf("action.ForceUpdate: %v", err)
		}
		return ""
	}
	if result.Item.HasWorktree() {
		dest, err := action.Switch(result.Item)
		if err != nil {
			t.Fatalf("action.Switch: %v", err)
		}
		return dest
	}
	switch result.Resolution {
	case "worktree":
		dest, err := action.NewWorktree(result.NewWorktreePath, result.Item.Branch)
		if err != nil {
			t.Fatalf("action.NewWorktree: %v", err)
		}
		return dest
	default:
		dest, err := action.Switch(result.Item)
		if err != nil {
			t.Fatalf("action.Switch: %v", err)
		}
		return dest
	}
}

func TestIntegrationSwitchPlainBranchInPlace(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	runGit(t, repo, "branch", "alpha") // plain branch, listed after current worktree
	chdir(t, repo)

	items, err := gitdata.Collect()
	if err != nil {
		t.Fatalf("gitdata.Collect: %v", err)
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		t.Fatalf("gitdata.Toplevel: %v", err)
	}

	m := ui.New(items, top, "", true)
	result := drivePicker(t, m, keyDown, keyEnter, keyRune('s'))

	dest := applyResult(t, result)
	if dest != "" {
		t.Fatalf("dest = %q, want empty (in-place switch)", dest)
	}

	head := strings.TrimSpace(runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	if head != "alpha" {
		t.Fatalf("HEAD = %q, want alpha", head)
	}
}

func TestIntegrationSwitchExistingWorktree(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	wtDir := filepath.Join(t.TempDir(), "zeta-wt-checkout")
	runGit(t, repo, "worktree", "add", "-b", "zeta-wt", wtDir)
	chdir(t, repo)

	items, err := gitdata.Collect()
	if err != nil {
		t.Fatalf("gitdata.Collect: %v", err)
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		t.Fatalf("gitdata.Toplevel: %v", err)
	}

	want := worktreePathForBranch(t, repo, "zeta-wt")

	// order: main (cursor 0), zeta-wt (cursor 1) -> one "down" then Enter,
	// which selects immediately since the item already has a worktree.
	m := ui.New(items, top, "", true)
	result := drivePicker(t, m, keyDown, keyEnter)

	dest := applyResult(t, result)
	if dest != want {
		t.Fatalf("dest = %q, want worktree path %q", dest, want)
	}

	head := strings.TrimSpace(runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	if head != "main" {
		t.Fatalf("HEAD in original checkout = %q, want unchanged (main)", head)
	}
}

func TestIntegrationCreateWorktreeForBranch(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	runGit(t, repo, "branch", "alpha")
	chdir(t, repo)

	items, err := gitdata.Collect()
	if err != nil {
		t.Fatalf("gitdata.Collect: %v", err)
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		t.Fatalf("gitdata.Toplevel: %v", err)
	}

	m := ui.New(items, top, "", true)
	result := drivePicker(t, m, keyDown, keyEnter, keyRune('w'))

	dest := applyResult(t, result)
	if dest == "" {
		t.Fatal("dest is empty, want new worktree path")
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("proposed worktree path %q does not exist: %v", dest, err)
	}

	want := worktreePathForBranch(t, repo, "alpha")
	if filepath.Clean(dest) != filepath.Clean(want) {
		t.Fatalf("dest = %q, want %q (matches git worktree list)", dest, want)
	}
}

func TestIntegrationDeleteMarkedWorktree(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	wtDir := filepath.Join(t.TempDir(), "zeta-wt-checkout")
	runGit(t, repo, "worktree", "add", "-b", "zeta-wt", wtDir)
	chdir(t, repo)

	items, err := gitdata.Collect()
	if err != nil {
		t.Fatalf("gitdata.Collect: %v", err)
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		t.Fatalf("gitdata.Toplevel: %v", err)
	}

	// order: main (cursor 0), zeta-wt (cursor 1) -> down, mark, D, Enter.
	m := ui.New(items, top, "", true)
	result := drivePicker(t, m, keyDown, keyRune('c'), keyRune('D'), keyEnter)

	if len(result.Delete) != 1 || result.Delete[0].Branch != "zeta-wt" {
		t.Fatalf("result.Delete = %+v, want [zeta-wt]", result.Delete)
	}
	applyResult(t, result)

	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Fatalf("worktree dir %q still exists after delete", wtDir)
	}
	branches := runGit(t, repo, "branch", "--list", "zeta-wt")
	if strings.TrimSpace(branches) != "" {
		t.Fatalf("branch zeta-wt still exists after delete: %q", branches)
	}
}

func TestIntegrationDeleteMarkedBranchWithoutWorktree(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	runGit(t, repo, "branch", "alpha") // plain branch, no worktree
	chdir(t, repo)

	items, err := gitdata.Collect()
	if err != nil {
		t.Fatalf("gitdata.Collect: %v", err)
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		t.Fatalf("gitdata.Toplevel: %v", err)
	}

	// order: main (cursor 0), alpha (cursor 1) -> down, mark, D, Enter.
	m := ui.New(items, top, "", true)
	result := drivePicker(t, m, keyDown, keyRune('c'), keyRune('D'), keyEnter)

	if len(result.Delete) != 1 || result.Delete[0].Branch != "alpha" {
		t.Fatalf("result.Delete = %+v, want [alpha]", result.Delete)
	}
	applyResult(t, result)

	branches := runGit(t, repo, "branch", "--list", "alpha")
	if strings.TrimSpace(branches) != "" {
		t.Fatalf("branch alpha still exists after delete: %q", branches)
	}
}

func TestIntegrationDeleteUnmergedBranchRequiresForcePhrase(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	runGit(t, repo, "branch", "wip")
	runGit(t, repo, "checkout", "wip")
	runGit(t, repo, "commit", "--allow-empty", "-m", "unmerged work")
	runGit(t, repo, "checkout", "main")
	chdir(t, repo)

	items, err := gitdata.Collect()
	if err != nil {
		t.Fatalf("gitdata.Collect: %v", err)
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		t.Fatalf("gitdata.Toplevel: %v", err)
	}

	var wip *gitdata.Item
	for i := range items {
		if items[i].Branch == "wip" {
			wip = &items[i]
		}
	}
	if wip == nil || !wip.Unmerged {
		t.Fatalf("items = %+v, want a wip branch marked Unmerged", items)
	}

	// order: main (cursor 0), wip (cursor 1) -> down, mark, D, Enter (opens
	// the force-delete phrase prompt), type the required phrase, Enter.
	keys := []tea.KeyMsg{keyDown, keyRune('c'), keyRune('D'), keyEnter}
	for _, r := range ui.ForceDeleteConfirmPhrase {
		keys = append(keys, keyRune(r))
	}
	keys = append(keys, keyEnter)

	m := ui.New(items, top, "", true)
	result := drivePicker(t, m, keys...)

	if len(result.Delete) != 1 || result.Delete[0].Branch != "wip" {
		t.Fatalf("result.Delete = %+v, want [wip]", result.Delete)
	}
	applyResult(t, result)

	branches := runGit(t, repo, "branch", "--list", "wip")
	if strings.TrimSpace(branches) != "" {
		t.Fatalf("branch wip still exists after force delete: %q", branches)
	}
}

func TestIntegrationDeleteDirtyWorktreeRequiresForcePhrase(t *testing.T) {
	repo := t.TempDir()
	initRepo(t, repo)
	wtDir := filepath.Join(t.TempDir(), "scratch-wt-checkout")
	runGit(t, repo, "worktree", "add", "-b", "scratch", wtDir)
	if err := os.WriteFile(filepath.Join(wtDir, "untracked.txt"), []byte("scratch"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	chdir(t, repo)

	items, err := gitdata.Collect()
	if err != nil {
		t.Fatalf("gitdata.Collect: %v", err)
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		t.Fatalf("gitdata.Toplevel: %v", err)
	}

	var scratch *gitdata.Item
	for i := range items {
		if items[i].Branch == "scratch" {
			scratch = &items[i]
		}
	}
	if scratch == nil || !scratch.Dirty {
		t.Fatalf("items = %+v, want a scratch branch marked Dirty", items)
	}

	// order: main (cursor 0), scratch (cursor 1) -> down, mark, D, Enter
	// (opens the force-delete phrase prompt), type the required phrase, Enter.
	keys := []tea.KeyMsg{keyDown, keyRune('c'), keyRune('D'), keyEnter}
	for _, r := range ui.ForceDeleteConfirmPhrase {
		keys = append(keys, keyRune(r))
	}
	keys = append(keys, keyEnter)

	m := ui.New(items, top, "", true)
	result := drivePicker(t, m, keys...)

	if len(result.Delete) != 1 || result.Delete[0].Branch != "scratch" {
		t.Fatalf("result.Delete = %+v, want [scratch]", result.Delete)
	}
	applyResult(t, result)

	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Fatalf("worktree dir %q still exists after force delete", wtDir)
	}
	branches := runGit(t, repo, "branch", "--list", "scratch")
	if strings.TrimSpace(branches) != "" {
		t.Fatalf("branch scratch still exists after force delete: %q", branches)
	}
}

// initRepoWithRemote initialises repo and pushes its initial commit to a
// fresh bare origin, returning the origin path.
func initRepoWithRemote(t *testing.T, repo string) string {
	t.Helper()
	initRepo(t, repo)
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "origin.git")
	runGit(t, tmp, "init", "--bare", "-b", "main", remote)
	runGit(t, repo, "remote", "add", "origin", remote)
	runGit(t, repo, "push", "-u", "origin", "main")
	return remote
}

// cloneOrigin clones remote into a fresh directory and configures a committer.
func cloneOrigin(t *testing.T, remote string) string {
	t.Helper()
	parent := t.TempDir()
	clone := filepath.Join(parent, "clone")
	runGit(t, parent, "clone", remote, clone)
	runGit(t, clone, "config", "user.email", "wt-test@example.com")
	runGit(t, clone, "config", "user.name", "wt test")
	runGit(t, clone, "config", "commit.gpgsign", "false")
	return clone
}

func TestIntegrationForceUpdatePlainBranchToRemoteTip(t *testing.T) {
	repo := t.TempDir()
	remote := initRepoWithRemote(t, repo)

	// Advance origin/main from a second clone so the local repo is behind.
	other := cloneOrigin(t, remote)
	runGit(t, other, "commit", "--allow-empty", "-m", "remote commit")
	runGit(t, other, "push", "origin", "main")
	want := strings.TrimSpace(runGit(t, other, "rev-parse", "main"))

	chdir(t, repo)
	items, err := gitdata.Collect()
	if err != nil {
		t.Fatalf("gitdata.Collect: %v", err)
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		t.Fatalf("gitdata.Toplevel: %v", err)
	}

	// main is highlighted at cursor 0: F then Enter confirms the force update.
	m := ui.New(items, top, "", true)
	result := drivePicker(t, m, keyRune('F'), keyEnter)
	if len(result.ForceUpdate) != 1 || result.ForceUpdate[0].Branch != "main" {
		t.Fatalf("result.ForceUpdate = %+v, want [main]", result.ForceUpdate)
	}
	applyResult(t, result)

	if got := strings.TrimSpace(runGit(t, repo, "rev-parse", "main")); got != want {
		t.Fatalf("main = %s, want remote tip %s", got, want)
	}
}

func TestIntegrationForceUpdateWorktreeBranchToRemoteTip(t *testing.T) {
	repo := t.TempDir()
	remote := initRepoWithRemote(t, repo)
	runGit(t, repo, "branch", "feature/x")
	runGit(t, repo, "push", "origin", "feature/x")

	// Advance origin/feature/x from a second clone.
	other := cloneOrigin(t, remote)
	runGit(t, other, "checkout", "-b", "feature/x", "origin/feature/x")
	runGit(t, other, "commit", "--allow-empty", "-m", "remote feature commit")
	runGit(t, other, "push", "origin", "feature/x")
	want := strings.TrimSpace(runGit(t, other, "rev-parse", "feature/x"))

	// The local worktree diverges with its own commit.
	wtDir := filepath.Join(t.TempDir(), "feature-x-checkout")
	runGit(t, repo, "worktree", "add", wtDir, "feature/x")
	runGit(t, wtDir, "commit", "--allow-empty", "-m", "local divergence")
	chdir(t, repo)

	items, err := gitdata.Collect()
	if err != nil {
		t.Fatalf("gitdata.Collect: %v", err)
	}
	top, err := gitdata.Toplevel()
	if err != nil {
		t.Fatalf("gitdata.Toplevel: %v", err)
	}

	// order: main (cursor 0, current), feature/x (cursor 1) -> down, mark, F, Enter.
	m := ui.New(items, top, "", true)
	result := drivePicker(t, m, keyDown, keyRune('c'), keyRune('F'), keyEnter)
	if len(result.ForceUpdate) != 1 || result.ForceUpdate[0].Branch != "feature/x" {
		t.Fatalf("result.ForceUpdate = %+v, want [feature/x]", result.ForceUpdate)
	}
	applyResult(t, result)

	if got := strings.TrimSpace(runGit(t, wtDir, "rev-parse", "HEAD")); got != want {
		t.Fatalf("feature/x worktree HEAD = %s, want remote tip %s", got, want)
	}
}

// initPRRemote initialises repo with a bare origin and pushes its first
// commit. Names are distinct from other PRs' helpers to keep branches
// independent until they merge.
func initPRRemote(t *testing.T, repo string) string {
	t.Helper()
	initRepo(t, repo)
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "origin.git")
	runGit(t, tmp, "init", "--bare", "-b", "main", remote)
	runGit(t, repo, "remote", "add", "origin", remote)
	runGit(t, repo, "push", "-u", "origin", "main")
	return remote
}

func clonePRRemote(t *testing.T, remote string) string {
	t.Helper()
	parent := t.TempDir()
	clone := filepath.Join(parent, "clone")
	runGit(t, parent, "clone", remote, clone)
	runGit(t, clone, "config", "user.email", "wt-test@example.com")
	runGit(t, clone, "config", "user.name", "wt test")
	runGit(t, clone, "config", "commit.gpgsign", "false")
	return clone
}

// advancedPRBranch pushes a new commit on branch from a second clone and
// returns the resulting remote tip.
func advancedPRBranch(t *testing.T, remote, branch string) string {
	t.Helper()
	other := clonePRRemote(t, remote)
	runGit(t, other, "checkout", "-b", branch, "origin/"+branch)
	runGit(t, other, "commit", "--allow-empty", "-m", "remote commit")
	runGit(t, other, "push", "origin", branch)
	return strings.TrimSpace(runGit(t, other, "rev-parse", branch))
}

func TestIntegrationPreparePRBranchFastForwardsPlainBranch(t *testing.T) {
	repo := t.TempDir()
	remote := initPRRemote(t, repo)
	runGit(t, repo, "branch", "feature/x")
	runGit(t, repo, "push", "origin", "feature/x")
	want := advancedPRBranch(t, remote, "feature/x")
	chdir(t, repo)

	ok, err := action.PreparePRBranch("feature/x", "", func() bool { return true })
	if err != nil || !ok {
		t.Fatalf("PreparePRBranch() = %v, %v, want true, nil", ok, err)
	}
	if got := strings.TrimSpace(runGit(t, repo, "rev-parse", "feature/x")); got != want {
		t.Fatalf("feature/x = %s, want remote tip %s", got, want)
	}
}

func TestIntegrationPreparePRBranchDeclinedLeavesBranchAlone(t *testing.T) {
	repo := t.TempDir()
	remote := initPRRemote(t, repo)
	runGit(t, repo, "branch", "feature/x")
	runGit(t, repo, "push", "origin", "feature/x")
	before := strings.TrimSpace(runGit(t, repo, "rev-parse", "feature/x"))
	advancedPRBranch(t, remote, "feature/x")
	chdir(t, repo)

	asked := false
	ok, err := action.PreparePRBranch("feature/x", "", func() bool { asked = true; return false })
	if err != nil {
		t.Fatalf("PreparePRBranch() error = %v", err)
	}
	if ok || !asked {
		t.Fatalf("PreparePRBranch() = %v, asked = %v, want false/true after decline", ok, asked)
	}
	if got := strings.TrimSpace(runGit(t, repo, "rev-parse", "feature/x")); got != before {
		t.Fatalf("feature/x = %s after decline, want unchanged %s", got, before)
	}
}

func TestIntegrationPreparePRBranchRejectsDivergedBranch(t *testing.T) {
	repo := t.TempDir()
	remote := initPRRemote(t, repo)
	runGit(t, repo, "branch", "feature/x")
	runGit(t, repo, "push", "origin", "feature/x")
	runGit(t, repo, "checkout", "feature/x")
	runGit(t, repo, "commit", "--allow-empty", "-m", "local commit")
	runGit(t, repo, "checkout", "main")
	before := strings.TrimSpace(runGit(t, repo, "rev-parse", "feature/x"))
	advancedPRBranch(t, remote, "feature/x")
	chdir(t, repo)

	ok, err := action.PreparePRBranch("feature/x", "", func() bool { return true })
	if err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("PreparePRBranch() error = %v, want diverged error", err)
	}
	if ok {
		t.Fatal("PreparePRBranch() = true for a diverged branch, want false")
	}
	if got := strings.TrimSpace(runGit(t, repo, "rev-parse", "feature/x")); got != before {
		t.Fatalf("feature/x = %s after rejected fast-forward, want unchanged %s", got, before)
	}
}

func TestIntegrationPreparePRBranchCreatesMissingBranch(t *testing.T) {
	repo := t.TempDir()
	remote := initPRRemote(t, repo)
	other := clonePRRemote(t, remote)
	runGit(t, other, "checkout", "-b", "feature/new")
	runGit(t, other, "commit", "--allow-empty", "-m", "new PR commit")
	runGit(t, other, "push", "-u", "origin", "feature/new")
	want := strings.TrimSpace(runGit(t, other, "rev-parse", "feature/new"))
	chdir(t, repo)

	if branches := runGit(t, repo, "branch", "--list", "feature/new"); strings.TrimSpace(branches) != "" {
		t.Fatalf("feature/new already exists locally before prepare: %q", branches)
	}
	asked := false
	ok, err := action.PreparePRBranch("feature/new", "", func() bool { asked = true; return true })
	if err != nil || !ok {
		t.Fatalf("PreparePRBranch() = %v, %v, want true, nil", ok, err)
	}
	if asked {
		t.Fatal("creating a missing branch should not ask for a fast-forward")
	}
	if got := strings.TrimSpace(runGit(t, repo, "rev-parse", "feature/new")); got != want {
		t.Fatalf("feature/new = %s, want remote tip %s", got, want)
	}
}

func TestIntegrationPreparePRBranchFastForwardsWorktree(t *testing.T) {
	repo := t.TempDir()
	remote := initPRRemote(t, repo)
	runGit(t, repo, "branch", "feature/x")
	runGit(t, repo, "push", "origin", "feature/x")
	wtDir := filepath.Join(t.TempDir(), "feature-x-checkout")
	runGit(t, repo, "worktree", "add", wtDir, "feature/x")
	want := advancedPRBranch(t, remote, "feature/x")
	chdir(t, repo)

	ok, err := action.PreparePRBranch("feature/x", wtDir, func() bool { return true })
	if err != nil || !ok {
		t.Fatalf("PreparePRBranch() = %v, %v, want true, nil", ok, err)
	}
	if got := strings.TrimSpace(runGit(t, wtDir, "rev-parse", "HEAD")); got != want {
		t.Fatalf("feature/x worktree HEAD = %s, want remote tip %s", got, want)
	}
}

func worktreePathForBranch(t *testing.T, repo, branch string) string {
	t.Helper()
	out := runGit(t, repo, "worktree", "list", "--porcelain")
	var path string
	for _, entry := range strings.Split(out, "\n\n") {
		if !strings.Contains(entry, "branch refs/heads/"+branch) {
			continue
		}
		for _, line := range strings.Split(entry, "\n") {
			if p, ok := strings.CutPrefix(line, "worktree "); ok {
				path = p
			}
		}
	}
	if path == "" {
		t.Fatalf("no worktree found for branch %q in:\n%s", branch, out)
	}
	return path
}
