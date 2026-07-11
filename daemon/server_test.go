package daemon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/client"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

func TestWebSocketAgentLoopAndProjectPersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "input.txt"), []byte("stored value"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultCWD: root, DefaultModel: "fake-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&daemon.Server{Manager: manager}).Handler())
	websocketURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	daemonClient, err := client.Dial(ctx, websocketURL)
	if err != nil {
		t.Fatal(err)
	}

	project, err := daemonClient.CreateProject(ctx, daemon.CreateProjectOptions{
		CWD: root, Model: "fake-model", Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(project.ID, "project_") {
		t.Fatalf("project id = %q, want project_ prefix", project.ID)
	}
	if err := daemonClient.Prompt(ctx, project.ID, "read the file", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := daemonClient.WaitForSettled(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
	messages, err := daemonClient.ProjectMessages(ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var sawUser, sawTool, sawReasoning, sawAssistant bool
	for _, message := range messages {
		switch {
		case message.Role == "user":
			sawUser = true
		case message.Role == "tool" && strings.Contains(message.Content, "stored value"):
			sawTool = true
			if !strings.Contains(string(message.Data), `"path":"input.txt"`) {
				t.Fatalf("persisted tool message lost its arguments: %s", message.Data)
			}
		case message.Role == "reasoning" && strings.Contains(message.Content, "Checking the tool result"):
			sawReasoning = true
		case message.Role == "assistant" && strings.Contains(message.Content, "stored value"):
			sawAssistant = true
		}
	}
	if !sawUser || !sawTool || !sawReasoning || !sawAssistant {
		t.Fatalf("messages missing lifecycle entries: %#v", messages)
	}
	projectState, err := daemonClient.ProjectState(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantUsage := agent.Usage{
		InputTokens: 240, OutputTokens: 30, CacheReadTokens: 150, CacheWriteTokens: 13,
		TotalTokens: 270, ContextTokens: 160, ContextWindow: 372_000,
	}
	if projectState.Usage != wantUsage || projectState.Project.Usage != wantUsage {
		t.Fatalf("project usage = state:%#v metadata:%#v, want %#v", projectState.Usage, projectState.Project.Usage, wantUsage)
	}
	if _, err := os.Stat(filepath.Join(store.Directory(), project.ID+".db")); err != nil {
		t.Fatalf("per-project database: %v", err)
	}

	if err := daemonClient.Close(); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	provider2 := &fakeProvider{}
	manager2, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider2, DefaultCWD: root, DefaultModel: "fake-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager2.Close()
	state, err := manager2.State(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Thread.ID != project.ID || provider2.restored.Load() != 1 {
		t.Fatalf("restored state/provider count = %#v/%d", state, provider2.restored.Load())
	}
	if state.Usage != wantUsage {
		t.Fatalf("restored usage = %#v, want %#v", state.Usage, wantUsage)
	}
}

func TestProjectFolderIsCanonicalAndGPT56IsAvailable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	projectFolder := filepath.Join(root, "work")
	if err := os.Mkdir(projectFolder, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "work-link")
	if err := os.Symlink(projectFolder, link); err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: link})
	if err != nil {
		t.Fatal(err)
	}
	wantFolder, err := filepath.EvalSymlinks(projectFolder)
	if err != nil {
		t.Fatal(err)
	}
	if project.CWD != wantFolder {
		t.Fatalf("project folder = %q, want canonical %q", project.CWD, wantFolder)
	}
	if project.Model != "gpt-5.6" {
		t.Fatalf("default model = %q, want gpt-5.6", project.Model)
	}
	found := false
	for _, model := range manager.AvailableModels() {
		if model.ID == "gpt-5.6" {
			found = model.ContextWindow == 372_000
		}
	}
	if !found {
		t.Fatalf("available models missing GPT-5.6 with context metadata: %#v", manager.AvailableModels())
	}
	file := filepath.Join(root, "not-a-folder")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: file}); err == nil {
		t.Fatal("CreateProject accepted a file as its folder")
	}
}

func TestCustomProviderModelRegistryDoesNotInjectCodexModels(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root,
		DefaultProvider: "example", DefaultModel: "example-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	models := manager.AvailableModels()
	if len(models) != 1 || models[0].Provider != "example" || models[0].ID != "example-model" {
		t.Fatalf("custom provider models = %#v", models)
	}
}

func TestIdleSteeringAndInvalidSettingsAreRejected(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "threads"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root, DefaultModel: "fake-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	thread, err := manager.CreateThread(ctx, daemon.CreateThreadOptions{Name: "original"})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Steer(ctx, thread.ID, "too early"); err == nil {
		t.Fatal("Steer() on an idle thread succeeded")
	}
	changedName := "must-not-stick"
	invalidLevel := "extreme"
	if _, err := manager.UpdateSettings(ctx, thread.ID, daemon.SettingsUpdate{
		Name: &changedName, ThinkingLevel: &invalidLevel,
	}); err == nil {
		t.Fatal("UpdateSettings() accepted an invalid thinking level")
	}
	stored, err := manager.Thread(ctx, thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "original" {
		t.Fatalf("invalid update partially changed name to %q", stored.Name)
	}
}

type fakeProvider struct {
	next     atomic.Int64
	restored atomic.Int64
}

func (p *fakeProvider) OpenSession(_ context.Context, _ agent.SessionOptions) (agent.Session, error) {
	id := fmt.Sprintf("fake-%d", p.next.Add(1))
	return &fakeSession{id: id}, nil
}

func (p *fakeProvider) OpenSessionFromState(_ context.Context, _ agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	p.restored.Add(1)
	var history []string
	if len(state.Data) != 0 {
		_ = json.Unmarshal(state.Data, &history)
	}
	return &fakeSession{id: state.ID, history: history}, nil
}

func (p *fakeProvider) Close() error { return nil }

type fakeSession struct {
	id      string
	mu      sync.Mutex
	history []string
}

func (s *fakeSession) ID() string { return s.id }
func (s *fakeSession) Run(ctx context.Context, prompt string) (agent.Result, error) {
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	return step.Result, err
}
func (s *fakeSession) Step(_ context.Context, request agent.StepRequest) (agent.StepResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(request.ToolResults) != 0 {
		answer := "tool returned: " + request.ToolResults[0].Output
		s.history = append(s.history, answer)
		if request.OnEvent != nil {
			request.OnEvent(agent.ModelEvent{Type: "reasoning_delta", Delta: "Checking the tool result."})
			request.OnEvent(agent.ModelEvent{Type: "reasoning_done", Text: "Checking the tool result."})
			request.OnEvent(agent.ModelEvent{Type: "text_delta", Delta: answer})
		}
		return agent.StepResult{Result: agent.Result{
			Text: answer, SessionID: s.id, TurnID: "turn-final",
			Usage: agent.Usage{
				InputTokens: 140, OutputTokens: 20, CacheReadTokens: 90, CacheWriteTokens: 8,
				TotalTokens: 160, ContextTokens: 160, ContextWindow: 372_000,
			},
		}}, nil
	}
	if len(request.UserMessages) != 0 {
		s.history = append(s.history, request.UserMessages...)
	}
	return agent.StepResult{
		Result: agent.Result{
			SessionID: s.id, TurnID: "turn-tool",
			Usage: agent.Usage{
				InputTokens: 100, OutputTokens: 10, CacheReadTokens: 60, CacheWriteTokens: 5,
				TotalTokens: 110, ContextTokens: 110, ContextWindow: 372_000,
			},
		},
		ToolCalls: []agent.ToolCall{{ID: "call-read", Name: "read", Arguments: json.RawMessage(`{"path":"input.txt"}`)}},
	}, nil
}
func (s *fakeSession) State() (agent.SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(s.history)
	return agent.SessionState{ID: s.id, Provider: "fake", Data: data}, err
}
