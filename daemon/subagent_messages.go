package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func (m *Manager) ListAgents(ctx context.Context, callerID string) ([]agentteam.Agent, error) {
	caller, err := m.Thread(ctx, callerID)
	if err != nil {
		return nil, err
	}
	members, err := m.listTeamThreads(ctx, teamRootID(caller))
	if err != nil {
		return nil, err
	}
	agents := make([]agentteam.Agent, 0, len(members))
	for _, member := range members {
		agents = append(agents, agentFromThread(member))
	}
	sort.Slice(agents, func(i, j int) bool {
		if agents[i].Depth != agents[j].Depth {
			return agents[i].Depth < agents[j].Depth
		}
		return agents[i].CreatedAt.Before(agents[j].CreatedAt)
	})
	return agents, nil
}

func (m *Manager) SendAgentMessage(ctx context.Context, fromID, toID, content string, wake bool) (agentteam.Message, error) {
	content = strings.TrimSpace(content)
	if content == "" || len(content) > 100_000 {
		return agentteam.Message{}, errors.New("daemon: agent message must be between 1 and 100000 bytes")
	}
	from, to, _, err := m.authorizeTeamRoute(ctx, fromID, toID)
	if err != nil {
		return agentteam.Message{}, err
	}
	id, err := newTeamID("agentmsg_")
	if err != nil {
		return agentteam.Message{}, err
	}
	message := agentteam.Message{ID: id, FromID: from.ID, ToID: to.ID, Content: content, CreatedAt: time.Now().UTC()}
	data, _ := json.Marshal(message)
	targetRuntime, err := m.getRuntime(ctx, to.ID)
	if err != nil {
		return agentteam.Message{}, err
	}
	if _, err := targetRuntime.db.AppendMessage(ctx, threadstore.Message{
		Kind: "agent_message", Role: "agent", Content: content, Data: data,
	}); err != nil {
		return agentteam.Message{}, err
	}
	m.emit(ctx, targetRuntime, "agent_message", message)
	if sourceRuntime, sourceErr := m.getRuntime(ctx, from.ID); sourceErr == nil {
		m.emit(ctx, sourceRuntime, "agent_message_sent", message)
	}

	rootID := teamRootID(from)
	m.teamMu.Lock()
	m.teamMailboxes[to.ID] = append(m.teamMailboxes[to.ID], message)
	m.notifyTeamLocked(rootID)
	m.teamMu.Unlock()
	if !wake {
		return message, nil
	}
	prompt := fmt.Sprintf("[Message from %s (%s)]\n%s", firstNonEmpty(from.AgentName, from.Name, from.ID), from.ID, content)
	if err := m.Prompt(ctx, to.ID, prompt, "steer"); err != nil {
		m.emit(context.Background(), targetRuntime, "agent_message_wake_error", map[string]string{"error": err.Error(), "message_id": message.ID})
		return message, fmt.Errorf("daemon: message persisted but recipient could not be woken: %w", err)
	}
	return message, nil
}

func (m *Manager) WaitAgents(ctx context.Context, callerID string, ids []string, timeout time.Duration) (agentteam.WaitResult, error) {
	caller, err := m.Thread(ctx, callerID)
	if err != nil {
		return agentteam.WaitResult{}, err
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if timeout > 60*time.Second {
		return agentteam.WaitResult{}, errors.New("daemon: agent wait cannot exceed 60 seconds")
	}
	rootID := teamRootID(caller)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		m.teamMu.Lock()
		signal := m.teamSignalLocked(rootID)
		hasMessages := len(m.teamMailboxes[caller.ID]) != 0
		m.teamMu.Unlock()

		agents, err := m.ListAgents(ctx, caller.ID)
		if err != nil {
			return agentteam.WaitResult{}, err
		}
		selected, err := selectWaitAgents(caller, agents, ids)
		if err != nil {
			return agentteam.WaitResult{}, err
		}
		if hasMessages || agentsReady(selected) {
			return agentteam.WaitResult{Agents: selected, Messages: m.drainAgentMailbox(caller.ID)}, nil
		}
		select {
		case <-ctx.Done():
			return agentteam.WaitResult{}, ctx.Err()
		case <-deadline.C:
			messages := m.drainAgentMailbox(caller.ID)
			return agentteam.WaitResult{Agents: selected, Messages: messages, TimedOut: len(messages) == 0}, nil
		case <-signal:
		}
	}
}

func (m *Manager) drainAgentMailbox(id string) []agentteam.Message {
	m.teamMu.Lock()
	defer m.teamMu.Unlock()
	messages := append([]agentteam.Message(nil), m.teamMailboxes[id]...)
	delete(m.teamMailboxes, id)
	return messages
}

func (m *Manager) InterruptAgent(ctx context.Context, callerID, targetID string) error {
	from, target, _, err := m.authorizeTeamRoute(ctx, callerID, targetID)
	if err != nil {
		return err
	}
	if err := m.Abort(ctx, target.ID); err != nil {
		return err
	}
	m.notifyTeam(teamRootID(from))
	return nil
}

