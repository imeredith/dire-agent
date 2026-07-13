package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentloop"
)

func (c *Client) ContextTools() map[string]agentloop.Tool {
	result := make(map[string]agentloop.Tool)
	for _, server := range c.serverNames() {
		runtime := c.servers[server]
		runtime.mu.RLock()
		session := runtime.session
		initialized := sessionInitializeResult(session)
		_, resourcesSupported := session.(resourceSession)
		_, promptsSupported := session.(promptSession)
		runtime.mu.RUnlock()
		if initialized == nil || initialized.Capabilities == nil {
			continue
		}
		if initialized.Capabilities.Resources != nil && resourcesSupported {
			result[ContextToolName(server, "list_resources")] = newListResourcesTool(c, server)
			result[ContextToolName(server, "read_resource")] = newReadResourceTool(c, server)
		}
		if initialized.Capabilities.Prompts != nil && promptsSupported {
			result[ContextToolName(server, "list_prompts")] = newListPromptsTool(c, server)
			result[ContextToolName(server, "get_prompt")] = newGetPromptTool(c, server)
		}
	}
	return result
}

type contextTool struct {
	definition agent.ToolDefinition
	execute    func(context.Context, json.RawMessage) (any, error)
	maxBytes   int
}

func (t *contextTool) Definition() agent.ToolDefinition { return cloneDefinition(t.definition) }
func (t *contextTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	result, err := t.execute(ctx, raw)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	output, truncated := truncateUTF8(string(encoded), t.maxBytes)
	if truncated {
		output += "\n[MCP context result truncated]"
	}
	return output, nil
}

func newContextTool(client *Client, server, operation, description, schema string, execute func(context.Context, json.RawMessage) (any, error)) *contextTool {
	return &contextTool{definition: agent.ToolDefinition{
		Name: ContextToolName(server, operation), Description: description, Parameters: json.RawMessage(schema),
	}, execute: execute, maxBytes: client.options.MaxResultBytes}
}

func newListResourcesTool(client *Client, server string) *contextTool {
	return newContextTool(client, server, "list_resources", "List resources exposed by the "+server+" MCP server.", `{"type":"object","additionalProperties":false}`, func(ctx context.Context, raw json.RawMessage) (any, error) {
		if err := decodeContextInput(raw, &struct{}{}); err != nil {
			return nil, err
		}
		return client.ListResources(ctx, server)
	})
}

func newReadResourceTool(client *Client, server string) *contextTool {
	return newContextTool(client, server, "read_resource", "Read a resource exposed by the "+server+" MCP server.", `{"type":"object","properties":{"uri":{"type":"string"}},"required":["uri"],"additionalProperties":false}`, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var input struct {
			URI string `json:"uri"`
		}
		if err := decodeContextInput(raw, &input); err != nil {
			return nil, err
		}
		return client.ReadResource(ctx, server, input.URI)
	})
}

func newListPromptsTool(client *Client, server string) *contextTool {
	return newContextTool(client, server, "list_prompts", "List prompt templates exposed by the "+server+" MCP server.", `{"type":"object","additionalProperties":false}`, func(ctx context.Context, raw json.RawMessage) (any, error) {
		if err := decodeContextInput(raw, &struct{}{}); err != nil {
			return nil, err
		}
		return client.ListPrompts(ctx, server)
	})
}

func newGetPromptTool(client *Client, server string) *contextTool {
	return newContextTool(client, server, "get_prompt", "Render a prompt template exposed by the "+server+" MCP server.", `{"type":"object","properties":{"name":{"type":"string"},"arguments":{"type":"object","additionalProperties":{"type":"string"}}},"required":["name"],"additionalProperties":false}`, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var input struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments"`
		}
		if err := decodeContextInput(raw, &input); err != nil {
			return nil, err
		}
		return client.GetPrompt(ctx, server, input.Name, input.Arguments)
	})
}

func ContextToolName(server, operation string) string { return "mcpctx__" + server + "__" + operation }

func decodeContextInput(raw json.RawMessage, destination any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("MCP context tool input must contain one object")
	}
	return nil
}
