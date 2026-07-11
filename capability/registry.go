package capability

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/imeredith/dire-agent/agentloop"
	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/skills"
	"github.com/imeredith/dire-agent/tools"
)

type RegistryConfig struct {
	Settings SettingsStore
	Defaults configuration.Settings
	Sources  []Source
}

type Registry struct {
	settings SettingsStore
	defaults configuration.Settings
	sources  []Source
}

func NewRegistry(config RegistryConfig) *Registry {
	return &Registry{
		settings: config.Settings,
		defaults: config.Defaults,
		sources:  append([]Source(nil), config.Sources...),
	}
}

func (r *Registry) Resolve(ctx context.Context, scope Scope) (Snapshot, error) {
	settingsID := scope.SettingsID
	if settingsID == "" {
		settingsID = scope.ConversationID
	}
	settings, err := r.resolveSettings(ctx, settingsID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Tools: make(map[string]agentloop.Tool), Commands: make(map[string]Command)}
	if scope.Kind != "chat" {
		builtins, err := tools.BuiltinsWithOptions(scope.CWD, scope.Builtins, tools.BuiltinOptions{
			AdditionalFolders: scope.AdditionalFolders,
		})
		if err != nil {
			return Snapshot{}, err
		}
		for name, tool := range builtins {
			snapshot.Tools[name] = tool
			snapshot.Descriptors = append(snapshot.Descriptors, descriptor(tool, "builtin", true, "ready"))
		}
	}

	var pluginRoots []skills.PluginRoot
	for _, source := range r.sources {
		fragment, err := source.Resolve(ctx, scope, settings)
		if err != nil {
			return Snapshot{}, fmt.Errorf("capability: resolve %s: %w", source.Name(), err)
		}
		for name, tool := range fragment.Tools {
			if _, exists := snapshot.Tools[name]; exists {
				return Snapshot{}, fmt.Errorf("capability: duplicate tool name %q", name)
			}
			snapshot.Tools[name] = tool
		}
		snapshot.Descriptors = append(snapshot.Descriptors, fragment.Descriptors...)
		pluginRoots = append(pluginRoots, fragment.PluginSkillRoots...)
		snapshot.Instructions = appendInstructions(snapshot.Instructions, fragment.Instructions)
		appendHooks(&snapshot.Hooks, fragment.Hooks)
		for name, command := range fragment.Commands {
			if _, exists := snapshot.Commands[name]; exists {
				return Snapshot{}, fmt.Errorf("capability: duplicate command name %q", name)
			}
			snapshot.Commands[name] = command
		}
	}

	catalog, err := skills.Discover(skills.Config{
		ProjectDir:    scope.CWD,
		GlobalRoots:   settings.Skills.Roots,
		PluginRoots:   pluginRoots,
		DisabledPaths: settings.Skills.Disabled,
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("capability: discover skills: %w", err)
	}
	snapshot.Skills = append([]skills.Skill(nil), catalog.Skills...)
	snapshot.Diagnostics = append([]skills.Diagnostic(nil), catalog.Diagnostics...)
	addSkills(&snapshot, catalog, settings.Skills.Trust)
	sort.Slice(snapshot.Descriptors, func(i, j int) bool {
		return snapshot.Descriptors[i].Name < snapshot.Descriptors[j].Name
	})
	return snapshot, nil
}

func (r *Registry) resolveSettings(ctx context.Context, id string) (configuration.Settings, error) {
	if r.settings == nil {
		return r.defaults, nil
	}
	settings, _, err := r.settings.RuntimeSettings(ctx, id)
	return settings, err
}

func (r *Registry) Close() error {
	var result error
	for _, source := range r.sources {
		result = errors.Join(result, source.Close())
	}
	return result
}

func descriptor(tool agentloop.Tool, source string, enabled bool, status string) Descriptor {
	definition := tool.Definition()
	return Descriptor{Name: definition.Name, Source: source, Description: definition.Description, Enabled: enabled, Status: status}
}

func addSkills(snapshot *Snapshot, catalog *skills.Catalog, trust configuration.TrustMode) {
	status := "approval_required"
	enable := trust == configuration.TrustTrusted && len(catalog.EnabledSkills()) > 0
	if trust == configuration.TrustDenied {
		status = "denied"
	} else if enable {
		status = "ready"
	}
	for _, skill := range catalog.Skills {
		description := strings.Join(strings.Fields(skill.Description), " ")
		snapshot.Descriptors = append(snapshot.Descriptors, Descriptor{
			Name: "skill:" + skill.Name, Source: "skill", Description: description,
			Enabled: enable && skill.Enabled, Status: status,
		})
	}
	if !enable {
		return
	}
	tool := skills.NewTool(catalog)
	if _, exists := snapshot.Tools[tool.Definition().Name]; exists {
		return
	}
	snapshot.Tools[tool.Definition().Name] = tool
	snapshot.Descriptors = append(snapshot.Descriptors, descriptor(tool, "skill", true, "ready"))
	snapshot.Instructions = appendInstructions(snapshot.Instructions, catalog.CatalogText())
	snapshot.PreparePrompt = skillPromptPreparer(catalog)
}

func appendInstructions(current, next string) string {
	current, next = strings.TrimSpace(current), strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "\n\n" + next
}

func appendHooks(destination *agentloop.Hooks, source agentloop.Hooks) {
	destination.BeforePrompt = append(destination.BeforePrompt, source.BeforePrompt...)
	destination.AfterModel = append(destination.AfterModel, source.AfterModel...)
	destination.BeforeTool = append(destination.BeforeTool, source.BeforeTool...)
	destination.AfterTool = append(destination.AfterTool, source.AfterTool...)
}

func skillPromptPreparer(catalog *skills.Catalog) func(context.Context, string) (string, error) {
	return func(ctx context.Context, prompt string) (string, error) {
		invocations, _ := catalog.ResolveInvocations(prompt)
		if len(invocations) == 0 {
			return prompt, nil
		}
		var expanded strings.Builder
		for _, invocation := range invocations {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			body, err := catalog.Load(invocation.Name)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&expanded, "<skill_instructions name=%q>\n%s\n", invocation.Name, body)
			if invocation.Args != "" {
				fmt.Fprintf(&expanded, "Arguments: %s\n", invocation.Args)
			}
			expanded.WriteString("</skill_instructions>\n\n")
		}
		expanded.WriteString("<user_request>\n")
		expanded.WriteString(prompt)
		expanded.WriteString("\n</user_request>")
		return expanded.String(), nil
	}
}
