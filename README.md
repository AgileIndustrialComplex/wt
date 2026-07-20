# wt

An interactive, keyboard-driven CLI for switching between Git branches and worktrees, built as a companion for developers running parallel Claude Code sessions.

![wt](https://github.com/user-attachments/assets/3e2d2b0b-af42-42f6-ae70-cd190c6a4cb6)

## Overview

`wt` merges the output of `git branch` and `git worktree list` into a single, navigable picker. Instead of running two separate commands and manually resolving which branch lives in which checkout, you get one list, keyboard navigation, and a single confirmation step that either checks out a branch in place or jumps you into an existing worktree.

It solves a specific, recurring annoyance: `git switch <branch>` refuses to check out a branch that is already checked out in another worktree, and tells you so without telling you where. `wt` shows you that mapping up front, so there is no dead end to work around.

## Pitch

Claude Code is Anthropic's command-line coding agent. Its terminal supports running multiple parallel agent sessions, each attached to its own worktree, so you can have several workers making progress on different branches at the same time without them stepping on each other's checked-out state.

That workflow multiplies the exact pain `wt` was built to remove. When you are juggling several worktrees for several parallel workers, remembering which path holds which branch, and hopping between them without losing your place, gets tedious fast. `wt` gives you one interactive list of every branch and worktree, filterable and navigable with the same keys you already use in Vim or Emacs, so switching between your parallel Claude Code workers is a few keystrokes instead of a lookup.

## Installation

Install from source with Go 1.24.2 or newer:

```bash
go install github.com/AgileIndustrialComplex/wt/cmd/wt@latest
```

From a local checkout, run `make build` to produce `./wt`.

After installing the binary, enable shell integration so `wt` can change your shell's working directory (a subprocess cannot `cd` its parent shell on its own):

```bash
# bash
wt init bash >> ~/.bashrc

# zsh
wt init zsh >> ~/.zshrc
```

```fish
wt init fish | source
```

## Usage

Run `wt` with no arguments to open the picker:

```bash
wt
```

Run the picker from inside a Git repository. If you invoke it elsewhere, `wt`
points you to `wt init bash|zsh|fish` to install shell integration and then asks
you to change into a repository.

```
Select branch or worktree (j/k, / to filter, c to mark, ? for help)
> ● main                     (current)         ~/proj
  ○ feature/login            [worktree]
  ○ bugfix/api-timeout                          (no worktree)
  ○ release/2.1              [worktree, locked]
```

Navigate with the arrow keys, `j`/`k`, or `Ctrl-N`/`Ctrl-P`. Jump to the top or bottom with `g`/`G`, page with `Ctrl-D`/`Ctrl-U`, and filter the list by typing `/` followed by a search term. Press `Enter` to confirm, or `Esc`/`Ctrl-C`/`q` to cancel.

Highlight a branch or worktree and press `c` to mark or unmark it as part of the selection. After the first mark, a `[x]`/`[ ]` marker column appears and remains visible until the last mark is removed. Marked state persists as you move the cursor or filter. Only the current worktree cannot be marked; locked worktrees can be marked and deleted, subject to the same typed confirmation as dirty worktrees below.

Once at least one item is marked, a hint line appears below the list telling you which key acts on the marked set. Press `D` to delete every marked branch: a confirmation screen lists the branches (and worktree paths, where applicable) about to be removed. For a batch containing only merged branches and clean, unlocked worktrees, `Enter` runs the deletion (`git worktree remove` for items with a worktree, then `git branch -d` for each); any cancel key (`Esc`, `Ctrl-C`, `q`) backs out without touching Git state or losing your marked selection.

If any marked branch has unmerged changes, any marked worktree has modified or untracked files, or any marked worktree is locked, confirming does not delete anything yet — `git branch -d` and `git worktree remove` would refuse them anyway. Instead a second screen lists the unmerged branch(es), dirty worktree(s), and/or locked worktree(s) and requires you to type `delete` and press `Enter` before the whole batch is deleted: merged branches via `git branch -d` and unmerged ones via `git branch -D`, clean and unlocked worktrees via `git worktree remove`, dirty ones via `git worktree remove --force`, and locked ones via `git worktree remove --force --force` (git requires `--force` twice to remove a locked worktree). `Esc`/`Ctrl-C` cancels the entire batch and returns to the list without touching Git state.

The picker uses color to distinguish the selected row, current worktree, marked items, worktree tags, locked worktrees, and filter prompt. Pass `--no-color` to disable all ANSI styling.

Selecting a worktree changes your shell's directory to it. Selecting a plain branch with no worktree prompts you to either switch to it in place or create a new worktree for it:

```
'feature/login' has no worktree. [s]witch here  [w]orktree at ../proj-login  [Esc] cancel
```

Other flags:

```bash
wt --path-only          # print a selected worktree path for shell integration
wt --worktree-root DIR  # base directory to propose for new worktrees
wt --no-color           # disable ANSI styling
wt --version
wt --help
```

Optional config file at `~/.config/wt/config.toml`:

```toml
worktree_root = "~/worktrees"
default_action = "prompt"   # prompt | switch | worktree
keymap = "vim"              # vim | emacs | arrows-only
```

## Architecture / How it works

`wt` is written in Go and compiles to a single static binary, so there is no interpreter or runtime to install alongside it. The interactive list is built with the `bubbletea` terminal UI framework, which handles raw terminal mode, key events, and resizing consistently across Linux, macOS, and Windows.

`wt` never reimplements Git logic. It shells out to the real `git` binary for every read (`git branch`, including its merged-branch query, `git worktree list --porcelain`, and `git status --porcelain`) and every write (`git switch`, `git worktree add`, `git worktree remove`, confirmed `git worktree remove --force`, confirmed `git worktree remove --force --force`, `git branch -d`, and confirmed `git branch -D`), so behavior always matches what you would get running those commands yourself, including your existing hooks and credential helpers. A small collector step parses those outputs and merges them into one list, tagging each branch with its merge status, whether it already has a worktree, whether that worktree is dirty, and whether it is locked. That merged view is what makes the "already checked out elsewhere" case visible instead of being an error you hit after the fact.

Because a subprocess cannot change its parent shell's working directory, `wt` prints the destination path and relies on a thin shell function, installed once via `wt init <shell>`, to perform the actual `cd`. This keeps your working directory untouched unless a switch actually happens.

This design is what makes `wt` a natural fit alongside Claude Code's multi-worker terminal. Each parallel Claude Code worker typically lives in its own worktree, on its own branch. `wt` gives you a single place to see all of them at once and move between them without breaking stride, whether you are checking on a worker's progress, picking up a finished branch, or starting a new one.

## Contributing

Contributions are welcome via issues and pull requests.
