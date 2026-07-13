package client

import (
	"context"
	"time"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/daemon"
)

func (c *Client) SpawnAgent(ctx context.Context, request agentteam.SpawnRequest) (agentteam.Agent, error) {
	var result agentteam.Agent
	command := daemon.Command{
		Type: "spawn_agent", ConversationID: request.ParentID, ParentID: request.ParentID,
		AgentName: request.Name, AgentRole: request.Role, Profile: request.Profile,
		Task: request.Task, Model: request.Model, Level: request.Thinking, Tools: request.Tools,
	}
	err := c.call(ctx, command, &result)
	return result, err
}

func (c *Client) ListAgents(ctx context.Context, callerID string) ([]agentteam.Agent, error) {
	var result []agentteam.Agent
	err := c.call(ctx, daemon.Command{Type: "list_agents", ConversationID: callerID, ParentID: callerID}, &result)
	return result, err
}

func (c *Client) Agent(ctx context.Context, callerID, agentID string) (agentteam.Agent, error) {
	var result agentteam.Agent
	err := c.call(ctx, daemon.Command{Type: "get_agent", ConversationID: callerID, AgentID: agentID}, &result)
	return result, err
}

func (c *Client) SendAgentMessage(ctx context.Context, fromID, toID, message string, wake bool) (agentteam.Message, error) {
	var result agentteam.Message
	err := c.call(ctx, daemon.Command{
		Type: "send_agent_message", ConversationID: fromID, ParentID: fromID,
		AgentID: toID, Message: message, Wake: &wake,
	}, &result)
	return result, err
}

func (c *Client) WaitAgents(ctx context.Context, callerID string, ids []string, timeout time.Duration) (agentteam.WaitResult, error) {
	var result agentteam.WaitResult
	err := c.call(ctx, daemon.Command{
		Type: "wait_agents", ConversationID: callerID, ParentID: callerID,
		AgentIDs: ids, TimeoutMS: int(timeout / time.Millisecond),
	}, &result)
	return result, err
}

func (c *Client) InterruptAgent(ctx context.Context, callerID, agentID string) error {
	return c.call(ctx, daemon.Command{Type: "interrupt_agent", ConversationID: callerID, ParentID: callerID, AgentID: agentID}, nil)
}

func (c *Client) DeleteAgent(ctx context.Context, callerID, agentID string) error {
	return c.call(ctx, daemon.Command{Type: "delete_agent", ConversationID: callerID, ParentID: callerID, AgentID: agentID}, nil)
}
