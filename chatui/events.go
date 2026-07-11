package chatui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

func (m *model) handleEvent(event daemon.WireEvent) {
	if event.ConversationID != m.thread.ID && event.ChatID != m.thread.ID && event.ProjectID != m.thread.ID && event.ThreadID != m.thread.ID {
		return
	}
	follow := m.viewport.AtBottom()
	switch event.Type {
	case "agent_start":
		m.running, m.status = true, "agent running"
	case "turn_start":
		m.status = "model thinking"
	case "message_start":
		var data struct {
			MessageID string `json:"message_id"`
		}
		_ = json.Unmarshal(event.Data, &data)
		m.streamID, m.stream, m.status = data.MessageID, "", "assistant responding"
	case "message_update":
		var data struct {
			MessageID string `json:"message_id"`
			Delta     string `json:"delta"`
		}
		_ = json.Unmarshal(event.Data, &data)
		if m.streamID != data.MessageID {
			m.streamID, m.stream = data.MessageID, ""
		}
		m.stream += data.Delta
	case "message_end":
		var data struct {
			MessageID string       `json:"message_id"`
			Text      string       `json:"text"`
			Usage     *agent.Usage `json:"usage,omitempty"`
		}
		_ = json.Unmarshal(event.Data, &data)
		text := data.Text
		if text == "" {
			text = m.stream
		}
		if strings.TrimSpace(text) != "" {
			m.entries = append(m.entries, transcriptEntry{role: "assistant", text: text})
		}
		m.stream, m.streamID = "", ""
		if data.Usage != nil {
			m.thread.Usage = accumulateUsage(m.thread.Usage, *data.Usage)
		}
	case "reasoning_start":
		var data struct {
			MessageID string `json:"message_id"`
		}
		_ = json.Unmarshal(event.Data, &data)
		m.reasoningID, m.reasoning, m.status = data.MessageID, "", "model thinking"
	case "reasoning_update":
		var data struct {
			MessageID string `json:"message_id"`
			Delta     string `json:"delta"`
		}
		_ = json.Unmarshal(event.Data, &data)
		if m.reasoningID != data.MessageID {
			m.reasoningID, m.reasoning = data.MessageID, ""
		}
		m.reasoning += data.Delta
	case "reasoning_end":
		var data struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(event.Data, &data)
		text := data.Text
		if text == "" {
			text = m.reasoning
		}
		if strings.TrimSpace(text) != "" {
			m.entries = append(m.entries, transcriptEntry{role: "reasoning", text: text})
		}
		m.reasoning, m.reasoningID = "", ""
	case "usage_update":
		var usage agent.Usage
		if json.Unmarshal(event.Data, &usage) == nil {
			m.thread.Usage = usage
		}
	case "tool_execution_start":
		var data struct {
			ToolName string `json:"tool_name"`
		}
		_ = json.Unmarshal(event.Data, &data)
		m.status = "running tool " + data.ToolName
	case "tool_execution_end":
		var data struct {
			ToolName  string          `json:"tool_name"`
			Arguments json.RawMessage `json:"arguments"`
			Output    string          `json:"output"`
			IsError   bool            `json:"is_error"`
		}
		_ = json.Unmarshal(event.Data, &data)
		output := data.Output
		if len(output) > 8000 {
			output = output[:8000] + "\n… output truncated in UI"
		}
		role := "tool"
		if data.IsError {
			role = "error"
		}
		m.entries = append(m.entries, transcriptEntry{role: role, text: formatToolEntry(data.ToolName, data.Arguments, output)})
		m.status = "tool finished"
	case "queue_update":
		var data struct {
			Steering int `json:"steering"`
			FollowUp int `json:"follow_ups"`
		}
		_ = json.Unmarshal(event.Data, &data)
		m.status = fmt.Sprintf("queued: %d steering, %d follow-up", data.Steering, data.FollowUp)
	case "agent_error":
		var data struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(event.Data, &data)
		m.lastErr = data.Error
		m.entries = append(m.entries, transcriptEntry{role: "error", text: "agent: " + data.Error})
		m.status = "agent failed"
	case "abort_requested":
		m.status = "abort requested"
	case "agent_settled":
		if strings.TrimSpace(m.reasoning) != "" {
			m.entries = append(m.entries, transcriptEntry{role: "reasoning", text: m.reasoning})
		}
		m.reasoning, m.reasoningID = "", ""
		m.running, m.status = false, "ready"
	case "chat_updated", "conversation_updated", "project_updated", "thread_updated":
		var thread threadstore.Thread
		if json.Unmarshal(event.Data, &thread) == nil && thread.ID != "" {
			m.thread = thread
		}
	}
	m.refreshTranscript(follow)
}

func formatToolEntry(name string, arguments json.RawMessage, output string) string {
	if name == "" {
		name = "tool"
	}
	parts := []string{name}
	if len(arguments) != 0 && string(arguments) != "null" {
		var value any
		if json.Unmarshal(arguments, &value) == nil {
			formatted, _ := json.MarshalIndent(value, "", "  ")
			parts = append(parts, "input:\n"+string(formatted))
		} else {
			parts = append(parts, "input: "+string(arguments))
		}
	}
	if strings.TrimSpace(output) == "" {
		output = "Completed without output."
	}
	parts = append(parts, "output:\n"+output)
	return strings.Join(parts, "\n")
}
