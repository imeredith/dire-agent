package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/dire-kiwi/dire-agent/agent"
)

// Tool exposes progressive list/load access without placing full skill bodies
// in every model request.
type Tool struct {
	Catalog *Catalog
}

// NewTool creates the agent-loop tool named "skill".
func NewTool(catalog *Catalog) *Tool {
	return &Tool{Catalog: catalog}
}

func (t *Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "skill",
		Description: "List available Agent Skills or load the complete instructions for one skill when needed.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["list","load"]},"name":{"type":"string","description":"Required when action is load."}},"required":["action"],"additionalProperties":false}`),
	}
}

func (t *Tool) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var input struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return "", fmt.Errorf("skill: decode input: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return "", err
	}
	if t == nil || t.Catalog == nil {
		return "", errors.New("skill: catalog is unavailable")
	}
	switch input.Action {
	case "list":
		if input.Name != "" {
			return "", errors.New("skill: name is only valid for load")
		}
		return t.Catalog.CatalogText(), nil
	case "load":
		if input.Name == "" {
			return "", errors.New("skill: name is required for load")
		}
		return t.Catalog.Load(input.Name)
	default:
		return "", fmt.Errorf("skill: unsupported action %q", input.Action)
	}
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("skill: input must contain one JSON object")
		}
		return fmt.Errorf("skill: decode input: %w", err)
	}
	return nil
}
