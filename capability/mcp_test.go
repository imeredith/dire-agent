package capability

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/mcpclient"
)

func TestMCPSourceConnectsCachesAndFiltersTools(t *testing.T) {
	var connects atomic.Int32
	session := &fakeMCPSession{}
	source := NewMCPSource(MCPSourceConfig{Options: mcpclient.Options{
		TransportFactory: mcpclient.TransportFactoryFunc(func(context.Context, mcpclient.ServerConfig) (mcp.Transport, error) {
			return nil, nil
		}),
		Connector: mcpclient.ConnectorFunc(func(context.Context, mcp.Transport, mcpclient.ConnectOptions) (mcpclient.Session, error) {
			connects.Add(1)
			return session, nil
		}),
	}})
	defer source.Close()
	settings := configuration.DefaultConfig(t.TempDir()).Global
	// This test supplies a fake transport and does not exercise process sandboxing.
	// Disable it so the test is portable to Linux runners without sandbox-exec.
	settings.Tools.Sandbox = configuration.SandboxOff
	settings.MCP.Servers["docs"] = configuration.MCPServer{
		Transport: configuration.MCPStdio, Command: "/usr/bin/docs-mcp", Enabled: true,
		Approval: configuration.ApprovalNever,
	}
	first, err := source.Resolve(context.Background(), Scope{ConversationID: "chat_1", Kind: "chat"}, settings)
	if err != nil {
		t.Fatal(err)
	}
	if first.Tools["mcp__docs__lookup"] == nil {
		t.Fatalf("tools = %v descriptors=%+v", first.Tools, first.Descriptors)
	}
	if first.Tools["mcpctx__docs__read_resource"] == nil || first.Tools["mcpctx__docs__get_prompt"] == nil {
		t.Fatalf("advertised MCP resources/prompts were not exposed: %v", first.Tools)
	}
	second, err := source.Resolve(context.Background(), Scope{ConversationID: "chat_1", Kind: "chat"}, settings)
	if err != nil || second.Tools["mcp__docs__lookup"] == nil {
		t.Fatalf("second resolve: tools=%v err=%v", second.Tools, err)
	}
	if connects.Load() != 1 {
		t.Fatalf("connections = %d", connects.Load())
	}
	output, err := first.Tools["mcp__docs__lookup"].Execute(context.Background(), json.RawMessage(`{"query":"go"}`))
	if err != nil || output != "found" {
		t.Fatalf("Execute = %q, %v", output, err)
	}
}

func TestMCPSourceRequiresApprovalAndRejectsRecursiveBridge(t *testing.T) {
	source := NewMCPSource(MCPSourceConfig{})
	defer source.Close()
	settings := configuration.DefaultConfig(t.TempDir()).Global
	settings.MCP.Servers["self"] = configuration.MCPServer{
		Transport: configuration.MCPStdio, Command: "dire-agent-mcp", Enabled: true,
		Approval: configuration.ApprovalNever,
	}
	fragment, err := source.Resolve(context.Background(), Scope{ConversationID: "chat_1", Kind: "chat"}, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(fragment.Tools) != 0 || !hasDescriptorStatus(fragment.Descriptors, "recursive_denied") {
		t.Fatalf("fragment = %+v", fragment)
	}
}

func TestRecursiveMCPRejectsCurrentAndLegacyBridgeNames(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		"dire-agent-mcp",
		"/usr/local/bin/dire-agent-mcp",
		"dire-agent-mcp.exe",
		"goagent-mcp",
		"/usr/local/bin/goagent-mcp",
		"goagent-mcp.exe",
	} {
		if !recursiveMCP(configuration.MCPServer{Transport: configuration.MCPStdio, Command: command}) {
			t.Errorf("recursiveMCP(%q) = false", command)
		}
	}
	for _, command := range []string{"dire-agent", "/usr/local/bin/dire-agent", "dire-agent.exe"} {
		if !recursiveMCP(configuration.MCPServer{Transport: configuration.MCPStdio, Command: command, Args: []string{"mcp"}}) {
			t.Errorf("recursiveMCP(%q mcp) = false", command)
		}
	}
	if recursiveMCP(configuration.MCPServer{Transport: configuration.MCPStdio, Command: "dire-agent", Args: []string{"ask"}}) {
		t.Fatal("recursiveMCP rejected a non-MCP dire-agent subcommand")
	}
	if recursiveMCP(configuration.MCPServer{Transport: configuration.MCPStdio, Command: "other-mcp"}) {
		t.Fatal("recursiveMCP accepted an unrelated stdio server")
	}
	if recursiveMCP(configuration.MCPServer{Transport: configuration.MCPStreamableHTTP, Command: "dire-agent-mcp"}) {
		t.Fatal("recursiveMCP rejected a non-stdio server")
	}
}

func hasDescriptorStatus(values []Descriptor, status string) bool {
	for _, value := range values {
		if value.Status == status {
			return true
		}
	}
	return false
}

type fakeMCPSession struct{ closed atomic.Bool }

func (*fakeMCPSession) ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	return &mcp.ListToolsResult{Tools: []*mcp.Tool{{
		Name: "lookup", Description: "Look up documentation.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
	}}}, nil
}

func (*fakeMCPSession) CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "found"}}}, nil
}

func (*fakeMCPSession) InitializeResult() *mcp.InitializeResult {
	return &mcp.InitializeResult{
		ProtocolVersion: "2025-11-25", ServerInfo: &mcp.Implementation{Name: "fake", Version: "1"},
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{}, Resources: &mcp.ResourceCapabilities{}, Prompts: &mcp.PromptCapabilities{},
		},
	}
}

func (*fakeMCPSession) ListResources(context.Context, *mcp.ListResourcesParams) (*mcp.ListResourcesResult, error) {
	return &mcp.ListResourcesResult{Resources: []*mcp.Resource{{URI: "docs://readme", Name: "readme"}}}, nil
}

func (*fakeMCPSession) ReadResource(context.Context, *mcp.ReadResourceParams) (*mcp.ReadResourceResult, error) {
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "docs://readme", Text: "documentation"}}}, nil
}

func (*fakeMCPSession) ListPrompts(context.Context, *mcp.ListPromptsParams) (*mcp.ListPromptsResult, error) {
	return &mcp.ListPromptsResult{Prompts: []*mcp.Prompt{{Name: "review"}}}, nil
}

func (*fakeMCPSession) GetPrompt(context.Context, *mcp.GetPromptParams) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "review"}}}}, nil
}

func (s *fakeMCPSession) Close() error { s.closed.Store(true); return nil }
