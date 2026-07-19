package ui

import (
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
	want := "/repo/proj-api-timeout"
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
