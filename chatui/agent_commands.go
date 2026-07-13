package chatui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dire-kiwi/dire-agent/agentteam"
)

func (m model) agentCommand(input parsedInput) tea.Cmd {
	return agentRequest(input.kind, func() (string, error) {
		switch input.kind {
		case "agents":
			agents, err := m.api.ListAgents(m.ctx, m.thread.ID)
			return formatAgents(agents), err
		case "spawn":
			request, err := parseSpawnRequest(m.thread.ID, input.argument)
			if err != nil {
				return "", err
			}
			agent, err := m.api.SpawnAgent(m.ctx, request)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("spawned %s (%s), profile=%s status=%s", agent.Name, agent.ID, agent.Profile, agent.Status), nil
		case "message":
			id, message, ok := strings.Cut(strings.TrimSpace(input.argument), " ")
			if !ok || strings.TrimSpace(message) == "" {
				return "", errors.New("usage: /message AGENT_ID TEXT")
			}
			delivered, err := m.api.SendAgentMessage(m.ctx, m.thread.ID, id, strings.TrimSpace(message), true)
			if err != nil {
				return "", err
			}
			return "message delivered: " + delivered.ID, nil
		case "wait":
			result, err := m.api.WaitAgents(m.ctx, m.thread.ID, strings.Fields(input.argument), 30*time.Second)
			if err != nil {
				return "", err
			}
			return formatWaitResult(result), nil
		case "interrupt":
			err := m.api.InterruptAgent(m.ctx, m.thread.ID, strings.TrimSpace(input.argument))
			return "interrupt requested", err
		case "delete-agent":
			err := m.api.DeleteAgent(m.ctx, m.thread.ID, strings.TrimSpace(input.argument))
			return "agent deleted", err
		default:
			return "", errors.New("unsupported agent command")
		}
	})
}

func parseSpawnRequest(parentID, argument string) (agentteam.SpawnRequest, error) {
	argument = strings.TrimSpace(argument)
	head, task, hasSeparator := strings.Cut(argument, " -- ")
	fields := strings.Fields(head)
	profile := "general"
	if hasSeparator {
		if len(fields) < 1 || len(fields) > 2 || strings.TrimSpace(task) == "" {
			return agentteam.SpawnRequest{}, errors.New("usage: /spawn NAME [PROFILE] -- TASK")
		}
		if len(fields) == 2 {
			profile = fields[1]
		}
	} else {
		fields = strings.Fields(argument)
		if len(fields) < 2 {
			return agentteam.SpawnRequest{}, errors.New("usage: /spawn NAME TASK")
		}
		task = strings.Join(fields[1:], " ")
		fields = fields[:1]
	}
	return agentteam.SpawnRequest{ParentID: parentID, Name: fields[0], Profile: profile, Task: strings.TrimSpace(task)}, nil
}

func formatAgents(agents []agentteam.Agent) string {
	if len(agents) == 0 {
		return "No agents in this conversation."
	}
	lines := []string{"Agent tree:"}
	for _, agent := range agents {
		indent := strings.Repeat("  ", agent.Depth)
		lines = append(lines, fmt.Sprintf("%s- %s (%s) [%s] profile=%s", indent, agent.Name, agent.ID, agent.Status, agent.Profile))
	}
	return strings.Join(lines, "\n")
}

func formatWaitResult(result agentteam.WaitResult) string {
	text := formatAgents(result.Agents)
	if result.TimedOut {
		text += "\nWait timed out."
	}
	for _, message := range result.Messages {
		text += fmt.Sprintf("\nMessage from %s: %s", message.FromID, message.Content)
	}
	return text
}
