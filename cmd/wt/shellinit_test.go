package main

import (
	"strings"
	"testing"
)

func TestShellInitRoutesNonPickerCommands(t *testing.T) {
	tests := []struct {
		name    string
		wrapper string
		want    []string
	}{
		{
			name:    "bash",
			wrapper: bashInit,
			want:    []string{"--path-only|--path-only=*|--version|--version=*|--help|-h", `if [ "$1" = "init" ]`, `command wt "$@"`},
		},
		{
			name:    "fish",
			wrapper: fishInit,
			want:    []string{"case --path-only '--path-only=*' --version '--version=*' --help -h", `if test "$argv[1]" = init`, "command wt $argv"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range tt.want {
				if !strings.Contains(tt.wrapper, want) {
					t.Errorf("wrapper does not contain %q", want)
				}
			}
		})
	}
}
