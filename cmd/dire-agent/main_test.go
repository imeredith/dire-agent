package main

import (
	"bytes"
	"context"
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

func TestRunVersionDoesNotStartDaemon(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := run([]string{"version"}, strings.NewReader(""), &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output.String(), "dire-agent ") {
		t.Fatalf("version output = %q", output.String())
	}
}

func TestRunHelpDocumentsLifecycleCommands(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := run([]string{"help"}, strings.NewReader(""), &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"start", "stop", "upgrade", "tui"} {
		if !strings.Contains(output.String(), command) {
			t.Errorf("help output does not mention %q", command)
		}
	}
}

func TestSameVersionIgnoresVPrefix(t *testing.T) {
	t.Parallel()
	if !sameVersion("v1.2.3", "1.2.3") {
		t.Fatal("sameVersion rejected equivalent release versions")
	}
	if sameVersion("dev", "v1.2.3") {
		t.Fatal("sameVersion accepted different versions")
	}
}

func TestAskOpenRouterProviderDefaults(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	provider, model, err := newAskProvider(context.Background(), "openrouter", "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if model != "openrouter/auto" {
		t.Fatalf("model = %q", model)
	}
}

func TestAskRejectsUnknownProvider(t *testing.T) {
	t.Parallel()
	if _, _, err := newAskProvider(context.Background(), "unknown", "model", ""); err == nil {
		t.Fatal("newAskProvider accepted an unknown provider")
	}
}
