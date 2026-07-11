package threadstore

import (
	"encoding/json"
	"time"

	"github.com/imeredith/dire-agent/agent"
)

type Thread struct {
	ID                string        `json:"id"`
	Kind              string        `json:"kind,omitempty"`
	SettingsID        string        `json:"settings_id,omitempty"`
	ParentID          string        `json:"parent_id,omitempty"`
	RootID            string        `json:"root_id,omitempty"`
	AgentName         string        `json:"agent_name,omitempty"`
	AgentRole         string        `json:"agent_role,omitempty"`
	AgentProfile      string        `json:"agent_profile,omitempty"`
	AgentTools        []string      `json:"agent_tools,omitempty"`
	Depth             int           `json:"depth,omitempty"`
	Name              string        `json:"name,omitempty"`
	Category          string        `json:"category,omitempty"`
	Model             string        `json:"model"`
	CWD               string        `json:"cwd"`
	Worktree          *WorktreeInfo `json:"worktree,omitempty"`
	AdditionalFolders []string      `json:"additional_folders,omitempty"`
	Instructions      string        `json:"instructions,omitempty"`
	ThinkingLevel     string        `json:"thinking_level"`
	SteeringMode      string        `json:"steering_mode"`
	FollowUpMode      string        `json:"follow_up_mode"`
	Tools             []string      `json:"tools"`
	Usage             agent.Usage   `json:"usage"`
	Status            string        `json:"status"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

// WorktreeInfo records the source and immutable Git starting point for a
// daemon-managed checkout. Published worktrees intentionally outlive their
// conversation history, so deleting a Thread never removes this path.
type WorktreeInfo struct {
	SourceCWD           string `json:"source_cwd"`
	SourceRepository    string `json:"source_repository"`
	Path                string `json:"path"`
	ProjectRelativePath string `json:"project_relative_path,omitempty"`
	BaseRef             string `json:"base_ref"`
	BaseCommit          string `json:"base_commit"`
	EnvironmentID       string `json:"environment_id,omitempty"`
}

// Project is the public name for a folder-scoped persistent conversation.
// Thread remains as a compatibility name for existing databases and clients.
type Project = Thread

// Chat and Conversation are project-independent and generic public names for
// the same persisted metadata record. Older records without Kind are projects.
type Chat = Thread
type Conversation = Thread

const (
	KindProject = "project"
	KindChat    = "chat"
)

func (t Thread) ResourceKind() string {
	if t.Kind == KindChat {
		return KindChat
	}
	return KindProject
}

func (t Thread) IsSubagent() bool { return t.ParentID != "" }

type Message struct {
	Sequence  int64           `json:"sequence"`
	Kind      string          `json:"kind"`
	Role      string          `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type Event struct {
	Sequence  int64           `json:"sequence"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type State struct {
	Provider  string          `json:"provider"`
	SessionID string          `json:"session_id"`
	Data      json.RawMessage `json:"data"`
	UpdatedAt time.Time       `json:"updated_at"`
}
