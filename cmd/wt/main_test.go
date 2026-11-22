package main

import (
	"bufio"
	"os"
	"testing"

	"github.com/AgileIndustrialComplex/wt/internal/github"
)

func TestPRItemsMarkCurrentBranch(t *testing.T) {
	items := prItems([]github.PR{
		{Number: 17, Branch: "feature/force-update", Title: "A"},
		{Number: 18, Branch: "feature/pr", Title: "B"},
	}, "feature/pr")
	if items[0].IsCurrent {
		t.Fatalf("row for feature/force-update marked current, want only feature/pr")
	}
	if !items[1].IsCurrent {
		t.Fatalf("row for feature/pr not marked current: %+v", items[1])
	}
}

func TestPRDisplayOrder(t *testing.T) {
	got := prDisplay(github.PR{Number: 17, Branch: "feature/x", Title: "Add F key"})
	if want := "#17  feature/x  Add F key"; got != want {
		t.Fatalf("prDisplay() = %q, want %q", got, want)
	}
}

func TestSubcommandSkipsLeadingWrapperFlags(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		idx  int
	}{
		{[]string{"--path-only", "pr", "123"}, "pr", 1},
		{[]string{"pr", "123", "--no-color"}, "pr", 0},
		{[]string{"--worktree-root", "/wt", "--path-only", "pr"}, "pr", 3},
		{[]string{"--version"}, "", -1},
	} {
		cmd, idx := subcommand(tc.args)
		if cmd != tc.want || idx != tc.idx {
			t.Errorf("subcommand(%v) = %q, %d, want %q, %d", tc.args, cmd, idx, tc.want, tc.idx)
		}
	}
}

func TestReadAnswerConsumesOneLinePerPrompt(t *testing.T) {
	old := stdinReader
	t.Cleanup(func() { stdinReader = old })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("y\ns\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	stdinReader = bufio.NewReader(r)

	if got := readAnswer(); got != "y" {
		t.Fatalf("first answer = %q, want y", got)
	}
	if got := readAnswer(); got != "s" {
		t.Fatalf("second answer = %q, want s (a fresh reader would have discarded it)", got)
	}
	if got := readAnswer(); got != "" {
		t.Fatalf("answer after EOF = %q, want empty", got)
	}
}
