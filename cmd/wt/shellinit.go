package main

import "fmt"

const bashInit = `wt() {
  local arg
  for arg in "$@"; do
    case "$arg" in
      --path-only|--path-only=*|--version|--version=*|--help|-h)
        command wt "$@"
        return
        ;;
    esac
  done
  if [ "$1" = "init" ]; then
    command wt "$@"
    return
  fi
  local dest
  dest=$(command wt --path-only "$@") || return
  [ -n "$dest" ] && cd -- "$dest"
}
`

const zshInit = bashInit

const fishInit = `function wt
    for arg in $argv
        switch $arg
            case --path-only '--path-only=*' --version '--version=*' --help -h
                command wt $argv
                return
        end
    end
    if test "$argv[1]" = init
        command wt $argv
        return
    end
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
