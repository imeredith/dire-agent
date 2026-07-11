package chatui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/agentteam"
	"github.com/imeredith/dire-agent/capability"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

func TestSlashCommandCompletion(t *testing.T) {
	t.Parallel()
	suggestions := slashCommandSuggestions("/thi")
	if len(suggestions) != 1 || suggestions[0].name != "thinking" {
		t.Fatalf("suggestions = %#v", suggestions)
	}
	if got := completeSlashCommand(suggestions[0]); got != "/thinking " {
		t.Fatalf("completion = %q", got)
	}
	if exact := slashCommandSuggestions("/help"); len(exact) != 0 {
		t.Fatalf("exact command still suggested: %#v", exact)
	}

	api := newFakeAPI()
	m := newModel(context.Background(), api, daemon.RuntimeState{Thread: api.thread}, nil, "")
	m.textarea.SetValue("/thi")
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m = updated.(model)
	if got := m.textarea.Value(); got != "/thinking " {
		t.Fatalf("Tab completion = %q", got)
	}
	m.textarea.SetValue("/he")
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(model)
	if got := m.textarea.Value(); got != "/help" {
		t.Fatalf("Enter completion = %q", got)
	}
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(model)
	if len(m.entries) == 0 || m.entries[len(m.entries)-1].role != "system" || !strings.Contains(m.entries[len(m.entries)-1].text, "/steer") {
		t.Fatalf("completed /help did not execute: %#v", m.entries)
	}
}

func TestReasoningAndToolUsageRenderInTranscript(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	m := newModel(context.Background(), api, daemon.RuntimeState{Thread: api.thread}, nil, "")
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "reasoning_start", map[string]any{"message_id": "reasoning-1"}))
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "reasoning_update", map[string]any{"message_id": "reasoning-1", "delta": "Inspecting the project."}))
	if got := m.renderTranscript(); !strings.Contains(got, "thinking") || !strings.Contains(got, "Inspecting the project") {
		t.Fatalf("live reasoning transcript = %q", got)
	}
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "reasoning_end", map[string]any{"message_id": "reasoning-1", "text": "Inspecting the project.\n\n<!-- -->"}))
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "tool_execution_end", map[string]any{
		"tool_call_id": "tool-1", "tool_name": "read", "arguments": map[string]any{"path": "README.md"}, "output": "documentation",
	}))
	transcript := m.renderTranscript()
	for _, want := range []string{"thinking", "Inspecting the project", "tool", "read", "input:", "README.md", "output:", "documentation"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("transcript %q missing %q", transcript, want)
		}
	}
	if strings.Contains(transcript, "<!--") {
		t.Fatalf("reasoning transcript leaked provider comment marker: %q", transcript)
	}
}

func TestParseInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		kind     string
		argument string
		wantErr  bool
	}{
		{"hello", "prompt", "hello", false},
		{"/steer focus on tests", "steer", "focus on tests", false},
		{"/followup next task", "follow-up", "next task", false},
		{"/follow-up next task", "follow-up", "next task", false},
		{"/abort", "abort", "", false},
		{"/agents", "agents", "", false},
		{"/spawn scout explore the tree", "spawn", "scout explore the tree", false},
		{"/message agent_1 status", "message", "agent_1 status", false},
		{"/wait agent_1 agent_2", "wait", "agent_1 agent_2", false},
		{"/interrupt agent_1", "interrupt", "agent_1", false},
		{"/commands", "capability-commands", "", false},
		{"/ext:demo:deploy staging", "capability-command", "ext:demo:deploy staging", false},
		{"/model", "model", "", false},
		{"/model gpt-test", "model", "gpt-test", false},
		{"/thinking HIGH", "thinking", "high", false},
		{"/folders", "folders", "", false},
		{"/folder-add /workspace/shared files", "folder-add", "/workspace/shared files", false},
		{"/folder-remove /workspace/shared", "folder-remove", "/workspace/shared", false},
		{"/help", "help", "", false},
		{"/skill:review changed files", "prompt", "/skill:review changed files", false},
		{"/quit", "quit", "", false},
		{"/steer", "", "", true},
		{"/unknown", "", "", true},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got := parseInput(test.input)
			if (got.err != nil) != test.wantErr || got.kind != test.kind || got.argument != test.argument {
				t.Fatalf("parseInput(%q) = %#v", test.input, got)
			}
		})
	}
}

func TestParseSpawnRequest(t *testing.T) {
	request, err := parseSpawnRequest("project_1", "reviewer review -- inspect authentication")
	if err != nil {
		t.Fatal(err)
	}
	if request.Name != "reviewer" || request.Profile != "review" || request.Task != "inspect authentication" {
		t.Fatalf("request = %+v", request)
	}
	request, err = parseSpawnRequest("project_1", "scout inspect files")
	if err != nil || request.Profile != "general" || request.Task != "inspect files" {
		t.Fatalf("default request = %+v, err=%v", request, err)
	}
}

