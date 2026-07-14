package daemon_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/capability"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
	"github.com/dire-kiwi/dire-agent/websearch"
)

func TestWebSearchCapabilityIsDetectedFromProviderInterface(t *testing.T) {
	t.Parallel()
	t.Run("future provider", func(t *testing.T) {
		root := t.TempDir()
		store, err := threadstore.New(filepath.Join(root, "conversations"))
		if err != nil {
			t.Fatal(err)
		}
		manager, err := daemon.NewManager(daemon.ManagerConfig{
			Store: store, Provider: &searchProvider{}, DefaultCWD: root,
			DefaultProvider: "future", DefaultModel: "future-model",
		})
		if err != nil {
			t.Fatal(err)
		}
		defer manager.Close()

		project, err := manager.CreateProject(context.Background(), daemon.CreateProjectOptions{CWD: root})
		if err != nil {
			t.Fatal(err)
		}
		chat, err := manager.CreateChat(context.Background(), daemon.CreateChatOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{project.ID, chat.ID} {
			state, err := manager.CapabilityState(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			if !hasCapability(state.Capabilities, websearch.Name, "provider:future") {
				t.Fatalf("capabilities for %s = %#v", id, state.Capabilities)
			}
		}
	})

	t.Run("provider name alone is insufficient", func(t *testing.T) {
		root := t.TempDir()
		store, err := threadstore.New(filepath.Join(root, "conversations"))
		if err != nil {
			t.Fatal(err)
		}
		manager, err := daemon.NewManager(daemon.ManagerConfig{
			Store: store, Provider: &fakeProvider{}, DefaultCWD: root,
			DefaultProvider: "codex", DefaultModel: "fake-model",
		})
		if err != nil {
			t.Fatal(err)
		}
		defer manager.Close()
		project, err := manager.CreateProject(context.Background(), daemon.CreateProjectOptions{CWD: root})
		if err != nil {
			t.Fatal(err)
		}
		state, err := manager.CapabilityState(context.Background(), project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if hasCapability(state.Capabilities, websearch.Name, "") {
			t.Fatalf("non-search provider exposed web_search: %#v", state.Capabilities)
		}
	})

	t.Run("explicitly disabled", func(t *testing.T) {
		root := t.TempDir()
		store, err := threadstore.New(filepath.Join(root, "conversations"))
		if err != nil {
			t.Fatal(err)
		}
		manager, err := daemon.NewManager(daemon.ManagerConfig{
			Store: store, Provider: &searchProvider{}, DefaultCWD: root,
			DefaultProvider: "future", DefaultModel: "future-model", DisableWebSearch: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer manager.Close()
		project, err := manager.CreateProject(context.Background(), daemon.CreateProjectOptions{CWD: root})
		if err != nil {
			t.Fatal(err)
		}
		state, err := manager.CapabilityState(context.Background(), project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if hasCapability(state.Capabilities, websearch.Name, "") {
			t.Fatalf("disabled provider exposed web_search: %#v", state.Capabilities)
		}
	})
}

func TestWebSearchRunsEphemeralProviderAgentWithoutTeamArtifacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	provider := &searchProvider{}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultCWD: root,
		DefaultProvider: "future", DefaultModel: "future-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{
		CWD: root, Model: "future-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Prompt(ctx, project.ID, "find the current release", ""); err != nil {
		t.Fatal(err)
	}
	waitForConversationIdle(t, ctx, manager, project.ID)

	requests := provider.SearchRequests()
	if len(requests) != 1 || requests[0].Query != "current release" || requests[0].NumResults != 3 {
		t.Fatalf("search requests = %#v", requests)
	}
	messages, err := manager.Messages(ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var toolOutput, finalAnswer string
	for _, message := range messages {
		if message.Role == "tool" && strings.Contains(message.Content, "search answer") {
			toolOutput = message.Content
		}
		if message.Role == "assistant" {
			finalAnswer = message.Content
		}
	}
	if !strings.Contains(toolOutput, "[Source](https://example.test/source)") || !strings.Contains(finalAnswer, "tool returned: search answer") {
		t.Fatalf("tool/final output = %q / %q; messages=%#v", toolOutput, finalAnswer, messages)
	}

	resources, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].ID != project.ID {
		t.Fatalf("search created persistent resources: %#v", resources)
	}
	agents, err := manager.ListAgents(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].ID != project.ID {
		t.Fatalf("search created team agents: %#v", agents)
	}
	events, err := manager.Events(ctx, project.ID, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == "agent_spawned" || event.Type == "agent_completed" {
			t.Fatalf("ephemeral search emitted persistent-agent event: %#v", event)
		}
	}
	state, err := manager.State(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Running || state.SteeringQueued != 0 || state.FollowUpsQueued != 0 {
		t.Fatalf("search left conversation work queued: %#v", state)
	}
}

func TestAbortCancelsEphemeralWebSearch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	canceled := make(chan struct{})
	provider := &searchProvider{search: func(ctx context.Context, _ websearch.Request) (websearch.Response, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return websearch.Response{}, ctx.Err()
	}}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultCWD: root, DefaultModel: "future-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Prompt(ctx, project.ID, "search slowly", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := manager.Abort(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waitForConversationIdle(t, ctx, manager, project.ID)
	agents, err := manager.ListAgents(ctx, project.ID)
	if err != nil || len(agents) != 1 {
		t.Fatalf("agents after abort = %#v, %v", agents, err)
	}
}

func TestWebSearchBudgetResetsForNextSettledPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	provider := &searchProvider{toolArguments: json.RawMessage(`{
		"queries":["one","two","three","four","five","six","seven","eight"]
	}`)}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultCWD: root, DefaultModel: "future-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"first search run", "second search run"} {
		if err := manager.Prompt(ctx, project.ID, prompt, ""); err != nil {
			t.Fatal(err)
		}
		waitForConversationIdle(t, ctx, manager, project.ID)
	}
	if requests := provider.SearchRequests(); len(requests) != 16 {
		t.Fatalf("search budget did not reset between runs: %d requests", len(requests))
	}
}

func hasCapability(descriptors []capability.Descriptor, name, source string) bool {
	for _, descriptor := range descriptors {
		if descriptor.Name == name && (source == "" || descriptor.Source == source) && descriptor.Enabled && descriptor.Status == "ready" {
			return true
		}
	}
	return false
}

func waitForConversationIdle(t *testing.T, ctx context.Context, manager *daemon.Manager, id string) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := manager.State(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !state.Running && state.Thread.Status == "idle" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

type searchProvider struct {
	next          atomic.Int64
	mu            sync.Mutex
	requests      []websearch.Request
	search        func(context.Context, websearch.Request) (websearch.Response, error)
	toolArguments json.RawMessage
}

func (*searchProvider) Name() string { return "future" }

func (p *searchProvider) Search(ctx context.Context, request websearch.Request) (websearch.Response, error) {
	p.mu.Lock()
	p.requests = append(p.requests, request)
	p.mu.Unlock()
	if p.search != nil {
		return p.search(ctx, request)
	}
	return websearch.Response{
		Query: request.Query, Answer: "search answer",
		Citations: []websearch.Citation{{Title: "Source", URL: "https://example.test/source"}},
		Provider:  "future", SessionID: fmt.Sprintf("search-%d", len(p.SearchRequests())),
	}, nil
}

func (p *searchProvider) SearchRequests() []websearch.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]websearch.Request(nil), p.requests...)
}

