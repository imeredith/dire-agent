// Package schedulestore persists scheduled prompts independently from
// conversation databases.
package schedulestore

import "time"

const (
	TargetProject = "project"
	TargetChat    = "chat"
	TargetOneOff  = "one_off"

	ScheduleCron = "cron"
	ScheduleOnce = "once"
)

// Schedule describes a prompt that should be dispatched by the daemon at a
// later time. Runtime scheduling and dispatch are deliberately outside this
// package; Store only owns durable definitions and their latest run summary.
type Schedule struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Prompt             string     `json:"prompt"`
	TargetType         string     `json:"target_type"`
	ConversationID     string     `json:"conversation_id,omitempty"`
	ScheduleType       string     `json:"schedule_type"`
	Cron               string     `json:"cron,omitempty"`
	Timezone           string     `json:"timezone,omitempty"`
	RunAt              *time.Time `json:"run_at,omitempty"`
	Enabled            bool       `json:"enabled"`
	NextRunAt          *time.Time `json:"next_run_at,omitempty"`
	LastRunAt          *time.Time `json:"last_run_at,omitempty"`
	LastStatus         string     `json:"last_status,omitempty"`
	LastError          string     `json:"last_error,omitempty"`
	LastConversationID string     `json:"last_conversation_id,omitempty"`
	Pending            bool       `json:"pending,omitempty"`
	RetryPending       bool       `json:"retry_pending,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
