// Package configuration owns the daemon's versioned, user-editable settings.
package configuration

const (
	CurrentVersion = 1
	RedactedValue  = "[redacted]"
)

type ThinkingLevel string
type ApprovalMode string
type QueueMode string
type SandboxMode string
type TrustMode string
type MCPTransport string
type ExtensionKind string
type SyncMode string
type LauncherKind string

const (
	ThinkingNone    ThinkingLevel = "none"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingMax     ThinkingLevel = "max"

	ApprovalNever     ApprovalMode = "never"
	ApprovalOnRequest ApprovalMode = "on-request"
	ApprovalAlways    ApprovalMode = "always"

	QueueOneAtATime QueueMode = "one-at-a-time"
	QueueAll        QueueMode = "all"

	SandboxStrict    SandboxMode = "strict"
	SandboxWorkspace SandboxMode = "workspace"
	SandboxOff       SandboxMode = "off"

	TrustDenied  TrustMode = "denied"
	TrustPrompt  TrustMode = "prompt"
	TrustTrusted TrustMode = "trusted"

	MCPStdio          MCPTransport = "stdio"
	MCPStreamableHTTP MCPTransport = "streamable-http"

	ExtensionLocal    ExtensionKind = "local"
	ExtensionGit      ExtensionKind = "git"
	ExtensionRegistry ExtensionKind = "registry"

	SyncOff           SyncMode = "off"
	SyncImport        SyncMode = "import"
	SyncExport        SyncMode = "export"
	SyncBidirectional SyncMode = "bidirectional"

	LauncherTerminal LauncherKind = "terminal"
	LauncherDesktop  LauncherKind = "desktop"
)

type Config struct {
	Version  int                        `json:"version"`
	Revision uint64                     `json:"revision"`
	Global   Settings                   `json:"global"`
	Projects map[string]ProjectOverride `json:"projects,omitempty"`
}

type Settings struct {
	Model          ModelSettings          `json:"model"`
	Thinking       ThinkingSettings       `json:"thinking"`
	Tools          ToolSettings           `json:"tools"`
	Queues         QueueSettings          `json:"queues"`
	Skills         SkillSettings          `json:"skills"`
	MCP            MCPSettings            `json:"mcp"`
	Extensions     ExtensionSettings      `json:"extensions"`
	Subagents      SubagentSettings       `json:"subagents"`
	Launchers      []ProjectLauncher      `json:"launchers"`
	Desktop        DesktopSettings        `json:"desktop"`
	StandaloneChat StandaloneChatSettings `json:"standalone_chat"`
}

// ProjectLauncher describes a user-operated application that starts in a
// project's canonical working directory. Commands and arguments are stored
// separately and are never interpreted by a shell.
type ProjectLauncher struct {
	ID       string       `json:"id"`
	Label    string       `json:"label"`
	Kind     LauncherKind `json:"kind"`
	Icon     string       `json:"icon,omitempty"`
	Command  string       `json:"command,omitempty"`
	Args     []string     `json:"args,omitempty"`
	Shortcut string       `json:"shortcut,omitempty"`
}

type ModelSettings struct {
	Provider      string `json:"provider"`
	ID            string `json:"id"`
	ContextWindow int64  `json:"context_window,omitempty"`
}

type ThinkingSettings struct {
	Level ThinkingLevel `json:"level"`
}

type ToolSettings struct {
	Enabled  []string     `json:"enabled"`
	Sandbox  SandboxMode  `json:"sandbox"`
	Approval ApprovalMode `json:"approval"`
}

type QueueSettings struct {
	SteeringMode QueueMode `json:"steering_mode"`
	FollowUpMode QueueMode `json:"follow_up_mode"`
	MaxPending   int       `json:"max_pending"`
}

type SkillSettings struct {
	Roots    []string  `json:"roots"`
	Disabled []string  `json:"disabled,omitempty"`
	Trust    TrustMode `json:"trust"`
}

type MCPSettings struct {
	Servers map[string]MCPServer `json:"servers,omitempty"`
}

type MCPServer struct {
	Transport     MCPTransport            `json:"transport"`
	Command       string                  `json:"command,omitempty"`
	Args          []string                `json:"args,omitempty"`
	InheritEnv    bool                    `json:"inherit_env,omitempty"`
	URL           string                  `json:"url,omitempty"`
	Env           map[string]string       `json:"env,omitempty"`
	SecretEnv     []string                `json:"secret_env,omitempty"`
	Headers       map[string]string       `json:"headers,omitempty"`
	SecretHeaders []string                `json:"secret_headers,omitempty"`
	EnabledTools  []string                `json:"enabled_tools,omitempty"`
	Approval      ApprovalMode            `json:"approval"`
	ToolApprovals map[string]ApprovalMode `json:"tool_approvals,omitempty"`
	Enabled       bool                    `json:"enabled"`
}

type ExtensionSettings struct {
	Sources       map[string]ExtensionSource `json:"sources,omitempty"`
	AllowUnsigned bool                       `json:"allow_unsigned"`
}

type ExtensionSource struct {
	Kind       ExtensionKind     `json:"kind"`
	Location   string            `json:"location"`
	Ref        string            `json:"ref,omitempty"`
	Trust      TrustMode         `json:"trust"`
	Enabled    bool              `json:"enabled"`
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	SecretEnv  []string          `json:"secret_env,omitempty"`
	InheritEnv bool              `json:"inherit_env,omitempty"`
}

type SubagentSettings struct {
	Enabled              bool                    `json:"enabled"`
	MaxDepth             int                     `json:"max_depth"`
	MaxChildren          int                     `json:"max_children"`
	MaxConcurrent        int                     `json:"max_concurrent"`
	AllowSiblingMessages bool                    `json:"allow_sibling_messages"`
	AutoReport           bool                    `json:"auto_report"`
	Profiles             map[string]AgentProfile `json:"profiles"`
}

type AgentProfile struct {
	Description  string        `json:"description"`
	Instructions string        `json:"instructions,omitempty"`
	Model        string        `json:"model,omitempty"`
	Thinking     ThinkingLevel `json:"thinking,omitempty"`
	// A nil list inherits the parent's tools; an empty list grants no local tools.
	Tools    []string `json:"tools"`
	CanSpawn bool     `json:"can_spawn"`
}

// PluginSettings and PluginSource are compatibility names for clients that use
// Pi's "plugin" terminology. Plugins and extensions share one source registry.
type PluginSettings = ExtensionSettings
type PluginSource = ExtensionSource

type DesktopSettings struct {
	CodexHome       string   `json:"codex_home"`
	DesktopConfig   string   `json:"desktop_config,omitempty"`
	SyncMode        SyncMode `json:"sync_mode"`
	SyncSkills      bool     `json:"sync_skills"`
	SyncMCP         bool     `json:"sync_mcp"`
	SyncExtensions  bool     `json:"sync_extensions"`
	WatchForChanges bool     `json:"watch_for_changes"`
}

type StandaloneChatSettings struct {
	Model          string        `json:"model"`
	Thinking       ThinkingLevel `json:"thinking"`
	Tools          []string      `json:"tools"`
	Instructions   string        `json:"instructions,omitempty"`
	PersistHistory bool          `json:"persist_history"`
}
