package chatui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.resize(message.Width, message.Height)
		return m, nil
	case tea.KeyPressMsg:
		completions := m.completions()
		switch message.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			if len(completions) > 0 {
				m.applyCompletion(completions)
				return m, nil
			}
		case "up", "down":
			if len(completions) > 0 {
				direction := 1
				if message.String() == "up" {
					direction = -1
				}
				m.completionIndex = (m.completionIndex + direction + len(completions)) % len(completions)
				m.syncLayout()
				return m, nil
			}
		case "esc":
			if len(completions) > 0 {
				m.completionDismissed = true
				m.syncLayout()
				return m, nil
			}
		case "enter":
			if len(completions) > 0 {
				m.applyCompletion(completions)
				return m, nil
			}
			return m.submit()
		case "pgup", "pgdown", "ctrl+up", "ctrl+down":
			var command tea.Cmd
			m.viewport, command = m.viewport.Update(message)
			return m, command
		}
	case eventMsg:
		m.handleEvent(daemon.WireEvent(message))
		return m, waitForEvent(m.events)
	case connectionClosedMsg:
		m.lastErr = "daemon connection closed"
		m.status = "disconnected"
		m.refreshTranscript(false)
		return m, nil
	case requestResultMsg:
		m.handleRequestResult(message)
		return m, nil
	case agentRequestResultMsg:
		m.handleAgentRequestResult(message)
		return m, nil
	}

	var commands []tea.Cmd
	var command tea.Cmd
	previousInput := m.textarea.Value()
	m.textarea, command = m.textarea.Update(message)
	commands = append(commands, command)
	if m.textarea.Value() != previousInput {
		m.completionIndex = 0
		m.completionDismissed = false
		m.syncLayout()
	}
	if _, ok := message.(tea.MouseMsg); ok {
		m.viewport, command = m.viewport.Update(message)
		commands = append(commands, command)
	}
	return m, tea.Batch(commands...)
}

func (m *model) completions() []slashCommandDefinition {
	if m.completionDismissed {
		return nil
	}
	return slashCommandSuggestions(m.textarea.Value())
}

func (m *model) applyCompletion(completions []slashCommandDefinition) {
	if len(completions) == 0 {
		return
	}
	if m.completionIndex >= len(completions) {
		m.completionIndex = len(completions) - 1
	}
	m.textarea.SetValue(completeSlashCommand(completions[m.completionIndex]))
	m.textarea.CursorEnd()
	m.completionIndex = 0
	m.completionDismissed = false
	m.syncLayout()
}

