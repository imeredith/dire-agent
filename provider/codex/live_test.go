//go:build live

package codex_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/provider/codex"
)

// TestLiveLunaReasoningAndImage exercises the streaming reasoning-summary and
// multimodal request paths with the current Codex CLI subscription login.
func TestLiveLunaReasoningAndImage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	provider, err := codex.New(ctx, codex.Config{})
	if err != nil {
		t.Fatalf("start direct provider: %v", err)
	}
	defer func() { _ = provider.Close() }()
	opened, err := provider.OpenSession(ctx, agent.SessionOptions{
		Model: "gpt-5.6-luna", Instructions: "Do not use tools. Follow the requested final output exactly.",
	})
	if err != nil {
		t.Fatal(err)
	}
	session, ok := opened.(agent.StepSession)
	if !ok {
		t.Fatal("Codex session does not implement agent.StepSession")
	}
	// Generate the fixture with the standard PNG encoder so the live API also
	// validates the exact byte path used by image inputs.
	fixture := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			pixel := color.RGBA{B: 220, A: 255}
			if x < 16 && y < 16 {
				pixel = color.RGBA{R: 240, A: 255}
			}
			fixture.Set(x, y, pixel)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, fixture); err != nil {
		t.Fatal(err)
	}
	var reasoning strings.Builder
	result, err := session.Step(ctx, agent.StepRequest{
		UserMessages:    []string{"Carefully inspect the attached square image, identify which quadrant contains the red patch, and reply with exactly TOP_LEFT, TOP_RIGHT, BOTTOM_LEFT, or BOTTOM_RIGHT and nothing else."},
		Images:          []agent.ImageInput{{Name: "pixel.png", MimeType: "image/png", Data: encoded.Bytes()}},
		ReasoningEffort: "high",
		OnEvent: func(event agent.ModelEvent) {
			if event.Type == "reasoning_delta" {
				reasoning.WriteString(event.Delta)
			}
			if event.Type == "reasoning_done" && strings.TrimSpace(event.Text) != "" {
				reasoning.Reset()
				reasoning.WriteString(event.Text)
			}
		},
	})
	if err != nil {
		t.Fatalf("run live multimodal reasoning turn: %v", err)
	}
	if strings.TrimSpace(result.Text) != "TOP_LEFT" {
		t.Fatalf("live image response = %q", result.Text)
	}
	if strings.TrimSpace(reasoning.String()) == "" {
		t.Fatal("live Luna stream emitted no reasoning summary")
	}
	if strings.Contains(reasoning.String(), "<!--") {
		t.Fatalf("live Luna reasoning summary leaked provider comment marker: %q", reasoning.String())
	}
	t.Logf("reasoning summary bytes=%d usage=%+v", reasoning.Len(), result.Usage)
}

// TestLiveSubscriptionCredentials calls the subscription endpoint directly
// using the current Codex CLI login. It is excluded from ordinary test runs
// because it makes a real model request and consumes subscription allowance.
func TestLiveSubscriptionCredentials(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	provider, err := codex.New(ctx, codex.Config{})
	if err != nil {
		t.Fatalf("start direct provider using Codex CLI credentials: %v", err)
	}
	defer func() {
		if err := provider.Close(); err != nil {
			t.Errorf("close provider: %v", err)
		}
	}()

	account, err := provider.Account(ctx)
	if err != nil {
		t.Fatalf("read Codex account: %v", err)
	}
	if account.Mode != "chatgpt" {
		t.Fatalf("account mode = %q, want chatgpt", account.Mode)
	}

	model := os.Getenv("CODEX_LIVE_MODEL")
	if model == "" {
		model = "gpt-5.6-luna"
	}
	session, err := provider.OpenSession(ctx, agent.SessionOptions{
		Model:        model,
		Instructions: "Do not use tools. Follow the requested output format exactly.",
	})
	if err != nil {
		t.Fatalf("open live session: %v", err)
	}
	result, err := session.Run(ctx, "Reply with exactly DIRE_AGENT_LIVE_OK and nothing else.")
	if err != nil {
		t.Fatalf("run live turn: %v", err)
	}
	if strings.TrimSpace(result.Text) != "DIRE_AGENT_LIVE_OK" {
		t.Fatalf("live response = %q, want DIRE_AGENT_LIVE_OK", result.Text)
	}
	if result.Usage.ContextWindow != 372_000 {
		t.Fatalf("Luna context window = %d, want 372000", result.Usage.ContextWindow)
	}
	t.Logf("Luna usage: input=%d output=%d cache_read=%d cache_write=%d context=%d/%d",
		result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.CacheReadTokens,
		result.Usage.CacheWriteTokens, result.Usage.ContextTokens, result.Usage.ContextWindow)
}

