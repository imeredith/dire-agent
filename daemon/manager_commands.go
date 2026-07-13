package daemon

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/dire-kiwi/dire-agent/capability"
)

type CapabilityCommandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
}

func (m *Manager) CapabilityCommands(ctx context.Context, id string) ([]CapabilityCommandInfo, error) {
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := m.refreshCapabilities(ctx, runtime); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	commands := make([]CapabilityCommandInfo, 0, len(runtime.commands))
	for _, command := range runtime.commands {
		commands = append(commands, CapabilityCommandInfo{
			Name: command.Name, Description: command.Description, Source: command.Source,
		})
	}
	runtime.mu.Unlock()
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
	return commands, nil
}

func (m *Manager) ExecuteCapabilityCommand(ctx context.Context, id, name, arguments string) (capability.CommandResult, error) {
	name = strings.TrimSpace(strings.TrimPrefix(name, "/"))
	if name == "" {
		return capability.CommandResult{}, errors.New("daemon: capability command name is required")
	}
	runtime, err := m.getRuntime(ctx, id)
	if err != nil {
		return capability.CommandResult{}, err
	}
	if err := m.refreshCapabilities(ctx, runtime); err != nil {
		return capability.CommandResult{}, err
	}
	runtime.mu.Lock()
	command, ok := runtime.commands[name]
	runtime.mu.Unlock()
	if !ok {
		return capability.CommandResult{}, errors.New("daemon: unknown capability command " + name)
	}
	result, err := command.Execute(ctx, arguments)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(result.Prompt) != "" {
		if err := m.FollowUp(ctx, id, result.Prompt); err != nil {
			return result, err
		}
	}
	return result, nil
}
