package daemon

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/dire-kiwi/dire-agent/capability"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/skills"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func (m *Manager) UpdateSettings(ctx context.Context, id string, update SettingsUpdate) (threadstore.Thread, error) {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return threadstore.Thread{}, err
	}
	runtime.mu.Lock()
	if runtime.running || runtime.finishing {
		runtime.mu.Unlock()
		return threadstore.Thread{}, errors.New("daemon: cannot change conversation settings while it is running")
	}
	if update.Model != nil && strings.TrimSpace(*update.Model) == "" {
		runtime.mu.Unlock()
		return threadstore.Thread{}, errors.New("daemon: model must not be empty")
	}
	var category string
	if update.Category != nil {
		if runtime.thread.ResourceKind() != threadstore.KindProject || runtime.thread.IsSubagent() {
			runtime.mu.Unlock()
			return threadstore.Thread{}, errors.New("daemon: categories are only available for top-level projects")
		}
		category, err = normalizeProjectCategory(*update.Category)
		if err != nil {
			runtime.mu.Unlock()
			return threadstore.Thread{}, err
		}
	}
	var additionalFolders []string
	if update.AdditionalFolders != nil {
		if runtime.thread.ResourceKind() != threadstore.KindProject || runtime.thread.IsSubagent() {
			runtime.mu.Unlock()
			return threadstore.Thread{}, errors.New("daemon: additional sandbox folders are only available for top-level projects")
		}
		additionalFolders, err = canonicalAdditionalFolders(runtime.thread.CWD, *update.AdditionalFolders)
		if err != nil {
			runtime.mu.Unlock()
			return threadstore.Thread{}, err
		}
	}
	if update.ThinkingLevel != nil && !validThinkingLevel(*update.ThinkingLevel) {
		runtime.mu.Unlock()
		return threadstore.Thread{}, errors.New("daemon: thinking level must be none, minimal, low, medium, high, xhigh, or max")
	}
	if update.SteeringMode != nil && !validQueueMode(*update.SteeringMode) {
		runtime.mu.Unlock()
		return threadstore.Thread{}, errors.New("daemon: steering mode must be all or one-at-a-time")
	}
	if update.FollowUpMode != nil && !validQueueMode(*update.FollowUpMode) {
		runtime.mu.Unlock()
		return threadstore.Thread{}, errors.New("daemon: follow-up mode must be all or one-at-a-time")
	}
	var mcpServerOverrides map[string]bool
	if update.MCPServer != nil {
		name := strings.TrimSpace(update.MCPServer.Name)
		if name == "" {
			runtime.mu.Unlock()
			return threadstore.Thread{}, errors.New("daemon: MCP server name is required")
		}
		if update.MCPServer.Enabled != nil && m.config.Settings != nil {
			settings, settingsErr := m.runtimeSettings(ctx, configScopeID(runtime.thread))
			if settingsErr != nil {
				runtime.mu.Unlock()
				return threadstore.Thread{}, settingsErr
			}
			if _, configured := settings.MCP.Servers[name]; !configured {
				runtime.mu.Unlock()
				return threadstore.Thread{}, errors.New("daemon: unknown MCP server " + name)
			}
		}
		if update.MCPServer.Enabled != nil && *update.MCPServer.Enabled && runtime.thread.IsSubagent() &&
			!subagentHasMCPServerGrant(runtime.thread, name) {
			runtime.mu.Unlock()
			return threadstore.Thread{}, errors.New("daemon: MCP server " + name + " was not granted when this child thread was spawned")
		}
		mcpServerOverrides = cloneBoolMap(runtime.thread.MCPServerOverrides)
		if mcpServerOverrides == nil {
			mcpServerOverrides = make(map[string]bool)
		}
		if update.MCPServer.Enabled == nil {
			delete(mcpServerOverrides, name)
		} else {
			mcpServerOverrides[name] = *update.MCPServer.Enabled
		}
	}
	var updatedSnapshot *capability.Snapshot
	if update.Tools != nil || update.AdditionalFolders != nil || update.MCPServer != nil {
		candidate := runtime.thread
		if update.Tools != nil {
			candidate.Tools = append([]string(nil), (*update.Tools)...)
		}
		if update.AdditionalFolders != nil {
			candidate.AdditionalFolders = append([]string(nil), additionalFolders...)
		}
		if update.MCPServer != nil {
			candidate.MCPServerOverrides = cloneBoolMap(mcpServerOverrides)
		}
		if candidate.ResourceKind() == threadstore.KindChat && len(candidate.Tools) != 0 {
			runtime.mu.Unlock()
			return threadstore.Thread{}, errors.New("daemon: standalone chats cannot enable project file or shell tools")
		}
		snapshot, resolveErr := m.resolveCapabilities(ctx, candidate)
		if resolveErr != nil {
			runtime.mu.Unlock()
			return threadstore.Thread{}, resolveErr
		}
		updatedSnapshot = &snapshot
	}

	previous := runtime.thread
	previousTools := runtime.tools
	previousCapabilityInstructions := runtime.capabilityInstructions
	previousCapabilities := runtime.capabilities
	previousSkills := runtime.skills
	previousDiagnostics := runtime.skillDiagnostics
	previousPreparePrompt := runtime.preparePrompt
	previousHooks := runtime.hooks
	previousCommands := runtime.commands
	if update.Name != nil {
		runtime.thread.Name = *update.Name
	}
	if update.Category != nil {
		runtime.thread.Category = category
	}
	if update.AdditionalFolders != nil {
		runtime.thread.AdditionalFolders = append([]string(nil), additionalFolders...)
	}
	if update.Model != nil && *update.Model != runtime.thread.Model {
		runtime.thread.Model = *update.Model
		runtime.thread.Usage.ContextTokens = 0
		runtime.thread.Usage.ContextWindow = 0
		if model, ok := m.modelInfo(runtime.thread.Model); ok {
			runtime.thread.Usage.ContextWindow = model.ContextWindow
		}
	}
	if update.ThinkingLevel != nil {
		runtime.thread.ThinkingLevel = *update.ThinkingLevel
	}
	if update.SteeringMode != nil {
		runtime.thread.SteeringMode = *update.SteeringMode
	}
	if update.FollowUpMode != nil {
		runtime.thread.FollowUpMode = *update.FollowUpMode
	}
	if update.MCPServer != nil {
		runtime.thread.MCPServerOverrides = cloneBoolMap(mcpServerOverrides)
	}
	if updatedSnapshot != nil {
		if update.Tools != nil {
			runtime.thread.Tools = append([]string(nil), (*update.Tools)...)
		}
		runtime.tools = updatedSnapshot.Tools
		runtime.capabilityInstructions = updatedSnapshot.Instructions
		runtime.capabilities = append([]capability.Descriptor(nil), updatedSnapshot.Descriptors...)
		runtime.skills = append([]skills.Skill(nil), updatedSnapshot.Skills...)
		runtime.skillDiagnostics = append([]skills.Diagnostic(nil), updatedSnapshot.Diagnostics...)
		runtime.preparePrompt = updatedSnapshot.PreparePrompt
		runtime.hooks = updatedSnapshot.Hooks
		runtime.commands = updatedSnapshot.Commands
	}
	if runtime.thread.Model != previous.Model ||
		runtime.capabilityInstructions != previousCapabilityInstructions ||
		!slices.Equal(runtime.thread.AdditionalFolders, previous.AdditionalFolders) {
		if err := runtime.reopenSessionLocked(ctx); err != nil {
			runtime.thread = previous
			runtime.tools = previousTools
			runtime.capabilityInstructions = previousCapabilityInstructions
			runtime.capabilities = previousCapabilities
			runtime.skills = previousSkills
			runtime.skillDiagnostics = previousDiagnostics
			runtime.preparePrompt = previousPreparePrompt
			runtime.hooks = previousHooks
			runtime.commands = previousCommands
			runtime.mu.Unlock()
			return threadstore.Thread{}, err
		}
	}
	runtime.thread.UpdatedAt = time.Now().UTC()
	thread := runtime.thread
	thread.Tools = append([]string(nil), thread.Tools...)
	thread.AdditionalFolders = append([]string(nil), thread.AdditionalFolders...)
	thread.MCPServerOverrides = cloneBoolMap(thread.MCPServerOverrides)
	runtime.mu.Unlock()
	if err := runtime.persistThread(ctx); err != nil {
		return threadstore.Thread{}, err
	}
	if update.MCPServer != nil && !thread.IsSubagent() {
		m.refreshMCPDependents(ctx, thread.ID)
	}
	m.emitResourceUpdated(runtime, thread)
	if updatedSnapshot != nil {
		m.emit(ctx, runtime, "capabilities_updated", map[string]bool{"refreshed": true})
	}
	return thread, nil
}

func validateMCPServerOverrides(overrides map[string]bool, settings configuration.Settings) error {
	for name := range overrides {
		if _, configured := settings.MCP.Servers[name]; !configured {
			return errors.New("daemon: unknown MCP server " + name)
		}
	}
	return nil
}

func subagentHasMCPServerGrant(thread threadstore.Thread, server string) bool {
	toolPrefix := "mcp__" + server + "__"
	contextPrefix := "mcpctx__" + server + "__"
	for _, name := range thread.AgentTools {
		if strings.HasPrefix(name, toolPrefix) || strings.HasPrefix(name, contextPrefix) {
			return true
		}
	}
	return false
}

func validQueueMode(mode string) bool { return mode == "all" || mode == "one-at-a-time" }

func validThinkingLevel(level string) bool {
	switch level {
	case "off", "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}
