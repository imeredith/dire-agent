package client

import (
	"context"

	"github.com/imeredith/dire-agent/capability"
	"github.com/imeredith/dire-agent/daemon"
)

func (c *Client) CapabilityCommands(ctx context.Context, conversationID string) ([]daemon.CapabilityCommandInfo, error) {
	var result []daemon.CapabilityCommandInfo
	err := c.call(ctx, daemon.Command{Type: "list_capability_commands", ConversationID: conversationID}, &result)
	return result, err
}

func (c *Client) ExecuteCapabilityCommand(ctx context.Context, conversationID, name, arguments string) (capability.CommandResult, error) {
	var result capability.CommandResult
	err := c.call(ctx, daemon.Command{
		Type: "execute_capability_command", ConversationID: conversationID,
		CommandName: name, Arguments: arguments,
	}, &result)
	return result, err
}
