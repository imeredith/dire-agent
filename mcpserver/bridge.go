// Package mcpserver exposes a Dire Agent daemon as a standard MCP server.
package mcpserver

import (
	"context"
	"time"

	"github.com/imeredith/dire-agent/agentteam"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

// Daemon is intentionally narrower than client.Client so the MCP bridge is
// deterministic in tests and independent of the WebSocket transport.
type Daemon interface {
	ListProjects(context.Context) ([]threadstore.Project, error)
	ListChats(context.Context) ([]threadstore.Chat, error)
	ListConversations(context.Context) ([]threadstore.Conversation, error)
	CreateProject(context.Context, daemon.CreateProjectOptions) (threadstore.Project, error)
	CreateChat(context.Context, daemon.CreateChatOptions) (threadstore.Chat, error)
	Conversation(context.Context, string) (threadstore.Conversation, error)
	State(context.Context, string) (daemon.RuntimeState, error)
	Messages(context.Context, string, int64, int) ([]threadstore.Message, error)
	Prompt(context.Context, string, string, string) error
	WaitForSettled(context.Context, string) (daemon.WireEvent, error)
	Abort(context.Context, string) error
	SpawnAgent(context.Context, agentteam.SpawnRequest) (agentteam.Agent, error)
	ListAgents(context.Context, string) ([]agentteam.Agent, error)
	SendAgentMessage(context.Context, string, string, string, bool) (agentteam.Message, error)
	WaitAgents(context.Context, string, []string, time.Duration) (agentteam.WaitResult, error)
	InterruptAgent(context.Context, string, string) error
	DeleteAgent(context.Context, string, string) error
}