func (m *Manager) authorizeTeamRoute(ctx context.Context, fromID, toID string) (threadstore.Thread, threadstore.Thread, configuration.Settings, error) {
	if fromID == "" || toID == "" {
		return threadstore.Thread{}, threadstore.Thread{}, configuration.Settings{}, errors.New("daemon: source and target agent ids are required")
	}
	if fromID == toID {
		return threadstore.Thread{}, threadstore.Thread{}, configuration.Settings{}, errors.New("daemon: an agent cannot target itself")
	}
	from, err := m.Thread(ctx, fromID)
	if err != nil {
		return threadstore.Thread{}, threadstore.Thread{}, configuration.Settings{}, err
	}
	to, err := m.Thread(ctx, toID)
	if err != nil {
		return threadstore.Thread{}, threadstore.Thread{}, configuration.Settings{}, err
	}
	rootID := teamRootID(from)
	if rootID != teamRootID(to) {
		return threadstore.Thread{}, threadstore.Thread{}, configuration.Settings{}, errors.New("daemon: cross-team agent communication is not allowed")
	}
	settings, err := m.runtimeSettings(ctx, rootID)
	if err != nil {
		return threadstore.Thread{}, threadstore.Thread{}, configuration.Settings{}, err
	}
	members, err := m.listTeamThreads(ctx, rootID)
	if err != nil {
		return threadstore.Thread{}, threadstore.Thread{}, configuration.Settings{}, err
	}
	byID := make(map[string]threadstore.Thread, len(members))
	for _, member := range members {
		byID[member.ID] = member
	}
	if from.ID != rootID && to.ID != rootID && !isAncestor(from.ID, to.ID, byID) && !isAncestor(to.ID, from.ID, byID) && !settings.Subagents.AllowSiblingMessages {
		return threadstore.Thread{}, threadstore.Thread{}, configuration.Settings{}, errors.New("daemon: sibling agent communication is disabled")
	}
	return from, to, settings, nil
}

func (m *Manager) listTeamThreads(ctx context.Context, rootID string) ([]threadstore.Thread, error) {
	resources, err := m.config.Store.List(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	loaded := make(map[string]*threadRuntime, len(m.runtimes))
	for id, runtime := range m.runtimes {
		loaded[id] = runtime
	}
	m.mu.Unlock()
	team := make([]threadstore.Thread, 0)
	for _, resource := range resources {
		if resource.ID != rootID && resource.RootID != rootID {
			continue
		}
		if runtime := loaded[resource.ID]; runtime != nil {
			resource = runtime.snapshotThread()
		}
		if resource.Kind == "" {
			resource.Kind = threadstore.KindProject
		}
		team = append(team, resource)
	}
	return team, nil
}

func selectWaitAgents(caller threadstore.Thread, agents []agentteam.Agent, ids []string) ([]agentteam.Agent, error) {
	byID := make(map[string]agentteam.Agent, len(agents))
	threadMap := make(map[string]threadstore.Thread, len(agents))
	for _, candidate := range agents {
		byID[candidate.ID] = candidate
		threadMap[candidate.ID] = threadstore.Thread{ID: candidate.ID, ParentID: candidate.ParentID}
	}
	selected := make([]agentteam.Agent, 0)
	if len(ids) == 0 {
		for _, candidate := range agents {
			if candidate.ID != caller.ID && isAncestor(caller.ID, candidate.ID, threadMap) {
				selected = append(selected, candidate)
			}
		}
		return selected, nil
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == caller.ID {
			return nil, errors.New("daemon: an agent cannot wait on itself")
		}
		candidate, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("daemon: agent %q is not in this team", id)
		}
		if !seen[id] {
			selected = append(selected, candidate)
			seen[id] = true
		}
	}
	return selected, nil
}

func agentsReady(agents []agentteam.Agent) bool {
	if len(agents) == 0 {
		return true
	}
	for _, candidate := range agents {
		if candidate.Status != "running" {
			return true
		}
	}
	return false
}

func isAncestor(ancestorID, descendantID string, members map[string]threadstore.Thread) bool {
	current := members[descendantID]
	for current.ParentID != "" {
		if current.ParentID == ancestorID {
			return true
		}
		next, ok := members[current.ParentID]
		if !ok || next.ID == current.ID {
			return false
		}
		current = next
	}
	return false
}

func (m *Manager) teamSignalLocked(rootID string) chan struct{} {
	signal := m.teamSignals[rootID]
	if signal == nil {
		signal = make(chan struct{})
		m.teamSignals[rootID] = signal
	}
	return signal
}

func (m *Manager) notifyTeamLocked(rootID string) {
	if signal := m.teamSignals[rootID]; signal != nil {
		close(signal)
	}
	m.teamSignals[rootID] = make(chan struct{})
}

func (m *Manager) notifyTeam(rootID string) {
	m.teamMu.Lock()
	m.notifyTeamLocked(rootID)
	m.teamMu.Unlock()
}
