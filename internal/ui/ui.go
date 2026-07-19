// Package ui implements the interactive picker. Update is a pure state
// transition function; no git command or filesystem write happens here.
package ui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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
	modeConfirmDelete
	modeConfirmForceDelete
)

// ForceDeleteConfirmPhrase is the exact text a user must type to delete a
// branch with unmerged changes. It must match verbatim (case-sensitive) —
// this is deliberate friction against losing unmerged work, so it is never
// made configurable.
const ForceDeleteConfirmPhrase = "delete"

// Result is what the picker produced when the program exited.
type Result struct {
	Cancelled       bool
	Item            gitdata.Item
	Resolution      string         // config.ActionSwitch or config.ActionWorktree, set only when Item has no worktree
	NewWorktreePath string         // suggested path, set only when Resolution == config.ActionWorktree
	Delete          []gitdata.Item // set when the user confirmed bulk deletion of the marked branches
}

// Model is the bubbletea model for the picker.
type Model struct {
	items         []gitdata.Item
	filtered      []int
	cursor        int
	marked        map[int]bool // keyed by index into items
	filter        string
	mode          mode
	toplevel      string
	worktreeRoot  string
	noColor       bool
	defaultAction string
	keymap        string
	height        int

	forceConfirmInput string // typed phrase in modeConfirmForceDelete

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
		marked:        make(map[int]bool),
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
	case modeConfirmDelete:
		return m.updateConfirmDelete(keyMsg)
	case modeConfirmForceDelete:
		return m.updateConfirmForceDelete(keyMsg)
	default:
		return m.updateList(keyMsg)
	}
}

