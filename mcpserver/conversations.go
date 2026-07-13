package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dire-kiwi/dire-agent/daemon"
)

func (s *Server) addConversationTools() {
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_list_conversations", Description: "List every persistent Dire Agent standalone chat and folder-scoped project."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
			value, err := s.daemon.ListConversations(ctx)
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_list_chats", Description: "List persistent standalone chats that are not tied to a folder."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
			value, err := s.daemon.ListChats(ctx)
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_list_projects", Description: "List persistent Dire Agent projects and their scoped folders."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
			value, err := s.daemon.ListProjects(ctx)
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_create_chat", Description: "Create a persistent standalone chat with no project folder or local file tools."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input createChatInput) (*mcp.CallToolResult, any, error) {
			value, err := s.daemon.CreateChat(ctx, daemon.CreateChatOptions{
				Name: input.Name, Model: input.Model, Instructions: input.Instructions, ThinkingLevel: input.ThinkingLevel,
			})
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_create_project", Description: "Create a persistent project in an existing folder or a managed detached Git worktree."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input createProjectInput) (*mcp.CallToolResult, any, error) {
			if input.Folder == "" && input.SourceProjectID == "" {
				return nil, nil, errors.New("folder or source_project_id is required")
			}
			options := daemon.CreateProjectOptions{
				Name: input.Name, Model: input.Model, CWD: input.Folder, Instructions: input.Instructions,
				ThinkingLevel: input.ThinkingLevel, Tools: input.Tools,
			}
			if input.Worktree || input.BaseRef != "" || input.EnvironmentID != "" || input.SourceProjectID != "" {
				options.Worktree = &daemon.CreateWorktreeOptions{
					BaseRef: input.BaseRef, EnvironmentID: input.EnvironmentID, SourceProjectID: input.SourceProjectID,
				}
			}
			value, err := s.daemon.CreateProject(ctx, options)
			return toolResult(value, err)
		})
}
