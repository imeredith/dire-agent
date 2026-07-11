package mcpserver

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/imeredith/dire-agent/agentteam"
)

type listAgentsInput struct {
	ConversationID string `json:"conversation_id" jsonschema:"root or child conversation whose team should be listed"`
}

type spawnAgentInput struct {
	ParentID string   `json:"parent_id"`
	Name     string   `json:"name"`
	Profile  string   `json:"profile,omitempty"`
	Role     string   `json:"role,omitempty"`
	Task     string   `json:"task"`
	Model    string   `json:"model,omitempty"`
	Thinking string   `json:"thinking,omitempty"`
	Tools    []string `json:"tools,omitempty"`
}

type agentMessageInput struct {
	FromID  string `json:"from_id"`
	AgentID string `json:"agent_id"`
	Message string `json:"message"`
	Wake    *bool  `json:"wake,omitempty"`
}

type waitAgentsInput struct {
	CallerID  string   `json:"caller_id"`
	AgentIDs  []string `json:"agent_ids,omitempty"`
	TimeoutMS int      `json:"timeout_ms,omitempty"`
}

type targetAgentInput struct {
	CallerID string `json:"caller_id"`
	AgentID  string `json:"agent_id"`
}

func (s *Server) addAgentTools() {
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_list_agents", Description: "List the persistent root and child agents in a Dire Agent conversation team."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input listAgentsInput) (*mcp.CallToolResult, any, error) {
			value, err := s.daemon.ListAgents(ctx, input.ConversationID)
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_spawn_agent", Description: "Spawn a bounded persistent child agent with inherited project and tool permissions."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input spawnAgentInput) (*mcp.CallToolResult, any, error) {
			value, err := s.daemon.SpawnAgent(ctx, agentteam.SpawnRequest{
				ParentID: input.ParentID, Name: input.Name, Profile: input.Profile, Role: input.Role,
				Task: input.Task, Model: input.Model, Thinking: input.Thinking, Tools: input.Tools,
			})
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_send_agent_message", Description: "Send a durable message between agents in one team and optionally wake the recipient."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input agentMessageInput) (*mcp.CallToolResult, any, error) {
			wake := input.Wake == nil || *input.Wake
			value, err := s.daemon.SendAgentMessage(ctx, input.FromID, input.AgentID, input.Message, wake)
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_wait_agents", Description: "Wait up to 60 seconds for selected child agents to finish or send messages."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input waitAgentsInput) (*mcp.CallToolResult, any, error) {
			timeout := time.Duration(input.TimeoutMS) * time.Millisecond
			value, err := s.daemon.WaitAgents(ctx, input.CallerID, input.AgentIDs, timeout)
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_interrupt_agent", Description: "Interrupt a running child agent without deleting its history."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input targetAgentInput) (*mcp.CallToolResult, any, error) {
			err := validateAgentTarget(input)
			if err == nil {
				err = s.daemon.InterruptAgent(ctx, input.CallerID, input.AgentID)
			}
			return toolResult(map[string]bool{"interrupted": err == nil}, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_delete_agent", Description: "Delete an idle leaf child agent and its SQLite history."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input targetAgentInput) (*mcp.CallToolResult, any, error) {
			err := validateAgentTarget(input)
			if err == nil {
				err = s.daemon.DeleteAgent(ctx, input.CallerID, input.AgentID)
			}
			return toolResult(map[string]bool{"deleted": err == nil}, err)
		})
}

func validateAgentTarget(input targetAgentInput) error {
	if input.CallerID == "" || input.AgentID == "" {
		return errors.New("caller_id and agent_id are required")
	}
	return nil
}
