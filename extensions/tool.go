package extensions

import (
	"context"
	"encoding/json"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/agentloop"
)

type agentTool struct {
	client *Client
	remote ToolSpec
	model  string
}

// AgentTools adapts the current extension tool snapshot to agentloop.Tool.
// RefreshTools may update future snapshots without mutating returned tools.
func (c *Client) AgentTools() map[string]agentloop.Tool {
	remote := c.ListTools()
	tools := make(map[string]agentloop.Tool, len(remote))
	for _, spec := range remote {
		name := ModelName(c.id, spec.Name)
		tools[name] = &agentTool{client: c, remote: cloneTool(spec), model: name}
	}
	return tools
}

func (t *agentTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name: t.model, Description: t.remote.Description,
		Parameters: append(json.RawMessage(nil), t.remote.InputSchema...),
	}
}

func (t *agentTool) Execute(ctx context.Context, arguments json.RawMessage) (string, error) {
	result, err := t.client.CallTool(ctx, t.remote.Name, arguments)
	if err != nil {
		return result.Output, err
	}
	if result.IsError {
		return result.Output, ErrToolReported
	}
	return result.Output, nil
}