func (p *searchProvider) OpenSession(context.Context, agent.SessionOptions) (agent.Session, error) {
	return &searchCallingSession{id: fmt.Sprintf("future-%d", p.next.Add(1)), arguments: p.toolArguments}, nil
}

func (p *searchProvider) OpenSessionFromState(_ context.Context, _ agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	var history []string
	if len(state.Data) != 0 {
		_ = json.Unmarshal(state.Data, &history)
	}
	return &searchCallingSession{id: state.ID, history: history, arguments: p.toolArguments}, nil
}

func (*searchProvider) Close() error { return nil }

type searchCallingSession struct {
	id        string
	mu        sync.Mutex
	history   []string
	arguments json.RawMessage
}

func (s *searchCallingSession) ID() string { return s.id }

func (s *searchCallingSession) Run(ctx context.Context, prompt string) (agent.Result, error) {
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	return step.Result, err
}

func (s *searchCallingSession) Step(_ context.Context, request agent.StepRequest) (agent.StepResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(request.ToolResults) != 0 {
		answer := "tool returned: " + request.ToolResults[0].Output
		s.history = append(s.history, answer)
		return agent.StepResult{Result: agent.Result{Text: answer, SessionID: s.id, TurnID: "final"}}, nil
	}
	found := false
	for _, definition := range request.Tools {
		if definition.Name == websearch.Name {
			found = true
			break
		}
	}
	if !found {
		return agent.StepResult{}, errors.New("web_search was not exposed to the model")
	}
	s.history = append(s.history, request.UserMessages...)
	arguments := s.arguments
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{"query":"current release","numResults":3}`)
	}
	return agent.StepResult{
		Result: agent.Result{SessionID: s.id, TurnID: "search-call"},
		ToolCalls: []agent.ToolCall{{
			ID: "call-search", Name: websearch.Name,
			Arguments: arguments,
		}},
	}, nil
}

func (s *searchCallingSession) State() (agent.SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(s.history)
	return agent.SessionState{ID: s.id, Provider: "future", Data: data}, err
}