func TestSubmitRoutesInteractiveCommands(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		running   bool
		wantCall  string
		wantValue string
	}{
		{name: "idle prompt", input: "hello", wantCall: "prompt", wantValue: "hello"},
		{name: "running prompt becomes follow-up", input: "next", running: true, wantCall: "follow-up", wantValue: "next"},
		{name: "steer", input: "/steer focus", running: true, wantCall: "steer", wantValue: "focus"},
		{name: "explicit follow-up", input: "/follow-up later", running: true, wantCall: "follow-up", wantValue: "later"},
		{name: "abort", input: "/abort", running: true, wantCall: "abort"},
		{name: "model", input: "/model gpt-new", wantCall: "model", wantValue: "gpt-new"},
		{name: "thinking", input: "/thinking high", wantCall: "thinking", wantValue: "high"},
		{name: "name", input: "/name demo task", wantCall: "name", wantValue: "demo task"},
		{name: "folder add", input: "/folder-add /workspace/shared", wantCall: "folders", wantValue: "/workspace/shared"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := newFakeAPI()
			state := daemon.RuntimeState{Thread: api.thread, Running: test.running}
			m := newModel(context.Background(), api, state, nil, "")
			m.textarea.SetValue(test.input)
			updated, command := m.submit()
			if command == nil {
				t.Fatal("submit returned no command")
			}
			result := command()
			if _, ok := result.(requestResultMsg); !ok {
				t.Fatalf("command result = %T, want requestResultMsg", result)
			}
			if api.call != test.wantCall || api.value != test.wantValue {
				t.Fatalf("call/value = %q/%q, want %q/%q", api.call, api.value, test.wantCall, test.wantValue)
			}
			if updated.(model).textarea.Value() != "" {
				t.Fatal("accepted input was not cleared")
			}
		})
	}
}

func TestStreamingEventBuildsTranscript(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	m := newModel(context.Background(), api, daemon.RuntimeState{Thread: api.thread}, nil, "")
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "message_start", map[string]any{"message_id": "m1"}))
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "message_update", map[string]any{"message_id": "m1", "delta": "hel"}))
	if !strings.Contains(m.renderTranscript(), "hel") {
		t.Fatalf("streaming transcript = %q", m.renderTranscript())
	}
	// Updating by value after the first delta catches state that is unsafe to
	// copy inside Bubble Tea models (for example, a non-zero strings.Builder).
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "message_update", map[string]any{"message_id": "m1", "delta": "lo"}))
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "message_end", map[string]any{"message_id": "m1", "text": "hello"}))
	if m.stream != "" || len(m.entries) != 1 || m.entries[0].role != "assistant" || m.entries[0].text != "hello" {
		t.Fatalf("completed transcript state = stream:%q entries:%#v", m.stream, m.entries)
	}
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "agent_settled", map[string]bool{"settled": true}))
	if m.running || m.status != "ready" {
		t.Fatalf("settled state = running:%v status:%q", m.running, m.status)
	}
	view := m.View()
	if !view.AltScreen || !strings.Contains(view.Content, "assistant") {
		t.Fatalf("View() = alt:%v content:%q", view.AltScreen, view.Content)
	}
}

func TestUsageEventsAndSummary(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.thread.Usage = agent.Usage{InputTokens: 100, ContextWindow: 372_000}
	m := newModel(context.Background(), api, daemon.RuntimeState{Thread: api.thread, Usage: api.thread.Usage}, nil, "")
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "message_end", map[string]any{
		"message_id": "m1",
		"usage": agent.Usage{
			InputTokens: 2_000, OutputTokens: 500, CacheReadTokens: 1_200, CacheWriteTokens: 300,
			TotalTokens: 2_500, ContextTokens: 2_500, ContextWindow: 372_000,
		},
	}))
	if m.thread.Usage.InputTokens != 2_100 || m.thread.Usage.CacheReadTokens != 1_200 || m.thread.Usage.ContextTokens != 2_500 {
		t.Fatalf("accumulated usage = %#v", m.thread.Usage)
	}
	canonical := agent.Usage{
		InputTokens: 8_000, OutputTokens: 900, CacheReadTokens: 4_000, CacheWriteTokens: 700,
		TotalTokens: 8_900, ContextTokens: 7_500, ContextWindow: 372_000,
	}
	m = updateWithEvent(m, wireEvent(t, api.thread.ID, "usage_update", canonical))
	if m.thread.Usage != canonical {
		t.Fatalf("canonical usage = %#v, want %#v", m.thread.Usage, canonical)
	}
	summary := usageSummary(m.thread.Usage)
	for _, want := range []string{"in 8k", "out 900", "cache read 4k", "write 700", "context 7.5k/372k"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("usage summary %q missing %q", summary, want)
		}
	}
}

