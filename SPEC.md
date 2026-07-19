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
| List widget + fuzzy filter | [`bubbles/list`](https://github.com/charmbracelet/bubbles) or a minimal hand-rolled list | Gives arrow-key navigation, filtering, and pagination for free; still small. |
| Styling | [`lipgloss`](https://github.com/charmbracelet/lipgloss) | Optional color/highlight for the selected row; degrades gracefully on dumb terminals. |
| Git interaction | none (shell out to system `git` via `os/exec`) | Avoids embedding a Git implementation (e.g. `go-git`), which is heavyweight and can diverge in behavior from the user's actual Git config/hooks/credential helpers. Shelling out guarantees identical behavior to manual `git` commands. |
| CLI flags | Go standard `flag` package or [`cobra`](https://github.com/spf13/cobra) | `cobra` only if subcommands grow; `flag` is sufficient for a single-purpose tool and keeps the dependency tree minimal. |

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
│  - branch checked  → prompt: open as new     needed
│    out elsewhere    worktree, or switch there
└──────────────────────┘
```

**Data flow:**
1. `gitdata` runs `git branch --list --format='%(refname:short)|%(worktreepath)'` and `git worktree list --porcelain`, parses both, and produces a single normalized list of `Item`s tagged as `branch` or `worktree`, cross-referencing which branches are already checked out in a worktree (the exact information `git switch` uses to produce its "already checked out" error — surfacing it up front removes the dead end).
2. `ui` renders the list, handles keystrokes purely as state transitions (no side effects) until the user confirms.
3. `action` executes exactly one of: `git switch`, `git worktree add`, or (for worktrees) a directory change — nothing else touches repository state.

**Directory-change trick:** a subprocess cannot change its parent shell's `cwd`. `wt` handles this the same way `zoxide`/`fzf`-based `cd` wrappers do — see §5.

## 4. User Interface Design

On launch, the tool renders a single scrollable list, current item first:

```
Select branch or worktree (/ to filter, ? for help)
> ● main                     (current)         ~/proj
  ○ feature/login            [worktree]        ~/proj-login
  ○ bugfix/api-timeout                          (no worktree)
  ○ release/2.1              [worktree, locked] ~/proj-release
```

- `●` marks the branch/worktree matching the shell's current directory.
- `[worktree]` tags branches with a dedicated worktree; untagged branches are plain local branches with no worktree.
- `[locked]` reflects `git worktree list --porcelain` locked state — selecting it warns before any destructive-adjacent action, though `wt` never unlocks/removes worktrees itself.

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
wt --path-only         # print chosen path/branch, no cd (for scripting)
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

**Shell integration (cwd preservation):** since a child process cannot `cd` its parent shell, `wt` prints the destination path to stdout (with `--path-only`) and ships a thin shell function installed via `wt init <shell>`:

```bash
# added to .bashrc / .zshrc by `wt init bash`
wt() {
  local dest
  dest=$(command wt --path-only "$@") || return
  [ -n "$dest" ] && cd -- "$dest"
}
```
```fish
# added by `wt init fish`
function wt
    set -l dest (command wt --path-only $argv)
    test -n "$dest"; and cd $dest
end
```
This preserves cwd unless a switch actually happens, per the constraint, and never wraps or overrides real `git` subcommands.

**Core snippet — merging branches and worktrees:**

```go
type Item struct {
    Branch     string
    Path       string // "" if no worktree
    IsCurrent  bool
    Locked     bool
}

func collect() ([]Item, error) {
    wtOut, err := exec.Command("git", "worktree", "list", "--porcelain").Output()
    if err != nil { return nil, fmt.Errorf("git worktree list: %w", err) }
    worktrees := parseWorktreePorcelain(wtOut) // map[branch]Item

    brOut, err := exec.Command("git", "branch", "--list",
        "--format=%(refname:short)").Output()
    if err != nil { return nil, fmt.Errorf("git branch: %w", err) }

    var items []Item
    for _, b := range parseLines(brOut) {
        if wt, ok := worktrees[b]; ok {
            items = append(items, wt)
        } else {
            items = append(items, Item{Branch: b})
        }
    }
    return items, nil
}
```

**Action execution** — never reimplements Git logic, only calls it:

```go
func doSwitch(item Item) error {
    if item.Path != "" {
        fmt.Println(item.Path) // consumed by shell wrapper's cd
        return nil
    }
    cmd := exec.Command("git", "switch", item.Branch)
    cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
    return cmd.Run()
}
```

## 6. Testing Strategy

**Unit tests** (no real Git needed):
- Parsers: feed canned `git branch --format=...` and `git worktree list --porcelain` output (detached HEAD, locked/prunable worktrees, branch names with slashes) and assert correct `Item` merging.
- UI state machine: `bubbletea`'s `Update` is pure — test key sequences (`j j k Enter`, `/feat Enter`) against expected model state using `teatest` (bundled with bubbletea), no real terminal needed.

**Integration tests** (real repositories):
- Use `t.TempDir()` to create scratch repos via actual `git init`, `git commit --allow-empty`, `git worktree add`, then run the compiled `wt` binary against them (via `os/exec`) and assert on stdout/exit code.
- Golden-file tests for `--path-only` output across scenarios: plain branch switch, existing worktree switch, branch-with-no-worktree prompt.
- Cross-platform path handling test (Windows path separators) run only on the Windows CI runner.

**CI pipeline** (GitHub Actions):
```yaml
strategy:
  matrix:
    os: [ubuntu-latest, macos-latest, windows-latest]
steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-go@v5
  - run: go vet ./...
  - run: go test ./... -race -cover
  - run: golangci-lint run
```
Gate merges on all three OS legs passing.

## 7. Deployment Plan

- **Versioning:** SemVer tags (`v1.0.0`); version baked in at build time via `-ldflags "-X main.version=$(git describe --tags)"`.
- **Packaging:**
  - **Homebrew** (macOS/Linux): a `homebrew-tap` formula (`brew install <tap>/wt`) pointing at GitHub release binaries. Check for name clashes with existing casks/formulae before publishing to a shared tap — consider `wt-switch` as a fallback formula name if `wt` is taken.
  - **Binaries**: [`goreleaser`](https://goreleaser.com) cross-builds and publishes `.tar.gz`/`.zip` artifacts for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` on every tag push, attached to a GitHub Release.
  - **Scoop** (Windows) as a secondary manifest pointing at the same release binaries — avoids requiring WSL, though WSL users can also just use the Linux binary.
  - `pip`/`npm` distribution is deliberately skipped — the binary has no Python/Node dependency, and wrapping a static binary in either ecosystem would only add packaging weight for no benefit.
- **Release steps:** tag → CI runs test matrix → on green, `goreleaser release` builds and publishes binaries + updates Homebrew tap formula automatically (via goreleaser's `brews:` config) → changelog auto-generated from Conventional Commit messages since last tag.