// updateConfirmDelete handles the approval step shown after pressing D with
// at least one branch marked. Enter confirms: if any marked branch is
// unmerged, it advances to modeConfirmForceDelete for an explicit
// phrase-typed confirmation instead of quitting immediately. Any cancel key
// stops the operation and returns to the list without quitting the picker.
func (m Model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isCancel(msg) {
		m.mode = modeList
		return m, nil
	}
	if msg.Type == tea.KeyEnter {
		marked := m.Marked()
		if hasUnmergedBranch(marked) {
			m.forceConfirmInput = ""
			m.mode = modeConfirmForceDelete
			return m, nil
		}
		m.result = Result{Delete: marked}
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

// updateConfirmForceDelete handles the phrase-typed approval step shown
// when at least one marked branch has unmerged changes. Esc/Ctrl-C cancel
// back to the list; Enter only confirms and quits (with the full marked set
// in Result.Delete) when the typed text matches ForceDeleteConfirmPhrase
// exactly, otherwise it is a no-op so the user can keep correcting the
// input. "q" is not treated as cancel here since it is valid input text.
func (m Model) updateConfirmForceDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.forceConfirmInput = ""
		m.mode = modeList
		return m, nil
	case tea.KeyEnter:
		if m.forceConfirmInput == ForceDeleteConfirmPhrase {
			m.result = Result{Delete: m.Marked()}
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyBackspace:
		if len(m.forceConfirmInput) > 0 {
			r := []rune(m.forceConfirmInput)
			m.forceConfirmInput = string(r[:len(r)-1])
		}
		return m, nil
	case tea.KeyRunes:
		m.forceConfirmInput += string(msg.Runes)
		return m, nil
	}
	return m, nil
}

// hasUnmergedBranch reports whether any item has unmerged changes on its
// branch. Detached-HEAD entries have no associated branch to delete, so
// they are excluded regardless of their zero-value Unmerged field.
func hasUnmergedBranch(items []gitdata.Item) bool {
	for _, item := range items {
		if item.Unmerged && !item.Detached {
			return true
		}
	}
	return false
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
	switch msg.Type {
	case tea.KeyEsc:
		m.filter = ""
		m.mode = modeList
		m.applyFilter()
		return m, nil
	case tea.KeyEnter:
		m.mode = modeList
		return m, nil
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			r := []rune(m.filter)
			m.filter = string(r[:len(r)-1])
			m.applyFilter()
		}
		return m, nil
	case tea.KeyCtrlC:
		m.result = Result{Cancelled: true}
		m.quitting = true
		return m, tea.Quit
	case tea.KeyRunes:
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

	case msg.String() == "c":
		m.toggleMarked()
		return m, nil

	case msg.String() == "D":
		if len(m.marked) > 0 {
			m.mode = modeConfirmDelete
		}
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

// toggleMarked flips the marked state of the highlighted item. Only
// removable branches can be marked: the current worktree and locked
// worktrees are excluded.
func (m *Model) toggleMarked() {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return
	}
	idx := m.filtered[m.cursor]
	if m.items[idx].IsCurrent || m.items[idx].Locked {
		return
	}
	m.marked[idx] = !m.marked[idx]
	if !m.marked[idx] {
		delete(m.marked, idx)
	}
}

// Marked returns the marked items in list order.
func (m Model) Marked() []gitdata.Item {
	var result []gitdata.Item
	for i, item := range m.items {
		if m.marked[i] {
			result = append(result, item)
		}
	}
	return result
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

// colorRenderer always emits ANSI codes: the noColor field on Model is the
// one place that decides whether styling is applied, so styles must not
// additionally depend on termenv's own TTY auto-detection, which would
// otherwise silently drop colors whenever stderr isn't a terminal (e.g. in
// tests, or when the caller pipes output).
var colorRenderer = func() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	return r
}()

var (
	selectedStyle = colorRenderer.NewStyle().Bold(true).Foreground(lipgloss.Color("6")) // cyan
	dimStyle      = colorRenderer.NewStyle().Faint(true)
	currentStyle  = colorRenderer.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // green
	worktreeStyle = colorRenderer.NewStyle().Foreground(lipgloss.Color("4"))            // blue
	lockedStyle   = colorRenderer.NewStyle().Foreground(lipgloss.Color("3"))            // yellow
	filterStyle   = colorRenderer.NewStyle().Foreground(lipgloss.Color("6")).Bold(true) // cyan
	markedStyle   = colorRenderer.NewStyle().Foreground(lipgloss.Color("5")).Bold(true) // magenta
)

func renderSelected(line string) string {
	parts := strings.SplitAfter(line, "\x1b[0m")
	for i, part := range parts {
		if part != "" {
			parts[i] = selectedStyle.Render(part)
		}
	}
	return strings.Join(parts, "")
}

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
	case modeConfirmDelete:
		marked := m.Marked()
		fmt.Fprintf(&b, "Delete %d branch(es) (and their worktrees, if any)?\n", len(marked))
		for _, item := range marked {
			path := item.Path
			if path == "" {
				path = "(no worktree)"
			}
			fmt.Fprintf(&b, "  %s  %s\n", item.Branch, path)
		}
		b.WriteString("[Enter] confirm  [Esc] cancel\n")
		return b.String()
	case modeConfirmForceDelete:
		b.WriteString("The following branch(es) have unmerged changes and will be permanently lost:\n")
		for _, item := range m.Marked() {
			if item.Unmerged && !item.Detached {
				fmt.Fprintf(&b, "  %s\n", item.Branch)
			}
		}
		fmt.Fprintf(&b, "Type %q and press Enter to proceed, or Esc to cancel:\n\n", ForceDeleteConfirmPhrase)
		fmt.Fprintf(&b, "> %s\n", m.forceConfirmInput)
		return b.String()
	}

	if m.mode == modeFilter {
		prompt := "Filter> "
		if !m.noColor {
			prompt = filterStyle.Render(prompt)
		}
		fmt.Fprintf(&b, "%s%s\n", prompt, m.filter)
	} else {
		fmt.Fprintf(&b, "Select branch or worktree (%s, / to filter, c to mark, ? for help)\n", m.navigationHint())
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
			if !m.noColor {
				mark = currentStyle.Render(mark)
			}
		}
		marker := "[ ]"
		if m.marked[idx] {
			marker = "[x]"
			if !m.noColor {
				marker = markedStyle.Render(marker)
			}
		}
		line := fmt.Sprintf("%s %s %-28s", cursor, mark, item.Branch)
		if len(m.marked) > 0 {
			line = fmt.Sprintf("%s %s %s %-28s", cursor, marker, mark, item.Branch)
		}
		if item.HasWorktree() {
			tag := "[worktree]"
			style := worktreeStyle
			if item.Locked {
				tag = "[worktree, locked]"
				style = lockedStyle
			}
			if i == m.cursor {
				tagField := fmt.Sprintf("%-20s", tag)
				if !m.noColor {
					tagField = style.Render(tagField)
				}
				line += fmt.Sprintf(" %s %s", tagField, item.Path)
			} else {
				tagField := tag
				if !m.noColor {
					tagField = style.Render(tagField)
				}
				line += fmt.Sprintf(" %s", tagField)
			}
		} else if m.noColor {
			line += " (no worktree)"
		} else {
			line += dimStyle.Render(" (no worktree)")
		}
		if item.IsCurrent {
			suffix := " (current)"
			if !m.noColor {
				suffix = currentStyle.Render(suffix)
			}
			line += suffix
		}
		if !m.noColor && i == m.cursor {
			line = renderSelected(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if n := len(m.marked); n > 0 {
		hint := fmt.Sprintf("%d marked — [D] delete\n", n)
		if !m.noColor {
			hint = markedStyle.Render(hint)
		}
		b.WriteString(hint)
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
		"  mark       : c  (selection)",
		"  delete     : D  (marked branches, asks to confirm; unmerged branches require typing a phrase)",
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
