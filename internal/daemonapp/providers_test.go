package daemonapp

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeModelProvider(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{
		"": "codex", "CODEX": "codex", " openrouter ": "openrouter",
	} {
		got, err := normalizeModelProvider(input)
		if err != nil || got != want {
			t.Errorf("normalizeModelProvider(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := normalizeModelProvider("unknown"); err == nil {
		t.Fatal("normalizeModelProvider accepted an unknown provider")
	}
}

func TestOpenRouterProviderSelection(t *testing.T) {
	t.Parallel()
	provider, err := newModelProvider(context.Background(), modelProviderOptions{
		Name: "openrouter", DefaultModel: "openrouter/auto", OpenRouterAPIKey: "test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider == nil {
		t.Fatal("newModelProvider returned a nil provider")
	}
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRouterRequiresQualifiedModel(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"gpt-5.6", "openrouter//auto", "/model", "provider/"} {
		_, err := newModelProvider(context.Background(), modelProviderOptions{
			Name: "openrouter", DefaultModel: model, OpenRouterAPIKey: "test-key",
		})
		if err == nil || !strings.Contains(err.Error(), "organization-qualified") {
			t.Errorf("newModelProvider(%q) error = %v", model, err)
		}
	}
}

func TestProviderDefaults(t *testing.T) {
	t.Parallel()
	if got := defaultModelForProvider("openrouter"); got != "openrouter/auto" {
		t.Fatalf("OpenRouter default model = %q", got)
	}
	if got := defaultModelForProvider("codex"); got != "gpt-5.6" {
		t.Fatalf("Codex default model = %q", got)
	}
}
