package main

import (
	"context"
	"fmt"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoArgs struct {
	Value string `json:"value" jsonschema:"value to echo"`
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "dire-agent-browser-fixture", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "Echo a deterministic browser fixture value."},
		func(_ context.Context, _ *mcp.CallToolRequest, args echoArgs) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "MCP_ECHO: " + args.Value}}}, nil, nil
		})
	server.AddResource(&mcp.Resource{Name: "fixture-status", MIMEType: "text/plain", URI: "fixture:status"},
		func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			parsed, err := url.Parse(request.Params.URI)
			if err != nil || parsed.Scheme != "fixture" {
				return nil, fmt.Errorf("invalid fixture URI")
			}
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: request.Params.URI, MIMEType: "text/plain", Text: "MCP_RESOURCE_OK"}}}, nil
		})
	server.AddPrompt(&mcp.Prompt{Name: "fixture-prompt", Description: "Returns the deterministic fixture prompt."},
		func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{Description: "Browser fixture", Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "Reply with exactly MCP_PROMPT_OK"}}}}, nil
		})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		panic(err)
	}
}
