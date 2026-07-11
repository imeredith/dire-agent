package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) addRunTools() {
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_get_conversation", Description: "Get metadata for one standalone chat or project."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input conversationInput) (*mcp.CallToolResult, any, error) {
			value, err := s.daemon.Conversation(ctx, input.ConversationID)
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_get_state", Description: "Get run, queue, usage, context, skill, and capability state for a conversation."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input conversationInput) (*mcp.CallToolResult, any, error) {
			value, err := s.daemon.State(ctx, input.ConversationID)
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_get_messages", Description: "Read persisted user, assistant, and tool messages for a conversation."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input messagesInput) (*mcp.CallToolResult, any, error) {
			if input.Limit == 0 {
				input.Limit = 200
			}
			value, err := s.daemon.Messages(ctx, input.ConversationID, input.After, input.Limit)
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_send_message", Description: "Send a message to a Dire Agent conversation and, by default, wait for the agentic run to settle."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input sendInput) (*mcp.CallToolResult, any, error) {
			value, err := s.send(ctx, input)
			return toolResult(value, err)
		})
	mcp.AddTool(s.server, &mcp.Tool{Name: "dire_agent_abort", Description: "Abort the active agentic run for a conversation."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input conversationInput) (*mcp.CallToolResult, any, error) {
			err := s.daemon.Abort(ctx, input.ConversationID)
			return toolResult(map[string]bool{"aborted": err == nil}, err)
		})
}

func (s *Server) send(ctx context.Context, input sendInput) (sendOutput, error) {
	if input.ConversationID == "" || input.Message == "" {
		return sendOutput{}, errors.New("conversation_id and message are required")
	}
	wait := input.Wait == nil || *input.Wait
	if !wait {
		err := s.daemon.Prompt(ctx, input.ConversationID, input.Message, input.StreamingBehavior)
		return sendOutput{Accepted: err == nil}, err
	}
	// client.Client has one event stream. Serializing waiters prevents one MCP
	// call from consuming another conversation's settled event.
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	if err := s.daemon.Prompt(ctx, input.ConversationID, input.Message, input.StreamingBehavior); err != nil {
		return sendOutput{}, err
	}
	if _, err := s.daemon.WaitForSettled(ctx, input.ConversationID); err != nil {
		return sendOutput{Accepted: true}, err
	}
	state, err := s.daemon.State(ctx, input.ConversationID)
	if err != nil {
		return sendOutput{Accepted: true, Settled: true}, err
	}
	messages, err := s.daemon.Messages(ctx, input.ConversationID, 0, 200)
	return sendOutput{Accepted: true, Settled: true, State: state, Messages: messages}, err
}
