package daemon

import (
	"encoding/json"

	"github.com/dire-kiwi/dire-agent/configuration"
)

// Command is the Pi-inspired WebSocket command envelope.
type Command struct {
	ID                string                `json:"id,omitempty"`
	Type              string                `json:"type"`
	ConversationID    string                `json:"conversation_id,omitempty"`
	ChatID            string                `json:"chat_id,omitempty"`
	ProjectID         string                `json:"project_id,omitempty"`
	ThreadID          string                `json:"thread_id,omitempty"`
	Message           string                `json:"message,omitempty"`
	Name              string                `json:"name,omitempty"`
	Folder            string                `json:"folder,omitempty"`
	Category          string                `json:"category,omitempty"`
	AdditionalFolders []string              `json:"additional_folders,omitempty"`
	StreamingBehavior string                `json:"streaming_behavior,omitempty"`
	Options           CreateThreadOptions   `json:"options,omitempty"`
	ChatOptions       CreateChatOptions     `json:"chat_options,omitempty"`
	After             int64                 `json:"after,omitempty"`
	Limit             int                   `json:"limit,omitempty"`
	Model             string                `json:"model,omitempty"`
	Level             string                `json:"level,omitempty"`
	Mode              string                `json:"mode,omitempty"`
	Tools             []string              `json:"tools,omitempty"`
	AgentID           string                `json:"agent_id,omitempty"`
	ParentID          string                `json:"parent_id,omitempty"`
	AgentName         string                `json:"agent_name,omitempty"`
	AgentRole         string                `json:"agent_role,omitempty"`
	Task              string                `json:"task,omitempty"`
	Profile           string                `json:"profile,omitempty"`
	AgentIDs          []string              `json:"agent_ids,omitempty"`
	Wake              *bool                 `json:"wake,omitempty"`
	TimeoutMS         int                   `json:"timeout_ms,omitempty"`
	CommandName       string                `json:"command_name,omitempty"`
	LauncherID        string                `json:"launcher_id,omitempty"`
	EnvironmentID     string                `json:"environment_id,omitempty"`
	Environment       *ProjectEnvironment   `json:"environment,omitempty"`
	ExpectedHash      string                `json:"expected_hash,omitempty"`
	Arguments         string                `json:"arguments,omitempty"`
	Attachments       []ImageAttachment     `json:"attachments,omitempty"`
	Config            *configuration.Config `json:"config,omitempty"`
	ExpectedRevision  uint64                `json:"expected_revision,omitempty"`
}

type Response struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Command string `json:"command"`
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

type ConversationScope struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type WireEvent struct {
	Type           string            `json:"type"`
	Scope          ConversationScope `json:"scope"`
	ConversationID string            `json:"conversation_id,omitempty"`
	ChatID         string            `json:"chat_id,omitempty"`
	ProjectID      string            `json:"project_id,omitempty"`
	ThreadID       string            `json:"thread_id"`
	Sequence       int64             `json:"sequence,omitempty"`
	Timestamp      string            `json:"timestamp"`
	Data           json.RawMessage   `json:"data,omitempty"`
}
