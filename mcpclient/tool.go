package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentloop"
)

type discoveredTool struct {
	remoteName string
	modelName  string
	definition agent.ToolDefinition
	status     ToolStatus
}

func newDiscoveredTool(server string, tool *mcp.Tool) (*discoveredTool, error) {
	if tool == nil {
		return nil, errors.New("server returned a null tool")
	}
	modelName, err := ModelName(server, tool.Name)
	if err != nil {
		return nil, err
	}
	schema := tool.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	parameters, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("tool %q input schema: %w", tool.Name, err)
	}
	var object map[string]any
	if err := json.Unmarshal(parameters, &object); err != nil || object == nil {
		return nil, fmt.Errorf("tool %q input schema is not a JSON object", tool.Name)
	}
	return &discoveredTool{
		remoteName: tool.Name,
		modelName:  modelName,
		definition: agent.ToolDefinition{Name: modelName, Description: tool.Description, Parameters: parameters},
		status:     ToolStatus{Server: server, Name: tool.Name, ModelName: modelName, Description: tool.Description, Available: true},
	}, nil
}

// AgentTools returns an immutable snapshot suitable for agentloop.Config.
func (c *Client) AgentTools() map[string]agentloop.Tool {
	tools := make(map[string]agentloop.Tool)
	for _, runtime := range c.servers {
		runtime.mu.RLock()
		for name, discovered := range runtime.tools {
			if !discovered.status.Available {
				continue
			}
			tools[name] = &agentTool{client: c, modelName: name, definition: cloneDefinition(discovered.definition)}
		}
		runtime.mu.RUnlock()
	}
	return tools
}

type agentTool struct {
	client     *Client
	modelName  string
	definition agent.ToolDefinition
}

func (t *agentTool) Definition() agent.ToolDefinition {
	return cloneDefinition(t.definition)
}

func (t *agentTool) Execute(ctx context.Context, arguments json.RawMessage) (string, error) {
	result, err := t.client.CallTool(ctx, t.modelName, arguments)
	if err != nil {
		return "", err
	}
	if result.IsError {
		if result.Output == ErrToolFailed.Error() {
			return "", ErrToolFailed
		}
		return result.Output, ErrToolFailed
	}
	return result.Output, nil
}

func cloneDefinition(definition agent.ToolDefinition) agent.ToolDefinition {
	definition.Parameters = append(json.RawMessage(nil), definition.Parameters...)
	return definition
}

// CallTool invokes a discovered tool by its model-facing name.
func (c *Client) CallTool(ctx context.Context, modelName string, raw json.RawMessage) (Result, error) {
	if c.isClosed() {
		return Result{}, ErrClosed
	}
	runtime, tool := c.findTool(modelName)
	if runtime == nil || tool == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrUnknownTool, modelName)
	}
	arguments := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &arguments); err != nil {
			return Result{}, fmt.Errorf("MCP tool %q arguments: %w", modelName, err)
		}
		if arguments == nil {
			arguments = map[string]any{}
		}
	}
	runtime.mu.RLock()
	session := runtime.session
	remoteName := tool.remoteName
	runtime.mu.RUnlock()
	if session == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrNotConnected, runtime.config.Name)
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout(runtime.config.CallTimeout, c.options.CallTimeout))
	defer cancel()
	response, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: remoteName, Arguments: arguments})
	calledAt := time.Now().UTC()
	if err != nil {
		safe := safeError(runtime.config, "calling tool", err)
		c.recordCall(runtime, modelName, calledAt, safe.Error(), StateDegraded)
		return Result{}, safe
	}
	if response == nil {
		err := safeError(runtime.config, "calling tool", errors.New("tools/call returned no result"))
		c.recordCall(runtime, modelName, calledAt, err.Error(), StateDegraded)
		return Result{}, err
	}
	result := flattenResult(response, c.options.MaxResultBytes, c.options.MaxStructuredBytes)
	message := ""
	if result.IsError {
		message = ErrToolFailed.Error()
	}
	c.recordCall(runtime, modelName, calledAt, message, StateReady)
	return result, nil
}

func (c *Client) findTool(modelName string) (*serverRuntime, *discoveredTool) {
	for _, runtime := range c.servers {
		runtime.mu.RLock()
		tool := runtime.tools[modelName]
		runtime.mu.RUnlock()
		if tool != nil {
			return runtime, tool
		}
	}
	return nil, nil
}

func (c *Client) recordCall(runtime *serverRuntime, modelName string, at time.Time, message string, state State) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.status.State == StateClosed {
		return
	}
	if tool := runtime.tools[modelName]; tool != nil {
		tool.status.LastCalledAt = at
		tool.status.LastError = message
	}
	runtime.status.State = state
	if state == StateDegraded {
		runtime.status.LastError = message
	} else {
		runtime.status.LastError = ""
	}
}