func updateWithEvent(m model, event daemon.WireEvent) model {
	updated, _ := m.Update(eventMsg(event))
	return updated.(model)
}

func wireEvent(t *testing.T, threadID, eventType string, data any) daemon.WireEvent {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return daemon.WireEvent{Type: eventType, ThreadID: threadID, Data: raw}
}

type fakeAPI struct {
	thread threadstore.Thread
	events chan daemon.WireEvent
	call   string
	value  string
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		thread: threadstore.Thread{ID: "thread_test", Model: "gpt-test", ThinkingLevel: "medium", Status: "idle"},
		events: make(chan daemon.WireEvent, 10),
	}
}

func (f *fakeAPI) Events() <-chan daemon.WireEvent { return f.events }
func (f *fakeAPI) State(context.Context, string) (daemon.RuntimeState, error) {
	return daemon.RuntimeState{Thread: f.thread}, nil
}
func (f *fakeAPI) Messages(context.Context, string, int64, int) ([]threadstore.Message, error) {
	return nil, nil
}
func (f *fakeAPI) Subscribe(context.Context, string) error   { return nil }
func (f *fakeAPI) Unsubscribe(context.Context, string) error { return nil }
func (f *fakeAPI) Prompt(_ context.Context, _ string, value, _ string) error {
	f.call, f.value = "prompt", value
	return nil
}
func (f *fakeAPI) Steer(_ context.Context, _ string, value string) error {
	f.call, f.value = "steer", value
	return nil
}
func (f *fakeAPI) FollowUp(_ context.Context, _ string, value string) error {
	f.call, f.value = "follow-up", value
	return nil
}
func (f *fakeAPI) Abort(context.Context, string) error {
	f.call = "abort"
	return nil
}
func (f *fakeAPI) SetModel(_ context.Context, _ string, value string) (threadstore.Thread, error) {
	f.call, f.value = "model", value
	f.thread.Model = value
	return f.thread, nil
}
func (f *fakeAPI) SetThinkingLevel(_ context.Context, _ string, value string) (threadstore.Thread, error) {
	f.call, f.value = "thinking", value
	f.thread.ThinkingLevel = value
	return f.thread, nil
}
func (f *fakeAPI) SetThreadName(_ context.Context, _ string, value string) (threadstore.Thread, error) {
	f.call, f.value = "name", value
	f.thread.Name = value
	return f.thread, nil
}
func (f *fakeAPI) SetProjectAdditionalFolders(_ context.Context, _ string, folders []string) (threadstore.Project, error) {
	f.call, f.value = "folders", strings.Join(folders, "|")
	f.thread.AdditionalFolders = append([]string(nil), folders...)
	return f.thread, nil
}
func (f *fakeAPI) SpawnAgent(_ context.Context, request agentteam.SpawnRequest) (agentteam.Agent, error) {
	f.call, f.value = "spawn", request.Task
	return agentteam.Agent{ID: "agent_1", Name: request.Name, Profile: request.Profile, Status: "running"}, nil
}
func (f *fakeAPI) ListAgents(context.Context, string) ([]agentteam.Agent, error) {
	f.call = "agents"
	return []agentteam.Agent{{ID: "agent_1", Name: "scout", Status: "running"}}, nil
}
func (f *fakeAPI) SendAgentMessage(_ context.Context, _, _ string, value string, _ bool) (agentteam.Message, error) {
	f.call, f.value = "message", value
	return agentteam.Message{ID: "agentmsg_1"}, nil
}
func (f *fakeAPI) WaitAgents(context.Context, string, []string, time.Duration) (agentteam.WaitResult, error) {
	f.call = "wait"
	return agentteam.WaitResult{}, nil
}
func (f *fakeAPI) InterruptAgent(context.Context, string, string) error {
	f.call = "interrupt"
	return nil
}
func (f *fakeAPI) DeleteAgent(context.Context, string, string) error {
	f.call = "delete-agent"
	return nil
}
func (f *fakeAPI) CapabilityCommands(context.Context, string) ([]daemon.CapabilityCommandInfo, error) {
	f.call = "capability-commands"
	return []daemon.CapabilityCommandInfo{{Name: "ext:demo:deploy"}}, nil
}
func (f *fakeAPI) ExecuteCapabilityCommand(_ context.Context, _, name, arguments string) (capability.CommandResult, error) {
	f.call, f.value = name, arguments
	return capability.CommandResult{Output: "done"}, nil
}
