package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dire-kiwi/dire-agent/agent"
)

func TestUsageJSONFieldNames(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(agent.Usage{
		InputTokens: 1, OutputTokens: 2, CacheReadTokens: 3,
		CacheWriteTokens: 4, TotalTokens: 5, ContextTokens: 6,
		ContextWindow: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"input_tokens":1,"output_tokens":2,"cache_read_tokens":3,"cache_write_tokens":4,"total_tokens":5,"context_tokens":6,"context_window":7}`
	if string(encoded) != want {
		t.Fatalf("Usage JSON = %s, want %s", encoded, want)
	}
}

type fakeProvider struct {
	options agent.SessionOptions
	session agent.Session
	err     error
}

func (p *fakeProvider) OpenSession(_ context.Context, options agent.SessionOptions) (agent.Session, error) {
	p.options = options
	return p.session, p.err
}

func (p *fakeProvider) Close() error { return nil }

type fakeSession struct {
	prompts []string
}

func (s *fakeSession) ID() string { return "session-1" }

func (s *fakeSession) Run(_ context.Context, prompt string) (agent.Result, error) {
	s.prompts = append(s.prompts, prompt)
	return agent.Result{Text: "answer", SessionID: s.ID()}, nil
}

func TestAgentUsesProviderSession(t *testing.T) {
	t.Parallel()

	session := &fakeSession{}
	provider := &fakeProvider{session: session}
	wantOptions := agent.SessionOptions{
		Model:            "model-a",
		WorkingDirectory: "/tmp/project",
		Instructions:     "Be concise.",
	}

	a, err := agent.New(context.Background(), provider, wantOptions)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if provider.options != wantOptions {
		t.Fatalf("OpenSession() options = %#v, want %#v", provider.options, wantOptions)
	}
	if got := a.ID(); got != "session-1" {
		t.Fatalf("ID() = %q, want session-1", got)
	}

	result, err := a.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Text != "answer" {
		t.Fatalf("Run().Text = %q, want answer", result.Text)
	}
	if len(session.prompts) != 1 || session.prompts[0] != "hello" {
		t.Fatalf("session prompts = %#v, want [hello]", session.prompts)
	}
}

func TestAgentRejectsEmptyPrompt(t *testing.T) {
	t.Parallel()

	a, err := agent.New(context.Background(), &fakeProvider{session: &fakeSession{}}, agent.SessionOptions{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := a.Run(context.Background(), "  \n\t"); err == nil {
		t.Fatal("Run() error = nil, want an empty prompt error")
	}
}

func TestAgentWrapsProviderError(t *testing.T) {
	t.Parallel()

	want := errors.New("provider unavailable")
	_, err := agent.New(context.Background(), &fakeProvider{err: want}, agent.SessionOptions{})
	if !errors.Is(err, want) {
		t.Fatalf("New() error = %v, want it to wrap %v", err, want)
	}
}
