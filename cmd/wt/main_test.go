package main

import (
	"bufio"
	"os"
	"testing"
)

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
