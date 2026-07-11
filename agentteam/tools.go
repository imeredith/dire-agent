package agentteam

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/imeredith/dire-agent/agent"
	"github.com/imeredith/dire-agent/agentloop"
)

type tool struct {
	backend    Backend
	scope      Scope
	definition agent.ToolDefinition
	execute    func(context.Context, map[string]json.RawMessage) (any, error)
}

func Tools(backend Backend, scope Scope) map[string]agentloop.Tool {
	if backend == nil || scope.AgentID == "" {
		return nil
	}
	result := map[string]agentloop.Tool{}
	if scope.CanSpawn {
		result["spawn_agent"] = newSpawnTool(backend, scope)
	}
	result["list_agents"] = newListTool(backend, scope)
	result["send_agent_message"] = newSendTool(backend, scope)
	result["wait_agents"] = newWaitTool(backend, scope)
	result["interrupt_agent"] = newInterruptTool(backend, scope)
	return result
}

func (t *tool) Definition() agent.ToolDefinition { return t.definition }

func (t *tool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	input, err := decodeObject(raw)
	if err != nil {
		return "", err
	}
	result, err := t.execute(ctx, input)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func definition(name, description, schema string) agent.ToolDefinition {
	return agent.ToolDefinition{Name: name, Description: description, Parameters: json.RawMessage(schema)}
}

func newSpawnTool(backend Backend, scope Scope) *tool {
	profiles := make([]string, 0, len(scope.Profiles))
	for name, description := range scope.Profiles {
		profiles = append(profiles, name+": "+description)
	}
	sort.Strings(profiles)
	description := "Spawn a persistent child agent for an independent task. The child inherits this conversation's folder sandbox and cannot gain tools the parent lacks."
	if len(profiles) > 0 {
		description += " Profiles: " + strings.Join(profiles, "; ")
	}
	return &tool{backend: backend, scope: scope,
		definition: definition("spawn_agent", description, `{"type":"object","properties":{"name":{"type":"string"},"profile":{"type":"string"},"role":{"type":"string"},"task":{"type":"string"},"model":{"type":"string"},"thinking":{"type":"string"},"tools":{"type":"array","items":{"type":"string"}}},"required":["name","task"],"additionalProperties":false}`),
		execute: func(ctx context.Context, input map[string]json.RawMessage) (any, error) {
			var request SpawnRequest
			if err := remarshal(input, &request); err != nil {
				return nil, err
			}
			request.ParentID = scope.AgentID
			return backend.SpawnAgent(ctx, request)
		},
	}
}

func newListTool(backend Backend, scope Scope) *tool {
	return &tool{backend: backend, scope: scope,
		definition: definition("list_agents", "List the parent, children, and sibling agents in this conversation team with their current status.", `{"type":"object","additionalProperties":false}`),
		execute: func(ctx context.Context, _ map[string]json.RawMessage) (any, error) {
			return backend.ListAgents(ctx, scope.AgentID)
		},
	}
}

func newSendTool(backend Backend, scope Scope) *tool {
	return &tool{backend: backend, scope: scope,
		definition: definition("send_agent_message", "Send a durable message to the parent, a child, or a sibling agent. Wake starts an idle recipient or steers a running one.", `{"type":"object","properties":{"agent_id":{"type":"string"},"message":{"type":"string"},"wake":{"type":"boolean","default":true}},"required":["agent_id","message"],"additionalProperties":false}`),
		execute: func(ctx context.Context, input map[string]json.RawMessage) (any, error) {
			var args struct {
				AgentID string `json:"agent_id"`
				Message string `json:"message"`
				Wake    *bool  `json:"wake"`
			}
			if err := remarshal(input, &args); err != nil {
				return nil, err
			}
			wake := args.Wake == nil || *args.Wake
			return backend.SendAgentMessage(ctx, scope.AgentID, args.AgentID, args.Message, wake)
		},
	}
}

func newWaitTool(backend Backend, scope Scope) *tool {
	return &tool{backend: backend, scope: scope,
		definition: definition("wait_agents", "Wait briefly for selected child agents to settle or send messages. Omit agent_ids to watch the whole team.", `{"type":"object","properties":{"agent_ids":{"type":"array","items":{"type":"string"}},"timeout_ms":{"type":"integer","minimum":100,"maximum":60000,"default":30000}},"additionalProperties":false}`),
		execute: func(ctx context.Context, input map[string]json.RawMessage) (any, error) {
			var args struct {
				AgentIDs  []string `json:"agent_ids"`
				TimeoutMS int      `json:"timeout_ms"`
			}
			if err := remarshal(input, &args); err != nil {
				return nil, err
			}
			if args.TimeoutMS == 0 {
				args.TimeoutMS = 30000
			}
			if args.TimeoutMS < 100 || args.TimeoutMS > 60000 {
				return nil, errors.New("agentteam: timeout_ms must be between 100 and 60000")
			}
			return backend.WaitAgents(ctx, scope.AgentID, args.AgentIDs, time.Duration(args.TimeoutMS)*time.Millisecond)
		},
	}
}

func newInterruptTool(backend Backend, scope Scope) *tool {
	return &tool{backend: backend, scope: scope,
		definition: definition("interrupt_agent", "Cancel a running child or sibling agent without deleting its persistent conversation.", `{"type":"object","properties":{"agent_id":{"type":"string"}},"required":["agent_id"],"additionalProperties":false}`),
		execute: func(ctx context.Context, input map[string]json.RawMessage) (any, error) {
			var args struct {
				AgentID string `json:"agent_id"`
			}
			if err := remarshal(input, &args); err != nil {
				return nil, err
			}
			err := backend.InterruptAgent(ctx, scope.AgentID, args.AgentID)
			return map[string]bool{"interrupted": err == nil}, err
		},
	}
}

func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value map[string]json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("agentteam: decode input: %w", err)
	}
	if value == nil {
		return nil, errors.New("agentteam: input must be an object")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("agentteam: input must contain one object")
	}
	return value, nil
}

func remarshal(input map[string]json.RawMessage, destination any) error {
	data, _ := json.Marshal(input)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}
