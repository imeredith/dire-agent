package daemon

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/imeredith/dire-agent/agentloop"
	"github.com/imeredith/dire-agent/agentteam"
	"github.com/imeredith/dire-agent/capability"
	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/skills"
	"github.com/imeredith/dire-agent/threadstore"
	"github.com/imeredith/dire-agent/tools"
)

func (m *Manager) resolveCapabilities(ctx context.Context, resource threadstore.Thread) (capability.Snapshot, error) {
	var snapshot capability.Snapshot
	var err error
	if m.config.Capabilities == nil {
		builtins, err := m.toolsFor(resource)
		if err != nil {
			return capability.Snapshot{}, err
		}
		snapshot = capability.Snapshot{Tools: builtins, Commands: make(map[string]capability.Command)}
	} else {
		snapshot, err = m.config.Capabilities.Resolve(ctx, capability.Scope{
			ConversationID:    resource.ID,
			SettingsID:        configScopeID(resource),
			Kind:              resource.ResourceKind(),
			CWD:               resource.CWD,
			AdditionalFolders: append([]string(nil), resource.AdditionalFolders...),
			Builtins:          append([]string(nil), resource.Tools...),
		})
		if err != nil {
			return capability.Snapshot{}, err
		}
	}
	settings, err := m.runtimeSettings(ctx, configScopeID(resource))
	if err != nil {
		return capability.Snapshot{}, err
	}
	profiles := make(map[string]string, len(settings.Subagents.Profiles))
	for name, profile := range settings.Subagents.Profiles {
		profiles[name] = profile.Description
	}
	canSpawn := settings.Subagents.Enabled && resource.Depth < settings.Subagents.MaxDepth
	if resource.IsSubagent() {
		profile, exists := settings.Subagents.Profiles[resource.AgentProfile]
		canSpawn = canSpawn && exists && profile.CanSpawn
	}
	if snapshot.Tools == nil {
		snapshot.Tools = make(map[string]agentloop.Tool)
	}
	if resource.IsSubagent() {
		effectiveTools, err := m.effectiveSubagentTools(ctx, resource)
		if err != nil {
			return capability.Snapshot{}, err
		}
		allowed := make(map[string]bool, len(effectiveTools))
		for _, name := range effectiveTools {
			allowed[name] = true
		}
		removed := make(map[string]bool)
		for name := range snapshot.Tools {
			if !allowed[name] {
				delete(snapshot.Tools, name)
				removed[name] = true
			}
		}
		filtered := snapshot.Descriptors[:0]
		for _, descriptor := range snapshot.Descriptors {
			if !removed[descriptor.Name] {
				filtered = append(filtered, descriptor)
			}
		}
		snapshot.Descriptors = filtered
	}
	var teamTools map[string]agentloop.Tool
	if settings.Subagents.Enabled {
		teamTools = agentteam.Tools(m, agentteam.Scope{AgentID: resource.ID, CanSpawn: canSpawn, Profiles: profiles})
	}
	for name, tool := range teamTools {
		if _, exists := snapshot.Tools[name]; exists {
			return capability.Snapshot{}, errors.New("daemon: duplicate orchestration tool " + name)
		}
		snapshot.Tools[name] = tool
		definition := tool.Definition()
		snapshot.Descriptors = append(snapshot.Descriptors, capability.Descriptor{
			Name: name, Source: "subagent", Description: definition.Description, Enabled: true, Status: "ready",
		})
	}
	return snapshot, nil
}

func configScopeID(resource threadstore.Thread) string {
	if resource.RootID != "" {
		return resource.RootID
	}
	return resource.ID
}

func (m *Manager) refreshCapabilities(ctx context.Context, runtime *threadRuntime) error {
	resource := runtime.snapshotThread()
	snapshot, err := m.resolveCapabilities(ctx, resource)
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.running {
		return nil
	}
	if snapshot.Instructions != runtime.capabilityInstructions {
		if err := runtime.reopenSessionWithInstructionsLocked(ctx, snapshot.Instructions); err != nil {
			return err
		}
	}
	runtime.tools = snapshot.Tools
	runtime.capabilityInstructions = snapshot.Instructions
	runtime.capabilities = append([]capability.Descriptor(nil), snapshot.Descriptors...)
	runtime.skills = append([]skills.Skill(nil), snapshot.Skills...)
	runtime.skillDiagnostics = append([]skills.Diagnostic(nil), snapshot.Diagnostics...)
	runtime.preparePrompt = snapshot.PreparePrompt
	runtime.hooks = snapshot.Hooks
	runtime.commands = snapshot.Commands
	return nil
}

