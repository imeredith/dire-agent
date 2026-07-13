// Package extensions discovers Pi and Codex plugin metadata and runs explicitly
// trusted extension adapters out of process.
package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const ProtocolVersion = "1.0"

type Trust string

const (
	TrustDenied  Trust = "denied"
	TrustPrompt  Trust = "prompt"
	TrustTrusted Trust = "trusted"
)

type State string

const (
	StateDisabled   State = "disabled"
	StateDenied     State = "denied"
	StateNeedsTrust State = "needs_trust"
	StateCatalogued State = "catalogued"
	StateRunnable   State = "runnable"
	StateInvalid    State = "invalid"
)

type Format string

const (
	FormatCodex Format = "codex-plugin"
	FormatPi    Format = "pi-package"
	FormatLocal Format = "local"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Source is a user-configured local extension or plugin. Command and Args are
// an explicit adapter process; manifest entrypoints are never executed directly.
type Source struct {
	ID         string            `json:"id,omitempty"`
	Location   string            `json:"location"`
	Enabled    bool              `json:"enabled"`
	Trust      Trust             `json:"trust"`
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	InheritEnv bool              `json:"inherit_env,omitempty"`
	Sandboxed  bool              `json:"-"`
}

type DiscoverOptions struct {
	Sources     []Source
	PluginRoots []string
	MaxDepth    int
	MaxEntries  int
}

type Diagnostic struct {
	Severity    Severity `json:"severity"`
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Path        string   `json:"path,omitempty"`
	ExtensionID string   `json:"extension_id,omitempty"`
}

type Extension struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Version     string       `json:"version,omitempty"`
	Description string       `json:"description,omitempty"`
	Format      Format       `json:"format"`
	Root        string       `json:"root"`
	Manifest    string       `json:"manifest,omitempty"`
	Entrypoints []string     `json:"entrypoints,omitempty"`
	SkillRoots  []string     `json:"skill_roots,omitempty"`
	PromptRoots []string     `json:"prompt_roots,omitempty"`
	ThemeRoots  []string     `json:"theme_roots,omitempty"`
	HasMCP      bool         `json:"has_mcp,omitempty"`
	HasApp      bool         `json:"has_app,omitempty"`
	Enabled     bool         `json:"enabled"`
	Trust       Trust        `json:"trust"`
	State       State        `json:"state"`
	Process     ProcessSpec  `json:"process,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type Catalog struct {
	Extensions  []Extension  `json:"extensions"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type ProcessSpec struct {
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Dir        string            `json:"dir,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	InheritEnv bool              `json:"inherit_env,omitempty"`
	Sandboxed  bool              `json:"-"`
}

type LaunchConfig struct {
	ID      string
	Enabled bool
	Trust   Trust
	Process ProcessSpec
}

func (e Extension) LaunchConfig() LaunchConfig {
	return LaunchConfig{ID: e.ID, Enabled: e.Enabled, Trust: e.Trust, Process: e.Process}
}

type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ToolResult struct {
	Output  string `json:"output"`
	IsError bool   `json:"is_error,omitempty"`
}

type Limits struct {
	InitializeTimeout  time.Duration
	CallTimeout        time.Duration
	CloseTimeout       time.Duration
	MaxMessageBytes    int
	MaxOutputBytes     int
	MaxStderrBytes     int
	MaxTools           int
	MaxCommands        int
	MaxHooks           int
	MaxPromptBytes     int
	MaxUIContributions int
}

type Connection interface {
	Call(context.Context, string, any, any) error
	Stderr() string
	Close(context.Context) error
}

type Connector interface {
	Connect(context.Context, ProcessSpec, Limits) (Connection, error)
}

type ConnectorFunc func(context.Context, ProcessSpec, Limits) (Connection, error)

func (f ConnectorFunc) Connect(ctx context.Context, spec ProcessSpec, limits Limits) (Connection, error) {
	return f(ctx, spec, limits)
}

type OpenOptions struct {
	Connector Connector
	Limits    Limits
	Client    PeerInfo
}

type PeerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

var (
	ErrDisabled     = errors.New("extensions: extension is disabled")
	ErrUntrusted    = errors.New("extensions: extension is not trusted")
	ErrClosed       = errors.New("extensions: connection is closed")
	ErrToolReported = errors.New("extensions: tool reported an error")
)
