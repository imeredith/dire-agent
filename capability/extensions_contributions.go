package capability

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/extensions"
)

func addExtensionContributions(client *extensions.Client, id string, fragment *Fragment) {
	registration := client.Registration()
	for _, prompt := range registration.PromptFragments {
		fragment.Instructions = appendInstructions(fragment.Instructions,
			fmt.Sprintf("<extension_prompt source=%q id=%q>\n%s\n</extension_prompt>", id, prompt.ID, prompt.Text))
	}
	for _, command := range registration.Commands {
		name := "ext:" + extensionID(id) + ":" + extensionID(command.Name)
		if fragment.Commands == nil {
			fragment.Commands = make(map[string]Command)
		}
		remoteName := command.Name
		fragment.Commands[name] = Command{
			Name: name, Description: command.Description, Source: "extension:" + id,
			Execute: func(ctx context.Context, arguments string) (CommandResult, error) {
				result, err := client.ExecuteCommand(ctx, remoteName, arguments)
				return CommandResult{Output: result.Output, Prompt: result.Prompt, IsError: result.IsError}, err
			},
		}
		fragment.Descriptors = append(fragment.Descriptors, Descriptor{
			Name: "/" + name, Source: "extension:" + id, Description: command.Description,
			Enabled: true, Status: "ready",
		})
	}
	events := registeredHookEvents(registration.Hooks)
	if events[extensions.HookBeforePrompt] {
		fragment.Hooks.BeforePrompt = append(fragment.Hooks.BeforePrompt, extensionBeforePrompt(client, id))
	}
	if events[extensions.HookAfterModel] {
		fragment.Hooks.AfterModel = append(fragment.Hooks.AfterModel, extensionAfterModel(client, id))
	}
	if events[extensions.HookBeforeTool] {
		fragment.Hooks.BeforeTool = append(fragment.Hooks.BeforeTool, extensionBeforeTool(client, id))
	}
	if events[extensions.HookAfterTool] {
		fragment.Hooks.AfterTool = append(fragment.Hooks.AfterTool, extensionAfterTool(client, id))
	}
}

func registeredHookEvents(hooks []extensions.HookSpec) map[extensions.HookEvent]bool {
	result := make(map[extensions.HookEvent]bool)
	for _, hook := range hooks {
		result[hook.Event] = true
	}
	return result
}

func extensionBeforePrompt(client *extensions.Client, id string) func(context.Context, string) (string, error) {
	return func(ctx context.Context, prompt string) (string, error) {
		payload, veto, err := client.InvokeHooks(ctx, extensions.HookBeforePrompt, extensions.HookPayload{Prompt: prompt})
		return payload.Prompt, extensionHookError(id, veto, err)
	}
}

func extensionAfterModel(client *extensions.Client, id string) func(context.Context, *agent.StepResult) error {
	return func(ctx context.Context, result *agent.StepResult) error {
		payload, veto, err := client.InvokeHooks(ctx, extensions.HookAfterModel, extensions.HookPayload{ModelText: result.Text})
		if hookErr := extensionHookError(id, veto, err); hookErr != nil {
			return hookErr
		}
		result.Text, result.Result.Text = payload.ModelText, payload.ModelText
		return nil
	}
}

func extensionBeforeTool(client *extensions.Client, id string) func(context.Context, *agent.ToolCall) error {
	return func(ctx context.Context, call *agent.ToolCall) error {
		payload, veto, err := client.InvokeHooks(ctx, extensions.HookBeforeTool, extensions.HookPayload{
			ToolName: call.Name, Arguments: call.Arguments,
		})
		if hookErr := extensionHookError(id, veto, err); hookErr != nil {
			return hookErr
		}
		if payload.ToolName != "" {
			call.Name = payload.ToolName
		}
		if len(payload.Arguments) > 0 {
			call.Arguments = payload.Arguments
		}
		return nil
	}
}

func extensionAfterTool(client *extensions.Client, id string) func(context.Context, agent.ToolCall, *agent.ToolResult) error {
	return func(ctx context.Context, call agent.ToolCall, result *agent.ToolResult) error {
		payload, veto, err := client.InvokeHooks(ctx, extensions.HookAfterTool, extensions.HookPayload{
			ToolName: call.Name, Arguments: call.Arguments, Output: result.Output, IsError: result.IsError,
		})
		if hookErr := extensionHookError(id, veto, err); hookErr != nil {
			return hookErr
		}
		result.Output, result.IsError = payload.Output, payload.IsError
		return nil
	}
}

func extensionHookError(id string, veto *extensions.HookResult, err error) error {
	if err != nil {
		return fmt.Errorf("extension %s hook: %w", id, err)
	}
	if veto != nil && veto.Veto {
		message := strings.TrimSpace(veto.Message)
		if message == "" {
			message = "operation vetoed"
		}
		return errors.New("extension " + id + ": " + message)
	}
	return nil
}
