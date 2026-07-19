// Package ui implements the interactive picker. Update is a pure state
// transition function; no git command or filesystem write happens here.
package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AgileIndustrialComplex/wt/internal/config"
	"github.com/AgileIndustrialComplex/wt/internal/gitdata"
)

type mode int

const (
	modeList mode = iota
	modeFilter
	modeResolve
	modeLocked
	modeHelp
)

// Result is what the picker produced when the program exited.
type Result struct {
	Cancelled       bool
	Item            gitdata.Item
	Resolution      string // config.ActionSwitch or config.ActionWorktree, set only when Item has no worktree
	NewWorktreePath string // suggested path, set only when Resolution == config.ActionWorktree
}

// Model is the bubbletea model for the picker.
type Model struct {
	items         []gitdata.Item
	filtered      []int
	cursor        int
	filter        string
	mode          mode
	toplevel      string
	worktreeRoot  string
	noColor       bool
	defaultAction string
	keymap        string
	height        int

	result   Result
	quitting bool
}

// New builds a Model from the collected items. toplevel is the current
// repository's top-level directory, used to compute the current marker and
// to propose new worktree paths. worktreeRoot overrides where new worktrees
// are proposed; if empty, new worktrees are proposed as siblings of toplevel.
func New(items []gitdata.Item, toplevel string, worktreeRoot string, noColor bool) Model {
	return NewConfigured(items, toplevel, worktreeRoot, noColor, config.ActionPrompt, config.KeymapVim)
}

func NewConfigured(items []gitdata.Item, toplevel string, worktreeRoot string, noColor bool, defaultAction, keymap string) Model {
	m := Model{
		items:         items,
		toplevel:      toplevel,
		worktreeRoot:  worktreeRoot,
		noColor:       noColor,
		defaultAction: defaultAction,
		keymap:        keymap,
	}
	m.applyFilter()
	return m
}

// Result returns the outcome after the program has exited.
func (m Model) Result() Result {
	return m.result
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = size.Height
		return m, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch m.mode {
	case modeHelp:
		return m.updateHelp(keyMsg)
	case modeFilter:
		return m.updateFilter(keyMsg)
	case modeResolve:
		return m.updateResolve(keyMsg)
	case modeLocked:
		return m.updateLocked(keyMsg)
	default:
		return m.updateList(keyMsg)
	}
}

