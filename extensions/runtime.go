package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Client struct {
	id           string
	peer         PeerInfo
	connection   Connection
	limits       Limits
	cancel       context.CancelFunc
	closed       atomic.Bool
	mu           sync.RWMutex
	tools        map[string]ToolSpec
	registration Registration
}

type initializeParams struct {
	ProtocolVersion string   `json:"protocol_version"`
	Client          PeerInfo `json:"client"`
	ExtensionID     string   `json:"extension_id"`
}

type initializeResult struct {
	ProtocolVersion string       `json:"protocol_version"`
	Server          PeerInfo     `json:"server"`
	Registration    Registration `json:"registration,omitempty"`
}

type listToolsResult struct {
	Tools []ToolSpec `json:"tools"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Open starts and initializes an extension adapter. ctx owns the process
// lifetime; callers must also call Close for orderly shutdown.
func Open(ctx context.Context, config LaunchConfig, options OpenOptions) (*Client, error) {
	if !config.Enabled {
		return nil, ErrDisabled
	}
	if config.Trust != TrustTrusted {
		return nil, ErrUntrusted
	}
	if config.ID == "" {
		return nil, errors.New("extensions: id is required")
	}
	if err := validateProcessSpec(config.Process, false); err != nil {
		return nil, fmt.Errorf("extensions: invalid process: %w", err)
	}
	limits := defaultLimits(options.Limits)
	connector := options.Connector
	if connector == nil {
		connector = StdioConnector{}
	}
	peer := options.Client
	if peer.Name == "" {
		peer = PeerInfo{Name: "dire-agent", Version: "1"}
	}
	lifetime, cancel := context.WithCancel(ctx)
	connection, err := connector.Connect(lifetime, config.Process, limits)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("extensions: start %s: %w", config.ID, err)
	}
	client := &Client{
		id: normalizeID(config.ID), peer: peer, connection: connection,
		limits: limits, cancel: cancel, tools: map[string]ToolSpec{},
	}
	if err := client.initialize(ctx); err != nil {
		client.abort()
		return nil, err
	}
	if err := client.RefreshTools(ctx); err != nil {
		client.abort()
		return nil, err
	}
	return client, nil
}

func (c *Client) initialize(ctx context.Context) error {
	requestCtx, cancel := withTimeout(ctx, c.limits.InitializeTimeout)
	defer cancel()
	var result initializeResult
	err := c.connection.Call(requestCtx, "initialize", initializeParams{
		ProtocolVersion: ProtocolVersion, Client: c.peer, ExtensionID: c.id,
	}, &result)
	if err != nil {
		return fmt.Errorf("extensions: initialize %s: %w", c.id, err)
	}
	if result.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("extensions: %s uses unsupported protocol %q", c.id, result.ProtocolVersion)
	}
	if err := validateRegistration(result.Registration, c.limits); err != nil {
		return fmt.Errorf("extensions: initialize %s: %w", c.id, err)
	}
	c.mu.Lock()
	c.registration = cloneRegistration(result.Registration)
	c.mu.Unlock()
	return nil
}

func (c *Client) RefreshTools(ctx context.Context) error {
	if c.closed.Load() {
		return ErrClosed
	}
	requestCtx, cancel := withTimeout(ctx, c.limits.CallTimeout)
	defer cancel()
	var result listToolsResult
	if err := c.connection.Call(requestCtx, "list_tools", struct{}{}, &result); err != nil {
		return fmt.Errorf("extensions: list tools for %s: %w", c.id, err)
	}
	if len(result.Tools) > c.limits.MaxTools {
		return fmt.Errorf("extensions: %s returned %d tools; maximum is %d", c.id, len(result.Tools), c.limits.MaxTools)
	}
	tools := make(map[string]ToolSpec, len(result.Tools))
	modelNames := map[string]string{}
	for _, tool := range result.Tools {
		if err := validateTool(tool); err != nil {
			return fmt.Errorf("extensions: %s: %w", c.id, err)
		}
		if _, exists := tools[tool.Name]; exists {
			return fmt.Errorf("extensions: %s returned duplicate tool %q", c.id, tool.Name)
		}
		modelName := ModelName(c.id, tool.Name)
		if previous, exists := modelNames[modelName]; exists {
			return fmt.Errorf("extensions: tools %q and %q map to %q", previous, tool.Name, modelName)
		}
		modelNames[modelName] = tool.Name
		tools[tool.Name] = cloneTool(tool)
	}
	c.mu.Lock()
	c.tools = tools
	c.mu.Unlock()
	return nil
}

func (c *Client) ListTools() []ToolSpec {
	c.mu.RLock()
	tools := make([]ToolSpec, 0, len(c.tools))
	for _, tool := range c.tools {
		tools = append(tools, cloneTool(tool))
	}
	c.mu.RUnlock()
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (ToolResult, error) {
	if c.closed.Load() {
		return ToolResult{}, ErrClosed
	}
	c.mu.RLock()
	_, exists := c.tools[name]
	c.mu.RUnlock()
	if !exists {
		return ToolResult{}, fmt.Errorf("extensions: unknown tool %q", name)
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(arguments) {
		return ToolResult{}, errors.New("extensions: tool arguments are invalid JSON")
	}
	if len(arguments) > c.limits.MaxMessageBytes {
		return ToolResult{}, fmt.Errorf("extensions: tool arguments exceed %d bytes", c.limits.MaxMessageBytes)
	}
	requestCtx, cancel := withTimeout(ctx, c.limits.CallTimeout)
	defer cancel()
	var result ToolResult
	err := c.connection.Call(requestCtx, "call_tool", callToolParams{Name: name, Arguments: arguments}, &result)
	result.Output = truncate(result.Output, c.limits.MaxOutputBytes)
	if err != nil {
		return result, fmt.Errorf("extensions: call %s/%s: %w", c.id, name, err)
	}
	return result, nil
}

func (c *Client) ID() string     { return c.id }
func (c *Client) Stderr() string { return c.connection.Stderr() }

func (c *Client) Close(ctx context.Context) error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	closeCtx, cancel := withTimeout(ctx, c.limits.CloseTimeout)
	defer cancel()
	shutdownErr := c.connection.Call(closeCtx, "shutdown", struct{}{}, &struct{}{})
	c.cancel()
	closeErr := c.connection.Close(closeCtx)
	return errors.Join(shutdownErr, closeErr)
}

func (c *Client) abort() {
	c.closed.Store(true)
	c.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), c.limits.CloseTimeout)
	defer cancel()
	_ = c.connection.Close(ctx)
}

func defaultLimits(limits Limits) Limits {
	if limits.InitializeTimeout <= 0 {
		limits.InitializeTimeout = 10 * time.Second
	}
	if limits.CallTimeout <= 0 {
		limits.CallTimeout = 60 * time.Second
	}
	if limits.CloseTimeout <= 0 {
		limits.CloseTimeout = 2 * time.Second
	}
	if limits.MaxMessageBytes <= 0 {
		limits.MaxMessageBytes = 2 << 20
	}
	if limits.MaxOutputBytes <= 0 {
		limits.MaxOutputBytes = 1 << 20
	}
	if limits.MaxStderrBytes <= 0 {
		limits.MaxStderrBytes = 64 << 10
	}
	if limits.MaxTools <= 0 {
		limits.MaxTools = 256
	}
	if limits.MaxCommands <= 0 {
		limits.MaxCommands = 128
	}
	if limits.MaxHooks <= 0 {
		limits.MaxHooks = 128
	}
	if limits.MaxPromptBytes <= 0 {
		limits.MaxPromptBytes = 64 << 10
	}
	if limits.MaxUIContributions <= 0 {
		limits.MaxUIContributions = 128
	}
	return limits
}
