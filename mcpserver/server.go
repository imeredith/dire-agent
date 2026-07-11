package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	daemon   Daemon
	server   *mcp.Server
	promptMu sync.Mutex
}

func New(backend Daemon) (*Server, error) {
	if backend == nil {
		return nil, errors.New("mcpserver: daemon client is nil")
	}
	bridge := &Server{daemon: backend}
	server := mcp.NewServer(&mcp.Implementation{
		Name: "dire-agent", Title: "Dire Agent daemon", Version: "0.1.0",
	}, nil)
	bridge.server = server
	bridge.addConversationTools()
	bridge.addRunTools()
	bridge.addAgentTools()
	return bridge, nil
}

func (s *Server) MCP() *mcp.Server { return s.server }

func (s *Server) RunStdio(ctx context.Context) error {
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

func textResult(value any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("mcpserver: encode result: %w", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
}

func toolResult(value any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	result, err := textResult(value)
	return result, value, err
}

type emptyInput struct{}

type conversationInput struct {
	ConversationID string `json:"conversation_id" jsonschema:"ID of a standalone chat or folder-scoped project"`
}

type createChatInput struct {
	Name          string `json:"name,omitempty"`
	Model         string `json:"model,omitempty"`
	Instructions  string `json:"instructions,omitempty"`
	ThinkingLevel string `json:"thinking_level,omitempty"`
}

type createProjectInput struct {
	Folder          string   `json:"folder,omitempty" jsonschema:"absolute source-project folder; optional when source_project_id is supplied"`
	Name            string   `json:"name,omitempty"`
	Model           string   `json:"model,omitempty"`
	Instructions    string   `json:"instructions,omitempty"`
	ThinkingLevel   string   `json:"thinking_level,omitempty"`
	Tools           []string `json:"tools,omitempty"`
	Worktree        bool     `json:"worktree,omitempty" jsonschema:"create a managed detached Git worktree instead of using the source folder directly"`
	BaseRef         string   `json:"base_ref,omitempty" jsonschema:"Git ref for the managed worktree; defaults to HEAD"`
	EnvironmentID   string   `json:"environment_id,omitempty" jsonschema:"repo-local .codex environment ID whose setup script runs in the new worktree"`
	SourceProjectID string   `json:"source_project_id,omitempty" jsonschema:"existing Dire Agent project whose settings the worktree inherits"`
}

type messagesInput struct {
	ConversationID string `json:"conversation_id"`
	After          int64  `json:"after,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

type sendInput struct {
	ConversationID    string `json:"conversation_id"`
	Message           string `json:"message"`
	StreamingBehavior string `json:"streaming_behavior,omitempty" jsonschema:"empty for a new run, steer, or followUp"`
	Wait              *bool  `json:"wait,omitempty" jsonschema:"wait for the agent to settle; defaults to true"`
}

type sendOutput struct {
	Accepted bool `json:"accepted"`
	Settled  bool `json:"settled"`
	State    any  `json:"state,omitempty"`
	Messages any  `json:"messages,omitempty"`
}