// TestLiveLunaPromptCaching verifies GPT-5.6 prompt-cache reuse. The first turn
// is intentionally longer than the 1,024-token eligibility floor; the second
// subsequent turns reuse the exact prefix and session cache key. Cache routing
// is best-effort, so the test allows a few short reuse attempts but still
// requires a real nonzero cache read. The subscription stream currently
// reports cache reads but may omit cache-write telemetry.
func TestLiveLunaPromptCaching(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	provider, err := codex.New(ctx, codex.Config{})
	if err != nil {
		t.Fatalf("start direct provider using Codex CLI credentials: %v", err)
	}
	defer func() { _ = provider.Close() }()

	session, err := provider.OpenSession(ctx, agent.SessionOptions{
		Model:        "gpt-5.6-luna",
		Instructions: "This is a prompt-cache telemetry test. Do not use tools. Obey the final reply instruction exactly and do not discuss the reference block.",
	})
	if err != nil {
		t.Fatalf("open Luna cache session: %v", err)
	}

	stablePrefix := strings.Repeat("dire-agent stable cache validation prefix; ", 700)
	first, err := session.Run(ctx, "Reference block (ignore its content):\n"+stablePrefix+"\nReply with exactly DIRE_AGENT_CACHE_WRITE_OK and nothing else.")
	if err != nil {
		t.Fatalf("run Luna cache-write turn: %v", err)
	}
	if strings.TrimSpace(first.Text) != "DIRE_AGENT_CACHE_WRITE_OK" {
		t.Fatalf("cache-write response = %q", first.Text)
	}
	if first.Usage.CacheWriteTokens == 0 {
		t.Logf("subscription endpoint did not expose cache-write telemetry: usage=%+v", first.Usage)
	}
	t.Logf("cache write turn: input=%d output=%d cache_read=%d cache_write=%d",
		first.Usage.InputTokens, first.Usage.OutputTokens, first.Usage.CacheReadTokens, first.Usage.CacheWriteTokens)

	var cached agent.Result
	for attempt := 1; attempt <= 3; attempt++ {
		cached, err = session.Run(ctx, "Reply with exactly DIRE_AGENT_CACHE_READ_OK and nothing else.")
		if err != nil {
			t.Fatalf("run Luna cache-read turn %d: %v", attempt, err)
		}
		if strings.TrimSpace(cached.Text) != "DIRE_AGENT_CACHE_READ_OK" {
			t.Fatalf("cache-read response %d = %q", attempt, cached.Text)
		}
		t.Logf("cache read attempt %d: input=%d output=%d cache_read=%d cache_write=%d context=%d/%d",
			attempt, cached.Usage.InputTokens, cached.Usage.OutputTokens, cached.Usage.CacheReadTokens,
			cached.Usage.CacheWriteTokens, cached.Usage.ContextTokens, cached.Usage.ContextWindow)
		if cached.Usage.CacheReadTokens > 0 {
			break
		}
	}
	if cached.Usage.CacheReadTokens == 0 {
		t.Fatalf("Luna reported no cache read after bounded reuse attempts: usage=%+v", cached.Usage)
	}
	if cached.Usage.ContextWindow != 372_000 {
		t.Fatalf("Luna context window = %d, want 372000", cached.Usage.ContextWindow)
	}
}
