//go:build live

package openrouter_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/provider/openrouter"
)

// TestLiveResponses is an opt-in smoke test that makes one small, tool-free
// OpenRouter Responses request. Run it with:
//
//	OPENROUTER_API_KEY=... go test -tags=live ./provider/openrouter -run TestLiveResponses -v
func TestLiveResponses(t *testing.T) {
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" {
		t.Skip("OPENROUTER_API_KEY is not set")
	}
	model := strings.TrimSpace(os.Getenv("OPENROUTER_LIVE_MODEL"))
	if model == "" {
		model = "openrouter/auto"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	provider, err := openrouter.New(ctx, openrouter.Config{})
	if err != nil {
		t.Fatalf("create OpenRouter provider: %v", err)
	}
	defer func() { _ = provider.Close() }()
	session, err := provider.OpenSession(ctx, agent.SessionOptions{
		Model: model, Instructions: "Do not call tools. Follow the requested output exactly.",
	})
	if err != nil {
		t.Fatalf("open live session: %v", err)
	}
	result, err := session.Run(ctx, "Reply with exactly OPENROUTER_OK and nothing else.")
	if err != nil {
		t.Fatalf("run live turn: %v", err)
	}
	if strings.TrimSpace(result.Text) != "OPENROUTER_OK" {
		t.Fatalf("live response = %q", result.Text)
	}
	t.Logf("model=%s usage=%+v", model, result.Usage)
}
