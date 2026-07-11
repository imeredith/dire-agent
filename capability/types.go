// Package capability composes model-callable tools and prompt metadata without
// coupling the daemon to discovery or transport implementations.
package capability

import (
	"context"

	"github.com/imeredith/dire-agent/agentloop"
	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/skills"
)

// Scope is the minimum conversation context a capability may inspect.
type Scope struct {
	ConversationID string
	// SettingsID allows child agents to inherit their root conversation's
	// configuration without sharing the child's transport/session identity.
	SettingsID        string
	Kind              string
	CWD               string
	AdditionalFolders []string
	Builtins          []string
}

type Descriptor struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	Status      string `json:"status,omitempty"`
}

// Snapshot is immutable for one provider session/run.
type Snapshot struct {
	Tools         map[string]agentloop.Tool                     `json:"-"`
	Instructions  string                                        `json:"instructions,omitempty"`
	Descriptors   []Descriptor                                  `json:"capabilities,omitempty"`
	Skills        []skills.Skill                                `json:"skills,omitempty"`
	Diagnostics   []skills.Diagnostic                           `json:"skill_diagnostics,omitempty"`
	PreparePrompt func(context.Context, string) (string, error) `json:"-"`
	Hooks         agentloop.Hooks                               `json:"-"`
	Commands      map[string]Command                            `json:"-"`
}

// Fragment lets independent transports contribute tools and plugin skill roots.
type Fragment struct {
	Tools            map[string]agentloop.Tool
	Descriptors      []Descriptor
	PluginSkillRoots []skills.PluginRoot
	Instructions     string
	Hooks            agentloop.Hooks
	Commands         map[string]Command
}

type CommandResult struct {
	Output  string `json:"output,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

type Command struct {
	Name        string
	Description string
	Source      string
	Execute     func(context.Context, string) (CommandResult, error)
}

type SettingsStore interface {
	RuntimeSettings(context.Context, string) (configuration.Settings, bool, error)
}

type Source interface {
	Name() string
	Resolve(context.Context, Scope, configuration.Settings) (Fragment, error)
	Close() error
}

type Resolver interface {
	Resolve(context.Context, Scope) (Snapshot, error)
	Close() error
}
