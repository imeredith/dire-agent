// Package tools contains the Pi-inspired local coding tools used by the daemon.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/agentloop"
)

const maxToolOutput = 1 << 20

type functionTool struct {
	definition agent.ToolDefinition
	execute    func(context.Context, json.RawMessage) (string, error)
}

func (t functionTool) Definition() agent.ToolDefinition { return t.definition }
func (t functionTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	return t.execute(ctx, input)
}

// Builtins returns the requested built-in tools rooted at the project's main
// cwd. Supported names match Pi's coding tools: read, bash, edit, write, grep,
// find, and ls.
func Builtins(cwd string, names []string) (map[string]agentloop.Tool, error) {
	return BuiltinsWithOptions(cwd, names, BuiltinOptions{})
}

// BuiltinsWithOptions is equivalent to Builtins, with additional sandbox roots
// and injectable bash settings for embedders and hermetic tests. Most callers
// should use Builtins so that the locked-down defaults cannot be accidentally
// weakened.
func BuiltinsWithOptions(cwd string, names []string, options BuiltinOptions) (map[string]agentloop.Tool, error) {
	paths, err := newPathSandbox(cwd, options.AdditionalFolders)
	if err != nil {
		return nil, err
	}
	options.AdditionalFolders = append([]string(nil), paths.additional...)

	available := map[string]agentloop.Tool{
		"read":  readTool(paths),
		"write": writeTool(paths),
		"edit":  editTool(paths),
		"ls":    listTool(paths),
		"find":  findTool(paths),
		"grep":  grepTool(paths),
	}
	for _, requested := range names {
		name := strings.TrimSpace(requested)
		if name != "bash" {
			if _, ok := available[name]; !ok {
				return nil, fmt.Errorf("tools: unsupported tool %q", name)
			}
		}
	}

	if containsTool(names, "bash") {
		executor, err := newSandboxExecutor(paths.main, options)
		if err != nil {
			return nil, err
		}
		available["bash"] = bashTool(paths.main, executor)
	}

	selected := make(map[string]agentloop.Tool, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		tool, ok := available[name]
		if !ok {
			return nil, fmt.Errorf("tools: unsupported tool %q", name)
		}
		selected[name] = tool
	}
	return selected, nil
}

func containsTool(names []string, wanted string) bool {
	for _, name := range names {
		if strings.TrimSpace(name) == wanted {
			return true
		}
	}
	return false
}

func definition(name, description, schema string) agent.ToolDefinition {
	return agent.ToolDefinition{Name: name, Description: description, Parameters: json.RawMessage(schema)}
}

// Names returns supported built-in names in stable order.
func Names() []string {
	names := []string{"read", "bash", "edit", "write", "grep", "find", "ls"}
	sort.Strings(names)
	return names
}
