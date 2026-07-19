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
	result := drivePicker(t, m, keyDown, keyRune('m'), keyRune('D'), keyEnter)

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