func (m *Manager) toolsFor(resource threadstore.Thread) (map[string]agentloop.Tool, error) {
	if resource.ResourceKind() == threadstore.KindChat {
		if len(resource.Tools) != 0 {
			return nil, errors.New("daemon: standalone chats cannot enable project file or shell tools")
		}
		return map[string]agentloop.Tool{}, nil
	}
	return tools.BuiltinsWithOptions(resource.CWD, resource.Tools, tools.BuiltinOptions{
		AdditionalFolders: resource.AdditionalFolders,
	})
}

func (m *Manager) AvailableTools() []string {
	names := tools.Names()
	sort.Strings(names)
	return names
}

func (m *Manager) CapabilityState(ctx context.Context, id string) (CapabilityState, error) {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return CapabilityState{}, err
	}
	if err := m.refreshCapabilities(ctx, runtime); err != nil {
		return CapabilityState{}, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return CapabilityState{
		Capabilities:     append([]capability.Descriptor(nil), runtime.capabilities...),
		Skills:           append([]skills.Skill(nil), runtime.skills...),
		SkillDiagnostics: append([]skills.Diagnostic(nil), runtime.skillDiagnostics...),
	}, nil
}

// RefreshCapabilities applies configuration and discovery changes to every
// loaded idle conversation. Running conversations keep their current snapshot
// until their next prompt.
func (m *Manager) RefreshCapabilities(ctx context.Context) error {
	m.mu.Lock()
	runtimes := make([]*threadRuntime, 0, len(m.runtimes))
	for _, runtime := range m.runtimes {
		runtimes = append(runtimes, runtime)
	}
	m.mu.Unlock()
	var result error
	for _, runtime := range runtimes {
		if err := m.refreshCapabilities(ctx, runtime); err != nil {
			result = errors.Join(result, err)
			continue
		}
		m.emit(ctx, runtime, "capabilities_updated", map[string]bool{"refreshed": true})
	}
	return result
}

func (m *Manager) runtimeSettings(ctx context.Context, id string) (configuration.Settings, error) {
	if m.config.Settings == nil {
		return configuration.Settings{}, nil
	}
	settings, _, err := m.config.Settings.RuntimeSettings(ctx, id)
	return settings, err
}

func combineInstructions(user, generated string) string {
	user = strings.TrimSpace(user)
	generated = strings.TrimSpace(generated)
	if user == "" {
		return generated
	}
	if generated == "" {
		return user
	}
	return user + "\n\n" + generated
}

func sessionInstructions(resource threadstore.Thread, capabilityInstructions string) string {
	generated := capabilityInstructions
	if resource.ResourceKind() == threadstore.KindProject {
		generated = combineInstructions(projectSandboxInstructions(resource), generated)
	}
	return combineInstructions(resource.Instructions, generated)
}

func projectSandboxInstructions(resource threadstore.Thread) string {
	var prompt strings.Builder
	prompt.WriteString("<project_sandbox>\n")
	prompt.WriteString("The main project folder is ")
	prompt.WriteString(strconv.Quote(resource.CWD))
	prompt.WriteString(". This is the primary working directory. Resolve relative file-tool paths from this folder, and run shell commands from this folder unless the user explicitly requests otherwise.\n")
	if len(resource.AdditionalFolders) == 0 {
		prompt.WriteString("No additional folders are included in the project sandbox.\n")
	} else {
		prompt.WriteString("The following additional folders are included in the project sandbox. Access them by canonical absolute path; they do not replace the main project folder:\n")
		for _, folder := range resource.AdditionalFolders {
			prompt.WriteString("- ")
			prompt.WriteString(strconv.Quote(folder))
			prompt.WriteByte('\n')
		}
	}
	prompt.WriteString("All other filesystem paths remain outside the project sandbox.\n")
	prompt.WriteString("</project_sandbox>")
	return prompt.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
