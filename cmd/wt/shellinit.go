package main

import "fmt"

const bashInit = `wt() {
  local dest
  dest=$(command wt --path-only "$@") || return
  [ -n "$dest" ] && cd -- "$dest"
}
`

const zshInit = bashInit

const fishInit = `function wt
    set -l dest (command wt --path-only $argv)
    test -n "$dest"; and cd $dest
end
`

func runInit(shell string) error {
	switch shell {
	case "bash":
		fmt.Print(bashInit)
	case "zsh":
		fmt.Print(zshInit)
	case "fish":
		fmt.Print(fishInit)
	default:
		return fmt.Errorf("unsupported shell %q (expected bash, zsh, or fish)", shell)
	}
	return nil
}
