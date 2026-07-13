package chatui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dire-kiwi/dire-agent/threadstore"
)

func (m *model) resize(width, height int) {
	if width < 24 {
		width = 24
	}
	if height < 10 {
		height = 10
	}
	m.width, m.height = width, height
	m.textarea.SetWidth(width - 2)
	m.syncLayout()
}

func (m *model) syncLayout() {
	viewportHeight := m.height - m.textarea.Height() - 6 - m.completionRows()
	if viewportHeight < 3 {
		viewportHeight = 3
	}
	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(viewportHeight)
	m.refreshTranscript(true)
}

func (m *model) completionRows() int {
	rows := len(m.completions())
	if rows > 6 {
		return 6
	}
	return rows
}

func (m *model) refreshTranscript(forceBottom bool) {
	m.viewport.SetContent(m.renderTranscript())
	if forceBottom {
		m.viewport.GotoBottom()
	}
}

func (m model) renderTranscript() string {
	parts := make([]string, 0, len(m.entries)+3)
	if len(m.entries) == 0 && len(m.stream) == 0 && len(m.reasoning) == 0 {
		parts = append(parts, m.styles.system.Render("No messages yet. Type a prompt and press Enter."))
	}
	for _, entry := range m.entries {
		parts = append(parts, m.renderEntry(entry))
	}
	if m.reasoning != "" {
		parts = append(parts, m.styles.reasoning.Render("thinking › ")+cleanReasoningDisplay(m.reasoning))
	}
	if m.stream != "" {
		parts = append(parts, m.styles.assistant.Render("assistant › ")+m.stream)
	}
	return strings.Join(parts, "\n\n")
}

func (m model) renderEntry(entry transcriptEntry) string {
	switch entry.role {
	case "user":
		return m.styles.user.Render("you › ") + entry.text
	case "steer":
		return m.styles.user.Render("you /steer › ") + entry.text
	case "follow-up":
		return m.styles.user.Render("you /follow-up › ") + entry.text
	case "assistant":
		return m.styles.assistant.Render("assistant › ") + entry.text
	case "reasoning":
		return m.styles.reasoning.Render("thinking › ") + cleanReasoningDisplay(entry.text)
	case "tool":
		return m.styles.tool.Render("tool › ") + entry.text
	case "error":
		return m.styles.error.Render("error › ") + entry.text
	default:
		return m.styles.system.Render("system › ") + entry.text
	}
}

func cleanReasoningDisplay(text string) string {
	for {
		start := strings.Index(text, "<!--")
		if start < 0 {
			break
		}
		end := strings.Index(text[start+4:], "-->")
		if end < 0 {
			break
		}
		text = text[:start] + text[start+4+end+3:]
	}
	return strings.TrimSpace(text)
}

func (m model) renderCompletions() string {
	completions := m.completions()
	if len(completions) == 0 {
		return ""
	}
	index := m.completionIndex
	if index >= len(completions) {
		index = len(completions) - 1
	}
	start := 0
	if index >= 6 {
		start = index - 5
	}
	end := min(len(completions), start+6)
	lines := make([]string, 0, end-start)
	for current := start; current < end; current++ {
		marker := "  "
		style := m.styles.help
		if current == index {
			marker = "› "
			style = m.styles.assistant
		}
		command := completions[current]
		lines = append(lines, style.Render(fmt.Sprintf("%s/%-14s %s", marker, command.name, command.description)))
	}
	return strings.Join(lines, "\n")
}

func (m model) statusText() string {
	state := "idle"
	if m.running {
		state = "running"
	}
	name := m.thread.Name
	if name == "" {
		name = "unnamed"
	}
	identity := fmt.Sprintf("%s=%s name=%s", m.thread.ResourceKind(), m.thread.ID, name)
	if m.thread.ResourceKind() == threadstore.KindProject {
		identity += fmt.Sprintf(" folder=%s additional_folders=%d", m.thread.CWD, len(m.thread.AdditionalFolders))
	}
	return fmt.Sprintf("%s state=%s model=%s thinking=%s %s", identity, state, m.thread.Model, m.thread.ThinkingLevel, usageSummary(m.thread.Usage))
}

func (m model) View() tea.View {
	state, statusStyle := "○ idle", m.styles.system
	if m.running {
		state, statusStyle = "● running", m.styles.status
	}
	header := m.styles.header.Render("Dire Agent") + "  " + m.styles.system.Render(m.thread.ID)
	meta := statusStyle.Render(state) + m.styles.system.Render("  "+m.thread.Model+"  thinking:"+m.thread.ThinkingLevel)
	status := m.styles.status.Render(m.status)
	if m.lastErr != "" {
		status = m.styles.error.Render(m.lastErr)
	}
	sections := []string{
		header + "\n" + meta + "\n" + m.styles.system.Render(usageSummary(m.thread.Usage)),
		m.styles.border.Render(strings.Repeat("─", max(1, m.width))), m.viewport.View(),
	}
	if completions := m.renderCompletions(); completions != "" {
		sections = append(sections, completions)
	}
	sections = append(sections, m.textarea.View(), status,
		m.styles.help.Render("Enter send  Ctrl+J/Shift+Enter newline  PgUp/PgDn scroll  /help commands  Ctrl+C quit"),
	)
	content := strings.Join(sections, "\n")
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Dire Agent · " + m.thread.ID
	if cursor := m.textarea.Cursor(); cursor != nil {
		cursor.Position.Y += 4 + m.viewport.Height() + m.completionRows()
		view.Cursor = cursor
	}
	return view
}