func (m model) submit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.textarea.Value())
	if input == "" {
		return m, nil
	}
	parsed := parseInput(input)
	if parsed.err != nil {
		m.lastErr = parsed.err.Error()
		m.status = "command rejected"
		return m, nil
	}
	switch parsed.kind {
	case "quit":
		return m, tea.Quit
	case "help":
		m.textarea.Reset()
		m.entries = append(m.entries, transcriptEntry{role: "system", text: helpText})
		m.refreshTranscript(true)
		return m, nil
	case "clear":
		m.textarea.Reset()
		m.entries = nil
		m.stream, m.streamID, m.reasoning, m.reasoningID, m.lastErr = "", "", "", "", ""
		m.status = "transcript cleared locally"
		m.refreshTranscript(true)
		return m, nil
	case "status":
		m.textarea.Reset()
		m.entries = append(m.entries, transcriptEntry{role: "system", text: m.statusText()})
		m.refreshTranscript(true)
		return m, nil
	case "folders":
		m.textarea.Reset()
		m.entries = append(m.entries, transcriptEntry{role: "system", text: m.folderText()})
		m.refreshTranscript(true)
		return m, nil
	}

	m.textarea.Reset()
	m.completionIndex, m.completionDismissed = 0, false
	m.lastErr = ""
	switch parsed.kind {
	case "prompt":
		m.entries = append(m.entries, transcriptEntry{role: "user", text: parsed.argument})
		if m.running {
			m.status = "queueing follow-up"
			m.refreshTranscript(true)
			return m, request("follow-up", func() (threadstore.Thread, error) {
				return threadstore.Thread{}, m.api.FollowUp(m.ctx, m.thread.ID, parsed.argument)
			})
		}
		m.running = true
		m.status = "sending prompt"
		m.refreshTranscript(true)
		return m, request("prompt", func() (threadstore.Thread, error) {
			return threadstore.Thread{}, m.api.Prompt(m.ctx, m.thread.ID, parsed.argument, "")
		})
	case "steer":
		m.entries = append(m.entries, transcriptEntry{role: "steer", text: parsed.argument})
		m.status = "sending steering message"
		m.refreshTranscript(true)
		return m, request("steer", func() (threadstore.Thread, error) {
			return threadstore.Thread{}, m.api.Steer(m.ctx, m.thread.ID, parsed.argument)
		})
	case "follow-up":
		m.entries = append(m.entries, transcriptEntry{role: "follow-up", text: parsed.argument})
		m.status = "queueing follow-up"
		m.refreshTranscript(true)
		return m, request("follow-up", func() (threadstore.Thread, error) {
			return threadstore.Thread{}, m.api.FollowUp(m.ctx, m.thread.ID, parsed.argument)
		})
	case "abort":
		m.status = "requesting abort"
		return m, request("abort", func() (threadstore.Thread, error) {
			return threadstore.Thread{}, m.api.Abort(m.ctx, m.thread.ID)
		})
	case "agents", "spawn", "message", "wait", "interrupt", "delete-agent":
		m.status = parsed.kind + " requested"
		return m, m.agentCommand(parsed)
	case "capability-commands", "capability-command":
		m.status = "extension command requested"
		return m, m.capabilityCommand(parsed)
	case "model":
		if parsed.argument == "" {
			m.addSystemEntry("model: " + m.thread.Model)
			return m, nil
		}
		m.status = "changing model"
		return m, request("model", func() (threadstore.Thread, error) {
			return m.api.SetModel(m.ctx, m.thread.ID, parsed.argument)
		})
	case "thinking":
		if parsed.argument == "" {
			m.addSystemEntry("thinking: " + m.thread.ThinkingLevel + " (none|minimal|low|medium|high|xhigh|max)")
			return m, nil
		}
		m.status = "changing thinking level"
		return m, request("thinking", func() (threadstore.Thread, error) {
			return m.api.SetThinkingLevel(m.ctx, m.thread.ID, parsed.argument)
		})
	case "name":
		if parsed.argument == "" {
			name := m.thread.Name
			if name == "" {
				name = "(unnamed)"
			}
			m.addSystemEntry("name: " + name)
			return m, nil
		}
		m.status = "renaming project"
		return m, request("name", func() (threadstore.Thread, error) {
			return m.api.SetThreadName(m.ctx, m.thread.ID, parsed.argument)
		})
	case "folder-add":
		folders := append([]string(nil), m.thread.AdditionalFolders...)
		folders = append(folders, parsed.argument)
		m.status = "adding sandbox folder"
		return m, request("folders", func() (threadstore.Thread, error) {
			return m.api.SetProjectAdditionalFolders(m.ctx, m.thread.ID, folders)
		})
	case "folder-remove":
		folders := make([]string, 0, len(m.thread.AdditionalFolders))
		for _, folder := range m.thread.AdditionalFolders {
			if folder != parsed.argument {
				folders = append(folders, folder)
			}
		}
		if len(folders) == len(m.thread.AdditionalFolders) {
			m.lastErr = "folder is not in the project sandbox; use /folders to list canonical paths"
			m.status = "folder unchanged"
			return m, nil
		}
		m.status = "removing sandbox folder"
		return m, request("folders", func() (threadstore.Thread, error) {
			return m.api.SetProjectAdditionalFolders(m.ctx, m.thread.ID, folders)
		})
	default:
		m.lastErr = "unsupported command"
		return m, nil
	}
}

func (m *model) handleAgentRequestResult(result agentRequestResultMsg) {
	if result.err != nil {
		m.lastErr = result.err.Error()
		m.status = result.kind + " failed"
		m.entries = append(m.entries, transcriptEntry{role: "error", text: result.kind + ": " + result.err.Error()})
		m.refreshTranscript(true)
		return
	}
	m.lastErr = ""
	m.status = result.kind + " complete"
	if strings.TrimSpace(result.text) != "" {
		m.addSystemEntry(result.text)
	}
}

func (m *model) addSystemEntry(text string) {
	m.entries = append(m.entries, transcriptEntry{role: "system", text: text})
	m.refreshTranscript(true)
}

func (m *model) handleRequestResult(result requestResultMsg) {
	if result.err != nil {
		m.lastErr = result.err.Error()
		m.status = result.kind + " failed"
		if result.kind == "prompt" {
			m.running = false
		}
		m.entries = append(m.entries, transcriptEntry{role: "error", text: result.kind + ": " + result.err.Error()})
		m.refreshTranscript(true)
		return
	}
	if result.thread.ID != "" {
		m.thread = result.thread
	}
	switch result.kind {
	case "prompt":
		m.running, m.status = true, "agent running"
	case "follow-up":
		m.status = "follow-up accepted"
	case "steer":
		m.status = "steering accepted"
	case "abort":
		m.status = "abort requested"
	case "model":
		m.status = "model changed to " + m.thread.Model
	case "thinking":
		m.status = "thinking changed to " + m.thread.ThinkingLevel
	case "name":
		m.status = "project renamed"
	case "folders":
		m.status = "sandbox folders updated"
	}
}

func (m model) folderText() string {
	if m.thread.ResourceKind() != threadstore.KindProject {
		return "This standalone chat has no project sandbox."
	}
	lines := []string{"main: " + m.thread.CWD}
	if len(m.thread.AdditionalFolders) == 0 {
		return strings.Join(append(lines, "additional: (none)"), "\n")
	}
	lines = append(lines, "additional:")
	for _, folder := range m.thread.AdditionalFolders {
		lines = append(lines, "  - "+folder)
	}
	return strings.Join(lines, "\n")
}
