# Interactive Git Worktree and Branch Switcher — Specification

## 1. Overview

`wt` is a single-binary CLI tool that presents a unified, interactively navigable list of local branches and Git worktrees, and switches the user's shell context to whichever one they select — either by `cd`-ing into an existing worktree or by checking out a branch in the current repository. It never mutates Git state on its own; every write (checkout, `worktree add`) happens only after explicit user confirmation, and only via standard `git` invocations.

## 2. Technology Choice

**Language: Go.**

- Compiles to a single static binary per platform (Linux, macOS, Windows) — no runtime, no interpreter version drift, satisfies the "avoid heavyweight dependencies" constraint better than Python (needs a venv/interpreter) or Node (needs a runtime).
- Excellent, mature TUI ecosystem.
- `os/exec` gives simple, safe subprocess control for shelling out to `git`.
- Cross-compilation (`GOOS`/`GOARCH`) makes producing Windows/macOS/Linux binaries from one CI job trivial.

**Libraries:**

| Purpose | Library | Why |
|---|---|---|
| TUI framework | [`bubbletea`](https://github.com/charmbracelet/bubbletea) (Elm-architecture TUI) | Mature, widely used, handles raw terminal mode, resize, and key events across platforms including Windows (via `x/term`). |
| List and substring filter | Hand-rolled Bubble Tea model | Keeps navigation, filtering, and pagination behavior explicit and small. |
| Styling | [`lipgloss`](https://github.com/charmbracelet/lipgloss) | Highlights the selected row; `--no-color` disables that styling. |
| Git interaction | none (shell out to system `git` via `os/exec`) | Avoids embedding a Git implementation (e.g. `go-git`), which is heavyweight and can diverge in behavior from the user's actual Git config/hooks/credential helpers. Shelling out guarantees identical behavior to manual `git` commands. |
| CLI flags | Go standard `flag` package | Sufficient for a single-purpose tool and keeps the dependency tree minimal. |

No `fzf` dependency is required, but the design deliberately keeps the same *interaction feel* the user's `fzf` one-liner already provides, while merging branches and worktrees into one list and natively understanding worktree-vs-plain-branch semantics — avoiding the "already checked out elsewhere" dead end of `git switch`.

## 3. Architecture Overview

```
┌────────────────────┐
│  cmd/wt (main)      │  parses flags, loads config, wires everything
└─────────┬───────────┘
          │
┌─────────▼───────────┐     ┌────────────────────┐
│  gitdata (collector) │────▶│  git branch --list  │
│                      │     │  git worktree list  │
│  - parses branches   │     │  git rev-parse ...  │
│  - parses worktrees  │     └────────────────────┘
│  - merges into Items │
└─────────┬───────────┘
          │  []Item{Name, Path, IsWorktree, IsCurrent, Locked, ...}
┌─────────▼───────────┐
│  ui (bubbletea)      │  renders list, handles keys, filters
│  - Model/Update/View │
└─────────┬───────────┘
          │  user confirms selection: Item
┌─────────▼───────────┐
│  action (executor)   │
│  - plain branch  → git switch <branch>       (in cwd repo)
│  - worktree path → emit cd path to wrapper   (no git mutation)
│  - existing path → return it to the shell wrapper
└──────────────────────┘
```

**Data flow:**
1. `gitdata` runs `git branch --list --format='%(refname:short)'` and `git worktree list --porcelain -z`, parses both, and produces a single normalized list of items, cross-referencing which branches are already checked out in a worktree (the exact information `git switch` uses to produce its "already checked out" error — surfacing it up front removes the dead end).
2. `ui` renders the list, handles keystrokes purely as state transitions (no side effects) until the user confirms.
3. `action` executes exactly one of: `git switch`, `git worktree add`, or (for worktrees) a directory change — nothing else touches repository state.

**Directory-change trick:** a subprocess cannot change its parent shell's `cwd`. `wt` handles this the same way `zoxide`/`fzf`-based `cd` wrappers do — see §5.

## 4. User Interface Design

On launch, the tool renders a single scrollable list, current item first:

```
Select branch or worktree (/ to filter, ? for help)
> ● main                     (current)         ~/proj
  ○ feature/login            [worktree]
  ○ bugfix/api-timeout                          (no worktree)
  ○ release/2.1              [worktree, locked]
```

- `●` marks the branch/worktree matching the shell's current directory.
- `[worktree]` tags branches with a dedicated worktree; untagged branches are plain local branches with no worktree.
- `[locked]` reflects `git worktree list --porcelain` locked state; selecting it asks for confirmation before returning its path. `wt` never unlocks or removes worktrees.
- The directory path is shown only for the currently highlighted item; moving the cursor reveals that item's path and hides the previous one.

**Key bindings** (Vim + Emacs + arrows):

| Action | Keys |
|---|---|
| Move down | `↓`, `j`, `Ctrl-N` |
| Move up | `↑`, `k`, `Ctrl-P` |
| Page down / up | `Ctrl-D` / `Ctrl-U` |
| Jump to top / bottom | `g` / `G` |
| Filter/search | `/` then type; `Esc` clears |
| Confirm selection | `Enter` |
| Cancel | `Esc`, `Ctrl-C`, `q` |
| Help overlay | `?` |

Selecting a plain branch with no worktree and confirming prompts one extra line:
```
'feature/login' has no worktree. [s]witch here  [w]orktree at ../proj-login  [Esc] cancel
```
This directly resolves the stated pain point: instead of `git switch` erroring out, the tool offers the two valid resolutions inline.

## 5. Implementation Details

**Command-line arguments:**

```
wt                     # interactive picker, default action
wt --path-only         # print a selected worktree path for shell integration
wt --worktree-root DIR # base dir for new worktrees (overrides config)
wt --no-color
wt --version / --help
```

**Config file** (`~/.config/wt/config.toml`, optional):

```toml
worktree_root = "~/worktrees"   # where "create worktree" proposes new paths
default_action = "prompt"       # prompt | switch | worktree
keymap = "vim"                  # vim | emacs | arrows-only (all always active; affects hint text only)
```

**Shell integration (cwd preservation):** since a child process cannot `cd` its parent shell, `wt --path-only` prints a selected worktree destination and `wt init bash|zsh|fish` emits a thin wrapper that changes directory when that output is non-empty. Help, version, explicit `--path-only`, and `init` invocations pass through to the binary. This preserves cwd unless the user selects a different worktree and never wraps or overrides real `git` subcommands. The emitted wrapper source in `cmd/wt/shellinit.go` is authoritative.

The authoritative collector and action implementations are in
`internal/gitdata/gitdata.go` and `internal/action/action.go`.

## 6. Testing Strategy

**Unit tests** (no real Git needed):
- Parsers: feed canned `git branch --format=...` and `git worktree list --porcelain` output (detached HEAD, locked/prunable worktrees, branch names with slashes) and assert correct `Item` merging.
- UI state machine: `bubbletea`'s `Update` is pure; test key sequences against expected model state directly, with no real terminal needed.

**Integration tests** (real repositories):
- Use `t.TempDir()` to create scratch repositories via actual `git init`, `git commit --allow-empty`, and `git worktree add` commands.
- Drive the action and collection layers against the scratch repository and assert the resulting paths and Git repository state.
- Cover switching a plain branch, resolving an existing worktree, and creating a worktree.

## 7. Building

The repository currently supports source builds. Run `make build` to produce the `wt` binary, or install the command with `go install github.com/AgileIndustrialComplex/wt/cmd/wt@latest`. Release automation and package-manager distribution are not configured.
