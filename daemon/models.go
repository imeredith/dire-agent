package daemon

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/capability"
	"github.com/dire-kiwi/dire-agent/modelcatalog"
	"github.com/dire-kiwi/dire-agent/skills"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

// ModelInfo describes a model offered to daemon clients. ContextWindow may be
// zero when the provider has not published a reliable limit for that model.
type ModelInfo struct {
	Provider      string `json:"provider"`
	ID            string `json:"id"`
	ContextWindow int64  `json:"context_window,omitempty"`
}

type CreateThreadOptions struct {
	Name              string                 `json:"name,omitempty"`
	Category          string                 `json:"category,omitempty"`
	Model             string                 `json:"model,omitempty"`
	CWD               string                 `json:"cwd,omitempty"`
	AdditionalFolders []string               `json:"additional_folders,omitempty"`
	Instructions      string                 `json:"instructions,omitempty"`
	ThinkingLevel     string                 `json:"thinking_level,omitempty"`
	Tools             []string               `json:"tools,omitempty"`
	Worktree          *CreateWorktreeOptions `json:"worktree,omitempty"`
}

// CreateWorktreeOptions requests an isolated detached Git checkout. CWD is
// the source project folder unless SourceProjectID identifies it explicitly.
type CreateWorktreeOptions struct {
	BaseRef         string `json:"base_ref,omitempty"`
	EnvironmentID   string `json:"environment_id,omitempty"`
	SourceProjectID string `json:"source_project_id,omitempty"`
}

type ProjectWorkspaceInspection struct {
	Folder              string               `json:"folder"`
	GitRepository       bool                 `json:"git_repository"`
	RepositoryRoot      string               `json:"repository_root,omitempty"`
	ProjectRelativePath string               `json:"project_relative_path,omitempty"`
	Head                string               `json:"head,omitempty"`
	CurrentBranch       string               `json:"current_branch,omitempty"`
	Branches            []string             `json:"branches"`
	Environments        []ProjectEnvironment `json:"environments"`
}

// CreateProjectOptions is the project-oriented public name. The alias keeps
// existing clients source compatible while projects replace threads in the UI.
type CreateProjectOptions = CreateThreadOptions

type CreateChatOptions struct {
	Name          string `json:"name,omitempty"`
	Model         string `json:"model,omitempty"`
	Instructions  string `json:"instructions,omitempty"`
	ThinkingLevel string `json:"thinking_level,omitempty"`
}

// ImageAttachment is the WebSocket and persistence representation of a pasted
// image. Data is accepted only on input; File and Size identify the copy owned
// by the project's sandbox.
type ImageAttachment struct {
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data,omitempty"`
	File     string `json:"file,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

type RuntimeState struct {
	Kind             string                  `json:"kind"`
	Conversation     threadstore.Thread      `json:"conversation"`
	Project          threadstore.Project     `json:"project"`
	Chat             threadstore.Chat        `json:"chat"`
	Thread           threadstore.Thread      `json:"thread"`
	Usage            agent.Usage             `json:"usage"`
	Capabilities     []capability.Descriptor `json:"capabilities,omitempty"`
	Skills           []skills.Skill          `json:"skills,omitempty"`
	SkillDiagnostics []skills.Diagnostic     `json:"skill_diagnostics,omitempty"`
	Running          bool                    `json:"running"`
	SteeringQueued   int                     `json:"steering_queued"`
	FollowUpsQueued  int                     `json:"follow_ups_queued"`
}

type Event struct {
	Type           string            `json:"type"`
	Scope          ConversationScope `json:"scope"`
	ConversationID string            `json:"conversation_id"`
	ProjectID      string            `json:"project_id,omitempty"`
	ChatID         string            `json:"chat_id,omitempty"`
	ThreadID       string            `json:"thread_id"`
	Sequence       int64             `json:"sequence,omitempty"`
	Timestamp      time.Time         `json:"timestamp"`
	Data           json.RawMessage   `json:"data,omitempty"`
}

type SettingsUpdate struct {
	Name              *string
	Category          *string
	AdditionalFolders *[]string
	Model             *string
	ThinkingLevel     *string
	SteeringMode      *string
	FollowUpMode      *string
	Tools             *[]string
}

type CapabilityState struct {
	Capabilities     []capability.Descriptor `json:"capabilities"`
	Skills           []skills.Skill          `json:"skills"`
	SkillDiagnostics []skills.Diagnostic     `json:"skill_diagnostics,omitempty"`
}

func (m *Manager) AvailableModels() []ModelInfo {
	return append([]ModelInfo(nil), m.config.AvailableModels...)
}

func (m *Manager) modelInfo(id string) (ModelInfo, bool) {
	for _, model := range m.config.AvailableModels {
		if model.ID == id {
			return model, true
		}
	}
	return ModelInfo{}, false
}

func defaultModels() []ModelInfo {
	return []ModelInfo{
		{Provider: "codex", ID: "gpt-5.6", ContextWindow: modelcatalog.GPT56ContextWindow},
		{Provider: "codex", ID: "gpt-5.6-sol", ContextWindow: modelcatalog.GPT56ContextWindow},
		{Provider: "codex", ID: "gpt-5.6-terra", ContextWindow: modelcatalog.GPT56ContextWindow},
		{Provider: "codex", ID: "gpt-5.6-luna", ContextWindow: modelcatalog.GPT56ContextWindow},
		{Provider: "codex", ID: "gpt-5.4", ContextWindow: modelcatalog.GPT54ContextWindow},
	}
}

func normalizeModels(models []ModelInfo, defaultProvider, defaultModel string) []ModelInfo {
	seen := make(map[string]bool, len(models)+1)
	normalized := make([]ModelInfo, 0, len(models)+1)
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" || seen[model.ID] {
			continue
		}
		if model.Provider == "" {
			model.Provider = defaultProvider
		}
		seen[model.ID] = true
		normalized = append(normalized, model)
	}
	if !seen[defaultModel] {
		normalized = append([]ModelInfo{{Provider: defaultProvider, ID: defaultModel}}, normalized...)
	}
	return normalized
}
