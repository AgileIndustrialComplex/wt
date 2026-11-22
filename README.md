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

Optional config file at `~/.config/wt/config.toml`:

```toml
worktree_root = "~/worktrees"
default_action = "prompt"   # prompt | switch | worktree
keymap = "vim"              # vim | emacs | arrows-only
```
