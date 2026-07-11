package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/imeredith/dire-agent/agentteam"
	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/threadstore"
	"github.com/imeredith/dire-agent/tools"
)

var validAgentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// SpawnAgent creates a persistent child conversation and immediately starts
// its assigned task. Every child gets its own SQLite file and provider session.
func (m *Manager) SpawnAgent(ctx context.Context, request agentteam.SpawnRequest) (agentteam.Agent, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Task = strings.TrimSpace(request.Task)
	request.Profile = strings.TrimSpace(request.Profile)
	request.Role = strings.TrimSpace(request.Role)
	request.Model = strings.TrimSpace(request.Model)
	request.Thinking = strings.TrimSpace(request.Thinking)
	if request.ParentID == "" {
		return agentteam.Agent{}, errors.New("daemon: parent agent id is required")
	}
	if !validAgentName.MatchString(request.Name) {
		return agentteam.Agent{}, errors.New("daemon: agent name must be 1-64 letters, digits, dots, dashes, or underscores")
	}
	if request.Task == "" || len(request.Task) > 100_000 {
		return agentteam.Agent{}, errors.New("daemon: agent task must be between 1 and 100000 bytes")
	}
	if len(request.Role) > 256 {
		return agentteam.Agent{}, errors.New("daemon: agent role must not exceed 256 bytes")
	}
	if request.Thinking != "" && !validThinkingLevel(request.Thinking) {
		return agentteam.Agent{}, errors.New("daemon: subagent thinking level is invalid")
	}

	parentRuntime, err := m.getRuntime(ctx, request.ParentID)
	if err != nil {
		return agentteam.Agent{}, err
	}
	if err := m.refreshCapabilities(ctx, parentRuntime); err != nil {
		return agentteam.Agent{}, err
	}
	parent := parentRuntime.snapshotThread()
	rootID := teamRootID(parent)
	settings, err := m.runtimeSettings(ctx, rootID)
	if err != nil {
		return agentteam.Agent{}, err
	}
	if request.Profile == "" {
		request.Profile = "general"
	}
	profile, ok := settings.Subagents.Profiles[request.Profile]
	if !ok {
		return agentteam.Agent{}, fmt.Errorf("daemon: unknown subagent profile %q", request.Profile)
	}
	if err := validateSpawnPolicy(parent, settings); err != nil {
		return agentteam.Agent{}, err
	}

	parentRuntime.mu.Lock()
	parentTools := make(map[string]bool, len(parentRuntime.tools))
	for name := range parentRuntime.tools {
		if !isTeamTool(name) {
			parentTools[name] = true
		}
	}
	parentRuntime.mu.Unlock()
	allowedTools, err := narrowAgentTools(parent, parentTools, profile, request.Tools)
	if err != nil {
		return agentteam.Agent{}, err
	}

	m.teamMu.Lock()
	defer m.teamMu.Unlock()
	team, err := m.listTeamThreads(ctx, rootID)
	if err != nil {
		return agentteam.Agent{}, err
	}
	if err := enforceTeamLimits(team, parent.ID, settings.Subagents); err != nil {
		return agentteam.Agent{}, err
	}
	id, err := newAgentID()
	if err != nil {
		return agentteam.Agent{}, err
	}
	child := m.newChildThread(id, parent, request, profile, allowedTools)
	created, err := m.createResource(ctx, child, "agent_created")
	if err != nil {
		return agentteam.Agent{}, err
	}
	m.emit(ctx, parentRuntime, "agent_spawned", agentFromThread(created))
	if err := m.Prompt(ctx, created.ID, request.Task, ""); err != nil {
		return agentteam.Agent{}, fmt.Errorf("daemon: start child agent: %w", err)
	}
	m.notifyTeamLocked(rootID)
	running, err := m.Thread(ctx, created.ID)
	if err != nil {
		return agentteam.Agent{}, err
	}
	return agentFromThread(running), nil
}

func (m *Manager) Agent(ctx context.Context, id string) (agentteam.Agent, error) {
	thread, err := m.Thread(ctx, id)
	if err != nil {
		return agentteam.Agent{}, err
	}
	if !thread.IsSubagent() {
		return agentteam.Agent{}, errors.New("daemon: conversation is not a subagent")
	}
	return agentFromThread(thread), nil
}

func (m *Manager) DeleteAgent(ctx context.Context, callerID, targetID string) error {
	_, target, _, err := m.authorizeTeamRoute(ctx, callerID, targetID)
	if err != nil {
		return err
	}
	if !target.IsSubagent() {
		return errors.New("daemon: the team root cannot be deleted as an agent")
	}
	return m.DeleteThread(ctx, target.ID)
}

func validateSpawnPolicy(parent threadstore.Thread, settings configuration.Settings) error {
	policy := settings.Subagents
	if !policy.Enabled {
		return errors.New("daemon: subagents are disabled")
	}
	if parent.Depth >= policy.MaxDepth {
		return fmt.Errorf("daemon: subagent depth limit %d reached", policy.MaxDepth)
	}
	if parent.IsSubagent() {
		profile, ok := policy.Profiles[parent.AgentProfile]
		if !ok || !profile.CanSpawn {
			return errors.New("daemon: this subagent profile cannot spawn children")
		}
	}
	return nil
}

