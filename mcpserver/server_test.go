package mcpserver_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/mcpserver"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestBridgeListsCreatesAndRunsChats(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	backend := &fakeDaemon{}
	bridge, err := mcpserver.New(backend)
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := bridge.MCP().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	created, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "dire_agent_create_chat", Arguments: map[string]any{"name": "Desktop chat"},
	})
	if err != nil || created.IsError {
		t.Fatalf("create: result=%+v err=%v", created, err)
	}
	project, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "dire_agent_create_project", Arguments: map[string]any{
			"name": "Isolated task", "worktree": true,
			"base_ref": "main", "environment_id": "node.toml", "source_project_id": "project_source",
		},
	})
	if err != nil || project.IsError {
		t.Fatalf("create worktree project: result=%+v err=%v", project, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "dire_agent_send_message",
		Arguments: map[string]any{"conversation_id": "chat_1", "message": "hello"},
	})
	if err != nil || result.IsError {
		t.Fatalf("send: result=%+v err=%v", result, err)
	}
	spawned, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "dire_agent_spawn_agent", Arguments: map[string]any{
			"parent_id": "chat_1", "name": "researcher", "task": "inspect the topic",
		},
	})
	if err != nil || spawned.IsError {
		t.Fatalf("spawn: result=%+v err=%v", spawned, err)
	}
	listed, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "dire_agent_list_agents", Arguments: map[string]any{"conversation_id": "chat_1"},
	})
	if err != nil || listed.IsError {
		t.Fatalf("list agents: result=%+v err=%v", listed, err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.prompt != "hello" || len(backend.messages) != 2 {
		t.Fatalf("backend = prompt %q messages %+v", backend.prompt, backend.messages)
	}
	if backend.projectOptions.Worktree == nil || backend.projectOptions.Worktree.BaseRef != "main" ||
		backend.projectOptions.Worktree.EnvironmentID != "node.toml" || backend.projectOptions.Worktree.SourceProjectID != "project_source" ||
		backend.projectOptions.CWD != "" {
		t.Fatalf("worktree project options = %#v", backend.projectOptions)
	}
}

type fakeDaemon struct {
	mu             sync.Mutex
	chat           threadstore.Chat
	prompt         string
	messages       []threadstore.Message
	agent          agentteam.Agent
	projectOptions daemon.CreateProjectOptions
}

func (f *fakeDaemon) ListProjects(context.Context) ([]threadstore.Project, error) { return nil, nil }
func (f *fakeDaemon) ListChats(context.Context) ([]threadstore.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.chat.ID == "" {
		return nil, nil
	}
	return []threadstore.Chat{f.chat}, nil
}
func (f *fakeDaemon) ListConversations(ctx context.Context) ([]threadstore.Conversation, error) {
	return f.ListChats(ctx)
}

func (f *fakeDaemon) CreateProject(_ context.Context, options daemon.CreateProjectOptions) (threadstore.Project, error) {
	f.mu.Lock()
	f.projectOptions = options
	f.mu.Unlock()
	return threadstore.Project{ID: "project_1", Kind: threadstore.KindProject}, nil
}
func (f *fakeDaemon) CreateChat(_ context.Context, options daemon.CreateChatOptions) (threadstore.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chat = threadstore.Chat{ID: "chat_1", Kind: threadstore.KindChat, Name: options.Name, Model: "gpt-5.6"}
	return f.chat, nil
}
func (f *fakeDaemon) Conversation(context.Context, string) (threadstore.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chat, nil
}
func (f *fakeDaemon) State(context.Context, string) (daemon.RuntimeState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return daemon.RuntimeState{Kind: threadstore.KindChat, Conversation: f.chat, Chat: f.chat, Usage: agent.Usage{}}, nil
}
func (f *fakeDaemon) Messages(context.Context, string, int64, int) ([]threadstore.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]threadstore.Message(nil), f.messages...), nil
}
func (f *fakeDaemon) Prompt(_ context.Context, _ string, message, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompt = message
	f.messages = []threadstore.Message{{Role: "user", Content: message}, {Role: "assistant", Content: "done"}}
	return nil
}
func (f *fakeDaemon) WaitForSettled(context.Context, string) (daemon.WireEvent, error) {
	return daemon.WireEvent{Type: "agent_settled", ConversationID: "chat_1"}, nil
}
func (f *fakeDaemon) Abort(context.Context, string) error { return nil }
func (f *fakeDaemon) SpawnAgent(_ context.Context, request agentteam.SpawnRequest) (agentteam.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agent = agentteam.Agent{ID: "agent_1", ParentID: request.ParentID, RootID: request.ParentID, Name: request.Name, Status: "running"}
	return f.agent, nil
}
func (f *fakeDaemon) ListAgents(context.Context, string) ([]agentteam.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.agent.ID == "" {
		return nil, nil
	}
	return []agentteam.Agent{f.agent}, nil
}
func (f *fakeDaemon) SendAgentMessage(_ context.Context, from, to, message string, _ bool) (agentteam.Message, error) {
	return agentteam.Message{ID: "agentmsg_1", FromID: from, ToID: to, Content: message}, nil
}
func (f *fakeDaemon) WaitAgents(context.Context, string, []string, time.Duration) (agentteam.WaitResult, error) {
	return agentteam.WaitResult{Agents: []agentteam.Agent{f.agent}}, nil
}
func (f *fakeDaemon) InterruptAgent(context.Context, string, string) error { return nil }
func (f *fakeDaemon) DeleteAgent(context.Context, string, string) error    { return nil }