func (m Model) updateLocked(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isCancel(msg) {
		m.mode = modeList
		return m, nil
	}
	if msg.Type == tea.KeyEnter {
		if item := m.selected(); item != nil {
			m.result = Result{Item: *item}
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isCancel(msg) {
		m.result = Result{Cancelled: true}
		m.quitting = true
		return m, tea.Quit
	}
	m.mode = modeList
	return m, nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyEsc:
		m.filter = ""
		m.mode = modeList
		m.applyFilter()
		return m, nil
	case msg.Type == tea.KeyEnter:
		m.mode = modeList
		return m, nil
	case msg.Type == tea.KeyBackspace:
		if len(m.filter) > 0 {
			r := []rune(m.filter)
			m.filter = string(r[:len(r)-1])
			m.applyFilter()
		}
		return m, nil
	case msg.Type == tea.KeyCtrlC:
		m.result = Result{Cancelled: true}
		m.quitting = true
		return m, tea.Quit
	case msg.Type == tea.KeyRunes:
		m.filter += string(msg.Runes)
		m.applyFilter()
		return m, nil
	}
	return m, nil
}

func (m Model) updateResolve(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isCancel(msg) {
		m.mode = modeList
		return m, nil
	}
	item := m.selected()
	if item == nil {
		m.mode = modeList
		return m, nil
	}
	switch msg.String() {
	case "s":
		m.result = Result{Item: *item, Resolution: config.ActionSwitch}
		m.quitting = true
		return m, tea.Quit
	case "w":
		m.result = Result{
			Item:            *item,
			Resolution:      config.ActionWorktree,
			NewWorktreePath: m.proposeWorktreePath(item.Branch),
		}
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isCancel(msg):
		m.result = Result{Cancelled: true}
		m.quitting = true
		return m, tea.Quit

	case isDown(msg):
		if len(m.filtered) > 0 {
			m.cursor = (m.cursor + 1) % len(m.filtered)
		}
		return m, nil

	case isUp(msg):
		if len(m.filtered) > 0 {
			m.cursor = (m.cursor - 1 + len(m.filtered)) % len(m.filtered)
		}
		return m, nil

	case msg.Type == tea.KeyCtrlD:
		m.movePage(5)
		return m, nil

	case msg.Type == tea.KeyCtrlU:
		m.movePage(-5)
		return m, nil

	case msg.String() == "g":
		m.cursor = 0
		return m, nil

	case msg.String() == "G":
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
		return m, nil

	case msg.String() == "/":
		m.mode = modeFilter
		return m, nil

	case msg.String() == "?":
		m.mode = modeHelp
		return m, nil

	case msg.Type == tea.KeyEnter:
		item := m.selected()
		if item == nil {
			return m, nil
		}
		if item.HasWorktree() {
			if item.Locked {
				m.mode = modeLocked
				return m, nil
			}
			m.result = Result{Item: *item}
			m.quitting = true
			return m, tea.Quit
		}
		switch m.defaultAction {
		case config.ActionSwitch:
			m.result = Result{Item: *item, Resolution: config.ActionSwitch}
			m.quitting = true
			return m, tea.Quit
		case config.ActionWorktree:
			m.result = Result{Item: *item, Resolution: config.ActionWorktree, NewWorktreePath: m.proposeWorktreePath(item.Branch)}
			m.quitting = true
			return m, tea.Quit
		default:
			m.mode = modeResolve
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) movePage(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
}

func (m *Model) applyFilter() {
	m.filtered = m.filtered[:0]
	for i, item := range m.items {
		if m.filter == "" || strings.Contains(strings.ToLower(item.Branch), strings.ToLower(m.filter)) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
}

func (m Model) selected() *gitdata.Item {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return &m.items[m.filtered[m.cursor]]
}

// proposeWorktreePath suggests a sibling directory for a new worktree, named
// after the repository directory plus the branch's last path segment, e.g.
// branch "feature/login" next to "~/proj" proposes "~/proj-login".
func (m Model) proposeWorktreePath(branch string) string {
	segment := branch
	if i := strings.LastIndex(branch, "/"); i >= 0 {
		segment = branch[i+1:]
	}
	if m.worktreeRoot != "" {
		return filepath.Join(m.worktreeRoot, filepath.Base(m.toplevel)+"-"+segment)
	}
	parent := filepath.Dir(m.toplevel)
	return filepath.Join(parent, filepath.Base(m.toplevel)+"-"+segment)
}

func isCancel(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		return true
	}
	return msg.String() == "q"
}

func isDown(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyDown, tea.KeyCtrlN:
		return true
	}
	return msg.String() == "j"
}

func isUp(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		return true
	}
	return msg.String() == "k"
}

var (
	selectedStyle = lipgloss.NewStyle().Bold(true)
	dimStyle      = lipgloss.NewStyle().Faint(true)
)

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	switch m.mode {
	case modeHelp:
		return m.helpView()
	case modeResolve:
		item := m.selected()
		if item != nil {
			path := m.proposeWorktreePath(item.Branch)
			fmt.Fprintf(&b, "'%s' has no worktree. [s]witch here  [w]orktree at %s  [Esc] cancel\n", item.Branch, path)
		}
		return b.String()
	case modeLocked:
		item := m.selected()
		if item != nil {
			fmt.Fprintf(&b, "Worktree at %s is locked. [Enter] continue  [Esc] cancel\n", item.Path)
		}
		return b.String()
	}

	if m.mode == modeFilter {
		fmt.Fprintf(&b, "Filter> %s\n", m.filter)
	} else {
		fmt.Fprintf(&b, "Select branch or worktree (%s, / to filter, ? for help)\n", m.navigationHint())
	}

	start, end := m.visibleRange()
	for i := start; i < end; i++ {
		idx := m.filtered[i]
		item := m.items[idx]
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		mark := "○"
		if item.IsCurrent {
			mark = "●"
		}
		line := fmt.Sprintf("%s %s %-28s", cursor, mark, item.Branch)
		if item.HasWorktree() {
			tag := "[worktree]"
			if item.Locked {
				tag = "[worktree, locked]"
			}
			if i == m.cursor {
				line += fmt.Sprintf(" %-20s %s", tag, item.Path)
			} else {
				line += fmt.Sprintf(" %s", tag)
			}
		} else if m.noColor {
			line += " (no worktree)"
		} else {
			line += dimStyle.Render(" (no worktree)")
		}
		if item.IsCurrent {
			line += " (current)"
		}
		if !m.noColor && i == m.cursor {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) visibleRange() (int, int) {
	end := len(m.filtered)
	if m.height == 0 || end <= m.height-1 {
		return 0, end
	}
	if m.height == 1 {
		return 0, 0
	}
	rows := m.height - 1
	start := m.cursor - rows + 1
	if start < 0 {
		start = 0
	}
	if start+rows > end {
		start = end - rows
	}
	return start, start + rows
}

func (m Model) helpView() string {
	navigation := m.navigationHint()
	return strings.Join([]string{
		"Key bindings:",
		"  navigation : " + navigation,
		"  page down  : Ctrl-D",
		"  page up    : Ctrl-U",
		"  top/bottom : g / G",
		"  filter     : /  (Esc clears)",
		"  confirm    : Enter",
		"  cancel     : Esc, Ctrl-C, q",
		"  help       : ?",
		"",
		"press any key to return",
	}, "\n")
}

func (m Model) navigationHint() string {
	switch m.keymap {
	case config.KeymapEmacs:
		return "Ctrl-N/Ctrl-P"
	case config.KeymapArrowsOnly:
		return "Up/Down"
	default:
		return "j/k"
	}
}