func enforceTeamLimits(team []threadstore.Thread, parentID string, policy configuration.SubagentSettings) error {
	children, running := 0, 0
	for _, member := range team {
		if member.ParentID == parentID {
			children++
		}
		if member.IsSubagent() && member.Status == "running" {
			running++
		}
	}
	if children >= policy.MaxChildren {
		return fmt.Errorf("daemon: parent already has the maximum %d children", policy.MaxChildren)
	}
	if running >= policy.MaxConcurrent {
		return fmt.Errorf("daemon: team already has the maximum %d concurrent subagents", policy.MaxConcurrent)
	}
	return nil
}

func narrowAgentTools(parent threadstore.Thread, parentTools map[string]bool, profile configuration.AgentProfile, requested []string) ([]string, error) {
	if parent.ResourceKind() == threadstore.KindChat {
		if len(requested) != 0 {
			return nil, errors.New("daemon: standalone chat agents cannot use project tools")
		}
		return []string{}, nil
	}
	base := make([]string, 0, len(parentTools))
	if profile.Tools == nil {
		for name := range parentTools {
			base = append(base, name)
		}
		sort.Strings(base)
	} else {
		for _, name := range profile.Tools {
			if parentTools[name] {
				base = append(base, name)
			}
		}
	}
	if requested == nil {
		return base, nil
	}
	granted := make(map[string]bool, len(base))
	for _, name := range base {
		granted[name] = true
	}
	seen := make(map[string]bool, len(requested))
	result := make([]string, 0, len(requested))
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return nil, fmt.Errorf("daemon: invalid or duplicate requested tool %q", name)
		}
		if !granted[name] {
			return nil, fmt.Errorf("daemon: requested tool %q is not granted by the parent and profile", name)
		}
		seen[name] = true
		result = append(result, name)
	}
	return result, nil
}

func (m *Manager) newChildThread(id string, parent threadstore.Thread, request agentteam.SpawnRequest, profile configuration.AgentProfile, allowed []string) threadstore.Thread {
	builtin := make(map[string]bool)
	for _, name := range tools.Names() {
		builtin[name] = true
	}
	localTools := make([]string, 0, len(allowed))
	for _, name := range allowed {
		if builtin[name] {
			localTools = append(localTools, name)
		}
	}
	model := firstNonEmpty(request.Model, profile.Model, parent.Model)
	thinking := firstNonEmpty(request.Thinking, string(profile.Thinking), parent.ThinkingLevel)
	child := threadstore.Thread{
		ID: id, Kind: parent.ResourceKind(), ParentID: parent.ID, RootID: teamRootID(parent),
		AgentName: request.Name, AgentRole: request.Role, AgentProfile: request.Profile,
		AgentTools: append([]string(nil), allowed...), Depth: parent.Depth + 1,
		Name: request.Name, Model: model, CWD: parent.CWD,
		AdditionalFolders: append([]string(nil), parent.AdditionalFolders...),
		Instructions:      combineInstructions(profile.Instructions, childAgentInstructions(parent, request)),
		ThinkingLevel:     thinking, SteeringMode: parent.SteeringMode, FollowUpMode: parent.FollowUpMode,
		Tools: localTools, Status: "idle",
	}
	if modelInfo, ok := m.modelInfo(model); ok {
		child.Usage.ContextWindow = modelInfo.ContextWindow
	}
	return child
}

func childAgentInstructions(parent threadstore.Thread, request agentteam.SpawnRequest) string {
	role := request.Role
	if role == "" {
		role = "independent collaborator"
	}
	return fmt.Sprintf("You are subagent %q (%s), a %s in conversation team %q. Work on the assigned task, stay within inherited permissions, and communicate useful findings or blockers to your parent with send_agent_message. Do not claim work you have not verified.", request.Name, request.Profile, role, teamRootID(parent))
}

func newTeamID(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value[:]), nil
}

func newAgentID() (string, error) { return newTeamID("agent_") }

func agentFromThread(thread threadstore.Thread) agentteam.Agent {
	return agentteam.Agent{
		ID: thread.ID, ParentID: thread.ParentID, RootID: teamRootID(thread),
		Name: firstNonEmpty(thread.AgentName, thread.Name, thread.ID), Role: thread.AgentRole,
		Profile: thread.AgentProfile, Depth: thread.Depth, Status: thread.Status,
		Model: thread.Model, CreatedAt: thread.CreatedAt, UpdatedAt: thread.UpdatedAt,
	}
}

func teamRootID(thread threadstore.Thread) string {
	if thread.RootID != "" {
		return thread.RootID
	}
	return thread.ID
}

func isTeamTool(name string) bool {
	switch name {
	case "spawn_agent", "list_agents", "send_agent_message", "wait_agents", "interrupt_agent":
		return true
	default:
		return false
	}
}

// Compile-time assertion keeps the orchestration tools and manager in sync.
var _ agentteam.Backend = (*Manager)(nil)
