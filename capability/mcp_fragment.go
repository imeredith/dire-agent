package capability

import (
	"fmt"
	"strings"

	"github.com/dire-kiwi/dire-agent/agentloop"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/mcpclient"
)

func mcpFragment(entry *mcpEntry) Fragment {
	fragment := Fragment{Tools: make(map[string]agentloop.Tool)}
	fragment.Descriptors = append(fragment.Descriptors, entry.denied...)
	for _, status := range entry.pool.ServerStatuses() {
		description := fmt.Sprintf("%s MCP server", status.Transport)
		if status.SupportsResources || status.SupportsPrompts {
			var features []string
			if status.SupportsResources {
				features = append(features, "resources")
			}
			if status.SupportsPrompts {
				features = append(features, "prompts")
			}
			description += " (" + strings.Join(features, ", ") + ")"
		}
		if status.LastError != "" {
			description += ": " + status.LastError
		}
		fragment.Descriptors = append(fragment.Descriptors, Descriptor{
			Name: "mcp:" + status.Name, Source: "mcp", Description: description,
			Enabled: status.Enabled && status.Trusted, Status: string(status.State),
		})
	}
	available := entry.pool.AgentTools()
	for _, status := range entry.pool.ToolStatuses() {
		server := entry.servers[status.Server]
		enabled := mcpToolEnabled(server, status.Name, status.ModelName)
		state := "ready"
		if !enabled {
			state = "disabled"
		} else if mcpApproval(server, status.Name) != configuration.ApprovalNever {
			enabled, state = false, "approval_required"
		} else if !status.Available {
			enabled, state = false, "unavailable"
		}
		if status.LastError != "" {
			state = "degraded"
		}
		fragment.Descriptors = append(fragment.Descriptors, Descriptor{
			Name: status.ModelName, Source: "mcp", Description: status.Description,
			Enabled: enabled, Status: state,
		})
		if enabled && available[status.ModelName] != nil {
			fragment.Tools[status.ModelName] = available[status.ModelName]
		}
	}
	contextTools := entry.pool.ContextTools()
	for serverName, server := range entry.servers {
		for _, operation := range []string{"list_resources", "read_resource", "list_prompts", "get_prompt"} {
			name := mcpclient.ContextToolName(serverName, operation)
			tool := contextTools[name]
			if tool == nil {
				continue
			}
			enabled, state := server.Enabled, "ready"
			if !enabled {
				state = "disabled"
			} else if server.Approval != configuration.ApprovalNever {
				enabled, state = false, "approval_required"
			}
			definition := tool.Definition()
			fragment.Descriptors = append(fragment.Descriptors, Descriptor{
				Name: name, Source: "mcp", Description: definition.Description,
				Enabled: enabled, Status: state,
			})
			if enabled {
				fragment.Tools[name] = tool
			}
		}
	}
	return fragment
}

func mcpToolEnabled(server configuration.MCPServer, remoteName, modelName string) bool {
	if !server.Enabled {
		return false
	}
	if len(server.EnabledTools) == 0 {
		return true
	}
	for _, name := range server.EnabledTools {
		if name == remoteName || name == modelName {
			return true
		}
	}
	return false
}

func mcpApproval(server configuration.MCPServer, tool string) configuration.ApprovalMode {
	if mode, ok := server.ToolApprovals[tool]; ok {
		return mode
	}
	return server.Approval
}
