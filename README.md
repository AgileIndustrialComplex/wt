# wt

An interactive, keyboard-driven CLI for switching between Git branches and worktrees, built as a companion for developers running parallel AI coding agents sessions.

![wt](https://github.com/user-attachments/assets/3e2d2b0b-af42-42f6-ae70-cd190c6a4cb6)

## Installation

Install from source with Go 1.24.2 or newer:

```bash
go install github.com/AgileIndustrialComplex/wt/cmd/wt@latest
```

From a local checkout, run `make build` to produce `./wt`.

After installing the binary, enable shell integration so `wt` can change your shell's working directory:

```bash
# bash
wt init bash >> ~/.bashrc

# zsh
wt init zsh >> ~/.zshrc

# fish
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

### Pull requests

`wt pr` needs the GitHub CLI (`gh`) installed and logged in for the current
repository.

```bash
wt pr          # list open PRs with number, branch and title; pick one to switch to
wt pr 123      # switch straight to the branch of PR #123
```

`wt` fetches the PR head from `origin` first. If the local branch is strictly
behind `origin/<branch>`, it asks `Fast-forward ...? [y/N]`; only yes runs
`git merge --ff-only origin/<branch>` in the branch's worktree (or a
fast-forward ref update for a plain branch). A branch that has diverged is
reported as an error and left untouched; a missing local branch is created
from the remote tip without asking. Switching then follows the usual rules:
a branch with a worktree returns its path, a plain branch shows the
`[s]witch here / [w]orktree` prompt. The row whose branch is checked out in
the current worktree shows ● and `(current)`, just like the normal picker.
`--no-color`, `--worktree-root DIR` and `--path-only` work here too. PR heads
are resolved as `origin/<branch>`, so
this covers branches pushed to the repository's `origin`, not fork PR heads.

Optional config file at `~/.config/wt/config.toml`:

```toml
worktree_root = "~/worktrees"
default_action = "prompt"   # prompt | switch | worktree
keymap = "vim"              # vim | emacs | arrows-only
```
