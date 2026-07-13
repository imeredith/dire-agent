package capability

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dire-kiwi/dire-agent/agentloop"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/mcpclient"
	localtools "github.com/dire-kiwi/dire-agent/tools"
)

type mcpPool interface {
	Connect(context.Context) error
	AgentTools() map[string]agentloop.Tool
	ContextTools() map[string]agentloop.Tool
	ServerStatuses() []mcpclient.ServerStatus
	ToolStatuses() []mcpclient.ToolStatus
	Close() error
}

type MCPSourceConfig struct {
	Options   mcpclient.Options
	NewClient func([]mcpclient.ServerConfig, mcpclient.Options) (mcpPool, error)
}

type MCPSource struct {
	mu        sync.Mutex
	options   mcpclient.Options
	newClient func([]mcpclient.ServerConfig, mcpclient.Options) (mcpPool, error)
	entries   map[string]*mcpEntry
	closed    bool
}

type mcpEntry struct {
	fingerprint [32]byte
	pool        mcpPool
	servers     map[string]configuration.MCPServer
	denied      []Descriptor
}

func NewMCPSource(config MCPSourceConfig) *MCPSource {
	newClient := config.NewClient
	if newClient == nil {
		newClient = func(configs []mcpclient.ServerConfig, options mcpclient.Options) (mcpPool, error) {
			return mcpclient.New(configs, options)
		}
	}
	return &MCPSource{options: config.Options, newClient: newClient, entries: make(map[string]*mcpEntry)}
}

func (*MCPSource) Name() string { return "mcp" }

func (s *MCPSource) Resolve(ctx context.Context, scope Scope, settings configuration.Settings) (Fragment, error) {
	configs, servers, denied := mcpConfigs(scope, settings)
	fingerprint := mcpFingerprint(configs, servers)
	key := scope.ConversationID
	if key == "" {
		key = scope.Kind + "|" + scope.CWD
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Fragment{}, fmt.Errorf("capability: MCP source is closed")
	}
	if entry := s.entries[key]; entry != nil && entry.fingerprint == fingerprint {
		s.mu.Unlock()
		return mcpFragment(entry), nil
	}
	s.mu.Unlock()

	pool, err := s.newClient(configs, s.options)
	if err != nil {
		return Fragment{}, fmt.Errorf("create MCP client: %w", err)
	}
	// Connect is intentionally best-effort: one unavailable optional MCP server
	// must not prevent a conversation from using its other capabilities.
	_ = pool.Connect(ctx)
	candidate := &mcpEntry{fingerprint: fingerprint, pool: pool, servers: servers, denied: denied}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = pool.Close()
		return Fragment{}, fmt.Errorf("capability: MCP source is closed")
	}
	if existing := s.entries[key]; existing != nil && existing.fingerprint == fingerprint {
		s.mu.Unlock()
		_ = pool.Close()
		return mcpFragment(existing), nil
	}
	old := s.entries[key]
	s.entries[key] = candidate
	s.mu.Unlock()
	if old != nil {
		_ = old.pool.Close()
	}
	return mcpFragment(candidate), nil
}

func (s *MCPSource) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	entries := s.entries
	s.entries = make(map[string]*mcpEntry)
	s.mu.Unlock()
	var result error
	for _, entry := range entries {
		result = errors.Join(result, entry.pool.Close())
	}
	return result
}

func mcpFingerprint(configs []mcpclient.ServerConfig, servers map[string]configuration.MCPServer) [32]byte {
	data, _ := json.Marshal(struct {
		Configs []mcpclient.ServerConfig
		Policy  map[string]configuration.MCPServer
	}{configs, servers})
	return sha256.Sum256(data)
}

func mcpConfigs(scope Scope, settings configuration.Settings) ([]mcpclient.ServerConfig, map[string]configuration.MCPServer, []Descriptor) {
	input := settings.MCP.Servers
	names := make([]string, 0, len(input))
	for name := range input {
		names = append(names, name)
	}
	sort.Strings(names)
	configs := make([]mcpclient.ServerConfig, 0, len(names))
	servers := make(map[string]configuration.MCPServer, len(names))
	var denied []Descriptor
	for _, name := range names {
		server := input[name]
		if enabled, overridden := scope.MCPServerOverrides[name]; overridden {
			server.Enabled = enabled
		}
		servers[name] = server
		if server.Enabled && recursiveMCP(server) {
			denied = append(denied, Descriptor{Name: "mcp:" + name, Source: "mcp", Status: "recursive_denied", Description: "The outward Dire Agent desktop bridge cannot be registered as an inward MCP server."})
			continue
		}
		config := mcpclient.ServerConfig{
			Name: name, Enabled: server.Enabled, Trusted: server.Enabled,
			Command: server.Command, Arguments: append([]string(nil), server.Args...),
			Environment: cloneStringsMap(server.Env), InheritEnvironment: server.InheritEnv,
			WorkingDirectory: scope.CWD, Endpoint: server.URL, Headers: cloneStringsMap(server.Headers),
		}
		if server.Transport == configuration.MCPStdio {
			config.Transport = mcpclient.TransportStdio
			if server.Enabled && settings.Tools.Sandbox != configuration.SandboxOff {
				workspace := scope.CWD
				privateWorkspace := false
				if workspace == "" {
					workspace = os.TempDir()
					config.WorkingDirectory = workspace
					privateWorkspace = true
				}
				command, args, err := localtools.WrapSandboxedProcess(localtools.ProcessSandbox{
					Workspace: workspace, WorkingDirectory: config.WorkingDirectory,
					Command: config.Command, Args: config.Arguments,
					AdditionalWritePaths: scope.AdditionalFolders,
					AllowNetwork:         settings.Tools.Sandbox == configuration.SandboxWorkspace,
					PrivateWorkspace:     privateWorkspace,
				})
				if err != nil {
					denied = append(denied, Descriptor{
						Name: "mcp:" + name, Source: "mcp", Enabled: false,
						Status: "sandbox_unavailable", Description: err.Error(),
					})
					continue
				}
				config.Command, config.Arguments = command, args
				config.Sandboxed = true
			}
		} else {
			config.Transport = mcpclient.TransportStreamableHTTP
		}
		configs = append(configs, config)
	}
	return configs, servers, denied
}

func recursiveMCP(server configuration.MCPServer) bool {
	if server.Transport != configuration.MCPStdio {
		return false
	}
	name := strings.ToLower(filepath.Base(strings.TrimSpace(server.Command)))
	if (name == "dire-agent" || name == "dire-agent.exe") && len(server.Args) > 0 && strings.EqualFold(strings.TrimSpace(server.Args[0]), "mcp") {
		return true
	}
	return name == "dire-agent-mcp" || name == "dire-agent-mcp.exe" ||
		name == "goagent-mcp" || name == "goagent-mcp.exe"
}

func cloneStringsMap(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
