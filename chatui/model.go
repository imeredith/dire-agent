package chatui

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

type transcriptEntry struct {
	role string
	text string
}

type model struct {
	ctx         context.Context
	api         API
	thread      threadstore.Thread
	running     bool
	events      <-chan daemon.WireEvent
	entries     []transcriptEntry
	streamID    string
	stream      string
	reasoningID string
	reasoning   string

	textarea            textarea.Model
	viewport            viewport.Model
	width               int
	height              int
	status              string
	lastErr             string
	completionIndex     int
	completionDismissed bool

	initialPrompt string
	styles        styles
}

type styles struct {
	header    lipgloss.Style
	user      lipgloss.Style
	assistant lipgloss.Style
	reasoning lipgloss.Style
	tool      lipgloss.Style
	system    lipgloss.Style
	error     lipgloss.Style
	status    lipgloss.Style
	help      lipgloss.Style
	border    lipgloss.Style
}

func defaultStyles() styles {
	return styles{
		header:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")),
		user:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
		assistant: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")),
		reasoning: lipgloss.NewStyle().Foreground(lipgloss.Color("141")),
		tool:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")),
		system:    lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		error:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")),
		status:    lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		help:      lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		border:    lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
	}
}

func newModel(ctx context.Context, api API, state daemon.RuntimeState, messages []threadstore.Message, initialPrompt string) model {
	input := textarea.New()
	input.Placeholder = "Message the agent, or type /help"
	input.Prompt = "┃ "
	input.CharLimit = 64 * 1024
	input.ShowLineNumbers = false
	input.SetVirtualCursor(false)
	input.SetHeight(3)
	input.KeyMap.InsertNewline.SetKeys("ctrl+j", "shift+enter")
	input.Focus()
	inputStyles := input.Styles()
	inputStyles.Focused.CursorLine = lipgloss.NewStyle()
	input.SetStyles(inputStyles)

	view := viewport.New(viewport.WithWidth(80), viewport.WithHeight(16))
	view.SoftWrap = true
	view.KeyMap.Left.SetEnabled(false)
	view.KeyMap.Right.SetEnabled(false)

	entries := make([]transcriptEntry, 0, len(messages)+1)
	for _, message := range messages {
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		role := message.Role
		if role == "" {
			role = message.Kind
		}
		text := message.Content
		if role == "tool" {
			var data struct {
				ToolName  string          `json:"tool_name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(message.Data, &data)
			text = formatToolEntry(data.ToolName, data.Arguments, text)
		}
		entries = append(entries, transcriptEntry{role: role, text: text})
	}
	if initialPrompt != "" {
		entries = append(entries, transcriptEntry{role: "user", text: initialPrompt})
	}

	conversation := state.Conversation
	if conversation.ID == "" {
		conversation = state.Project
	}
	if conversation.ID == "" {
		conversation = state.Chat
	}
	if conversation.ID == "" {
		conversation = state.Thread
	}
	if !usagePresent(conversation.Usage) && usagePresent(state.Usage) {
		conversation.Usage = state.Usage
	}
	m := model{
		ctx: ctx, api: api, thread: conversation, running: state.Running || initialPrompt != "",
		events: api.Events(), entries: entries, textarea: input, viewport: view,
		width: 80, height: 24, status: "ready", initialPrompt: initialPrompt, styles: defaultStyles(),
	}
	m.resize(80, 24)
	m.refreshTranscript(true)
	return m
}

func (m model) Init() tea.Cmd {
	commands := []tea.Cmd{textarea.Blink, waitForEvent(m.events)}
	if m.initialPrompt != "" {
		prompt := m.initialPrompt
		commands = append(commands, request("prompt", func() (threadstore.Thread, error) {
			return threadstore.Thread{}, m.api.Prompt(m.ctx, m.thread.ID, prompt, "")
		}))
	}
	return tea.Batch(commands...)
}

type eventMsg daemon.WireEvent
type connectionClosedMsg struct{}

type requestResultMsg struct {
	kind   string
	thread threadstore.Thread
	err    error
}

type agentRequestResultMsg struct {
	kind string
	text string
	err  error
}

func waitForEvent(events <-chan daemon.WireEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return connectionClosedMsg{}
		}
		return eventMsg(event)
	}
}

func request(kind string, invoke func() (threadstore.Thread, error)) tea.Cmd {
	return func() tea.Msg {
		thread, err := invoke()
		return requestResultMsg{kind: kind, thread: thread, err: err}
	}
}

func agentRequest(kind string, invoke func() (string, error)) tea.Cmd {
	return func() tea.Msg {
		text, err := invoke()
		return agentRequestResultMsg{kind: kind, text: text, err: err}
	}
}
