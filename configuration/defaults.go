package configuration

import (
	"os"
	"path/filepath"

	"github.com/dire-kiwi/dire-agent/modelcatalog"
)

// DefaultPath returns the conventional daemon configuration path.
func DefaultPath(home string) string {
	path := filepath.Join(home, ".dire-agent", "config.json")
	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		return path
	}
	legacy := filepath.Join(home, ".goagent", "config.json")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return path
}

// DefaultConfig returns safe global defaults. Standalone chat intentionally has
// no folder field and therefore does not create or require a project.
func DefaultConfig(home string) Config {
	codexHome := filepath.Join(home, ".codex")
	return Config{
		Version:  CurrentVersion,
		Revision: 1,
		Global: Settings{
			Model: ModelSettings{
				Provider:      "codex",
				ID:            "gpt-5.6",
				ContextWindow: modelcatalog.GPT56ContextWindow,
			},
			Thinking: ThinkingSettings{Level: ThinkingMedium},
			Tools: ToolSettings{
				Enabled:  []string{"read", "grep", "find", "ls"},
				Sandbox:  SandboxStrict,
				Approval: ApprovalOnRequest,
			},
			Queues: QueueSettings{
				SteeringMode: QueueOneAtATime,
				FollowUpMode: QueueOneAtATime,
				MaxPending:   64,
			},
			Skills: SkillSettings{
				Roots: []string{
					filepath.Join(home, ".agents", "skills"),
					filepath.Join(codexHome, "skills"),
					filepath.Join(home, ".pi", "agent", "skills"),
				},
				Trust: TrustPrompt,
			},
			MCP:        MCPSettings{Servers: map[string]MCPServer{}},
			Extensions: ExtensionSettings{Sources: map[string]ExtensionSource{}},
			Subagents: SubagentSettings{
				Enabled: true, MaxDepth: 2, MaxChildren: 8, MaxConcurrent: 4,
				AllowSiblingMessages: true, AutoReport: true,
				Profiles: map[string]AgentProfile{
					"general": {
						Description: "General-purpose agent for independent implementation or research.",
						Thinking:    ThinkingMedium, Tools: []string{"read", "grep", "find", "ls"}, CanSpawn: false,
					},
					"explore": {
						Description: "Fast read-only agent for codebase exploration and focused questions.",
						Thinking:    ThinkingLow, Tools: []string{"read", "grep", "find", "ls"}, CanSpawn: false,
					},
					"review": {
						Description: "Read-only reviewer for correctness, security, and maintainability.",
						Thinking:    ThinkingHigh, Tools: []string{"read", "grep", "find", "ls"}, CanSpawn: false,
					},
				},
			},
			Launchers: DefaultProjectLaunchers(),
			Desktop: DesktopSettings{
				CodexHome:      codexHome,
				DesktopConfig:  filepath.Join(codexHome, "config.toml"),
				SyncMode:       SyncImport,
				SyncSkills:     true,
				SyncMCP:        true,
				SyncExtensions: true,
			},
			StandaloneChat: StandaloneChatSettings{
				Model:          "gpt-5.6",
				Thinking:       ThinkingMedium,
				Tools:          []string{},
				PersistHistory: true,
			},
		},
		Projects: map[string]ProjectOverride{},
	}
}

// DefaultProjectLaunchers returns the built-in interactive applications. A
// fresh slice is returned on every call so callers can safely customize it.
func DefaultProjectLaunchers() []ProjectLauncher {
	return []ProjectLauncher{
		{ID: "shell", Label: "Terminal", Kind: LauncherTerminal, Shortcut: "mod+backquote"},
		{ID: "lazygit", Label: "lazygit", Kind: LauncherTerminal, Command: "lazygit", Shortcut: "mod+shift+g"},
		{ID: "nvim", Label: "nvim", Kind: LauncherTerminal, Command: "nvim", Args: []string{"."}, Shortcut: "mod+shift+e"},
	}
}

// ResolveProjectLaunchers supplies the built-ins for legacy configuration
// documents that predate the launchers field. An explicit empty list remains
// empty, allowing every launcher to be disabled.
func ResolveProjectLaunchers(configured []ProjectLauncher) []ProjectLauncher {
	if configured == nil {
		return DefaultProjectLaunchers()
	}
	return cloneProjectLaunchers(configured)
}
