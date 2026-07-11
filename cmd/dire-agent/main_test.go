package main

import (
	"strings"
	"testing"
)

func TestReadPromptFromArguments(t *testing.T) {
	t.Parallel()

	got, err := readPrompt([]string{"hello", "agent"}, strings.NewReader("ignored"))
	if err != nil {
		t.Fatalf("readPrompt() error = %v", err)
	}
	if got != "hello agent" {
		t.Fatalf("readPrompt() = %q, want hello agent", got)
	}
}

func TestReadPromptFromStdin(t *testing.T) {
	t.Parallel()

	got, err := readPrompt(nil, strings.NewReader("  hello from stdin\n"))
	if err != nil {
		t.Fatalf("readPrompt() error = %v", err)
	}
	if got != "hello from stdin" {
		t.Fatalf("readPrompt() = %q, want hello from stdin", got)
	}
}

func TestReadPromptRejectsEmptyInput(t *testing.T) {
	t.Parallel()

	if _, err := readPrompt(nil, strings.NewReader(" \n")); err == nil {
		t.Fatal("readPrompt() error = nil, want an error")
	}
}
