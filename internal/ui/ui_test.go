package ui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AgileIndustrialComplex/wt/internal/config"
	"github.com/AgileIndustrialComplex/wt/internal/gitdata"
)

func testItems() []gitdata.Item {
	return []gitdata.Item{
		{Branch: "main", Path: "/repo/proj", IsCurrent: true},
		{Branch: "feature/login", Path: "/repo/proj-login"},
		{Branch: "bugfix/api-timeout"},
		{Branch: "release/2.1", Path: "/repo/proj-release", Locked: true},
	}
}

func key(runes ...rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: runes}
}

func keyType(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func send(t *testing.T, m Model, msgs ...tea.KeyMsg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func TestNavigationDownWraps(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	// j j j j (4 items, wraps back to 0)
	m = send(t, m, key('j'), key('j'), key('j'), key('j'))
	if m.cursor != 0 {
		t.Fatalf("cursor after 4 downs on 4 items = %d, want 0 (wrapped)", m.cursor)
	}
}

func TestNavigationUpWraps(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	m = send(t, m, key('k'))
	if m.cursor != len(testItems())-1 {
		t.Fatalf("cursor after k from top = %d, want %d (wrapped to bottom)", m.cursor, len(testItems())-1)
	}
}

func TestVimAndArrowKeysAgree(t *testing.T) {
	m1 := New(testItems(), "/repo/proj", "", true)
	m1 = send(t, m1, key('j'), key('j'))

	m2 := New(testItems(), "/repo/proj", "", true)
	m2 = send(t, m2, keyType(tea.KeyDown), keyType(tea.KeyDown))

	m3 := New(testItems(), "/repo/proj", "", true)
	m3 = send(t, m3, keyType(tea.KeyCtrlN), keyType(tea.KeyCtrlN))

	if m1.cursor != m2.cursor || m2.cursor != m3.cursor {
		t.Fatalf("j/Down/Ctrl-N disagree: %d, %d, %d", m1.cursor, m2.cursor, m3.cursor)
	}
}

func TestGAndCapitalGJumpToEnds(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	m = send(t, m, key('j'), key('G'))
	if m.cursor != len(testItems())-1 {
		t.Fatalf("cursor after G = %d, want %d", m.cursor, len(testItems())-1)
	}
	m = send(t, m, key('g'))
	if m.cursor != 0 {
		t.Fatalf("cursor after g = %d, want 0", m.cursor)
	}
}

func TestFilterNarrowsList(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	m = send(t, m, key('/'), key('f'), key('e'), key('a'), key('t'), keyType(tea.KeyEnter))
	if len(m.filtered) != 1 || m.items[m.filtered[0]].Branch != "feature/login" {
		t.Fatalf("filter %q -> %v, want only feature/login", m.filter, m.filtered)
	}
}

func TestFilterEscClearsFilter(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	m = send(t, m, key('/'), key('m'), key('a'), key('i'), key('n'), keyType(tea.KeyEsc))
	if m.filter != "" || len(m.filtered) != len(testItems()) {
		t.Fatalf("after Esc: filter=%q filtered=%v, want cleared", m.filter, m.filtered)
	}
}

func TestEnterOnWorktreeItemSelectsImmediately(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true) // cursor starts on "main", which has a worktree
	m = send(t, m, keyType(tea.KeyEnter))
	res := m.Result()
	if res.Cancelled || res.Item.Branch != "main" {
		t.Fatalf("Result() = %+v, want main selected", res)
	}
}

func TestEnterOnBranchWithNoWorktreeEntersResolveMode(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	m = send(t, m, key('j'), key('j'), keyType(tea.KeyEnter)) // -> bugfix/api-timeout
	if m.mode != modeResolve {
		t.Fatalf("mode = %v, want modeResolve", m.mode)
	}
	if m.quitting {
		t.Fatal("entering resolve mode should not quit the program")
	}
}

func TestConfiguredDefaultActionSkipsResolvePrompt(t *testing.T) {
	m := NewConfigured(testItems(), "/repo/proj", "", true, config.ActionWorktree, config.KeymapEmacs)
	m = send(t, m, key('j'), key('j'), keyType(tea.KeyEnter))
	if m.Result().Resolution != config.ActionWorktree || !m.quitting {
		t.Fatalf("Result() = %+v, want immediate worktree resolution", m.Result())
	}
	if got := NewConfigured(testItems(), "/repo/proj", "", true, config.ActionPrompt, config.KeymapEmacs).navigationHint(); got != "Ctrl-N/Ctrl-P" {
		t.Fatalf("navigationHint() = %q", got)
	}
}

func TestLockedWorktreeRequiresConfirmation(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	m = send(t, m, key('G'), keyType(tea.KeyEnter))
	if m.mode != modeLocked || m.quitting {
		t.Fatalf("locked selection was not paused: %+v", m)
	}
	m = send(t, m, keyType(tea.KeyEnter))
	if !m.quitting || m.Result().Item.Branch != "release/2.1" {
		t.Fatalf("locked confirmation result = %+v", m.Result())
	}
}

func TestResolveSwitchHere(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	m = send(t, m, key('j'), key('j'), keyType(tea.KeyEnter), key('s'))
	res := m.Result()
	if res.Item.Branch != "bugfix/api-timeout" || res.Resolution != config.ActionSwitch {
		t.Fatalf("Result() = %+v, want switch resolution for bugfix/api-timeout", res)
	}
}

func TestResolveNewWorktreeProposesSiblingPath(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	m = send(t, m, key('j'), key('j'), keyType(tea.KeyEnter), key('w'))
	res := m.Result()
	if res.Resolution != config.ActionWorktree {
		t.Fatalf("Resolution = %v, want ActionWorktree", res.Resolution)
	}
	want := filepath.Join(string(filepath.Separator), "repo", "proj-api-timeout")
	if res.NewWorktreePath != want {
		t.Fatalf("NewWorktreePath = %q, want %q", res.NewWorktreePath, want)
	}
}

func TestResolveEscCancelsPromptNotProgram(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	m = send(t, m, key('j'), key('j'), keyType(tea.KeyEnter), keyType(tea.KeyEsc))
	if m.mode != modeList {
		t.Fatalf("mode after Esc in resolve = %v, want modeList", m.mode)
	}
	if m.quitting {
		t.Fatal("Esc in resolve mode should not quit the program")
	}
}

func TestCancelKeysQuitWithCancelled(t *testing.T) {
	for _, k := range []tea.KeyMsg{keyType(tea.KeyEsc), keyType(tea.KeyCtrlC), key('q')} {
		m := New(testItems(), "/repo/proj", "", true)
		m = send(t, m, k)
		if !m.Result().Cancelled {
			t.Fatalf("key %v did not set Cancelled", k)
		}
	}
}

func TestHelpOverlayReturnsToList(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	m = send(t, m, key('?'))
	if m.mode != modeHelp {
		t.Fatalf("mode after ? = %v, want modeHelp", m.mode)
	}
	m = send(t, m, key('x'))
	if m.mode != modeList {
		t.Fatalf("mode after key in help = %v, want modeList", m.mode)
	}
}

func TestViewKeepsCursorInsideTerminalHeight(t *testing.T) {
	items := make([]gitdata.Item, 10)
	for i := range items {
		items[i].Branch = fmt.Sprintf("branch-%d", i)
	}
	m := New(items, "/repo/proj", "", true)
	next, _ := m.Update(tea.WindowSizeMsg{Height: 4})
	m = next.(Model)
	m = send(t, m, key('G'))
	view := m.View()
	if strings.Count(view, "\n") != 4 {
		t.Fatalf("View() rendered %d lines, want 4:\n%s", strings.Count(view, "\n"), view)
	}
	if !strings.Contains(view, "> ○ branch-9") || strings.Contains(view, "branch-0") {
		t.Fatalf("View() did not keep cursor visible:\n%s", view)
	}
}

func TestViewShowsDirectoryOnlyForHighlightedItem(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	view := m.View()
	if !strings.Contains(view, "/repo/proj (current)") {
		t.Fatalf("View() missing highlighted item's directory:\n%s", view)
	}
	if strings.Contains(view, "/repo/proj-login") || strings.Contains(view, "/repo/proj-release") {
		t.Fatalf("View() shows directory for non-highlighted item:\n%s", view)
	}

	m = send(t, m, key('j'))
	view = m.View()
	if !strings.Contains(view, "/repo/proj-login") {
		t.Fatalf("View() missing directory after moving highlight:\n%s", view)
	}
	if strings.Contains(view, "/repo/proj (current)") || strings.Contains(view, "/repo/proj-release") {
		t.Fatalf("View() shows directory for non-highlighted item after move:\n%s", view)
	}
}

func TestNoColorViewContainsNoANSISequences(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", true)
	if view := m.View(); strings.Contains(view, "\x1b[") {
		t.Fatalf("no-color View() contains ANSI sequence: %q", view)
	}
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestColorViewMatchesNoColorContentWithEscapesStripped(t *testing.T) {
	colorView := ansiEscape.ReplaceAllString(New(testItems(), "/repo/proj", "", false).View(), "")
	plainView := New(testItems(), "/repo/proj", "", true).View()
	if colorView != plainView {
		t.Fatalf("color view with escapes stripped = %q, want %q", colorView, plainView)
	}
}

func TestColorViewContainsANSISequences(t *testing.T) {
	m := New(testItems(), "/repo/proj", "", false)
	if view := m.View(); !strings.Contains(view, "\x1b[") {
		t.Fatalf("color View() contains no ANSI sequence: %q", view)
	}
}

func TestSelectedStyleResumesAfterNestedStyleReset(t *testing.T) {
	nested := currentStyle.Render("●")
	got := renderSelected("> " + nested + " main")
	want := selectedStyle.Render("> "+nested) + selectedStyle.Render(" main")
	if got != want {
		t.Fatalf("renderSelected() = %q, want %q", got, want)
	}
}
