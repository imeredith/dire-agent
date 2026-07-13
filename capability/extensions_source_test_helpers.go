package capability

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/dire-kiwi/dire-agent/extensions"
)

type fakeExtensionConnector struct {
	mu          sync.Mutex
	specs       []extensions.ProcessSpec
	connections []*fakeExtensionConnection
}

func (f *fakeExtensionConnector) Connect(_ context.Context, spec extensions.ProcessSpec, _ extensions.Limits) (extensions.Connection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.specs = append(f.specs, spec)
	if spec.Command == "broken" {
		return nil, errors.New("failed using " + spec.Env["TOKEN"])
	}
	connection := &fakeExtensionConnection{}
	f.connections = append(f.connections, connection)
	return connection, nil
}

func (f *fakeExtensionConnector) connectCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.specs)
}

func (f *fakeExtensionConnector) lastSpec() extensions.ProcessSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.specs) == 0 {
		return extensions.ProcessSpec{}
	}
	return f.specs[len(f.specs)-1]
}

func (f *fakeExtensionConnector) closedCount() int {
	f.mu.Lock()
	connections := append([]*fakeExtensionConnection(nil), f.connections...)
	f.mu.Unlock()
	count := 0
	for _, connection := range connections {
		connection.mu.Lock()
		if connection.closed {
			count++
		}
		connection.mu.Unlock()
	}
	return count
}

type fakeExtensionConnection struct {
	mu     sync.Mutex
	closed bool
}

func (f *fakeExtensionConnection) Call(_ context.Context, method string, params any, result any) error {
	switch method {
	case "initialize":
		return copyExtensionJSON(result, map[string]any{
			"protocol_version": extensions.ProtocolVersion,
			"server":           map[string]string{"name": "fake"},
		})
	case "list_tools":
		return copyExtensionJSON(result, map[string]any{"tools": []map[string]any{{
			"name": "echo", "description": "Echo.",
			"input_schema": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]string{"type": "string"}}},
		}}})
	case "call_tool":
		contents, _ := json.Marshal(params)
		var call struct {
			Arguments struct {
				Value string `json:"value"`
			} `json:"arguments"`
		}
		_ = json.Unmarshal(contents, &call)
		return copyExtensionJSON(result, extensions.ToolResult{Output: call.Arguments.Value})
	case "shutdown":
		return copyExtensionJSON(result, struct{}{})
	default:
		return errors.New("unknown method")
	}
}

func (f *fakeExtensionConnection) Stderr() string { return "" }

func (f *fakeExtensionConnection) Close(context.Context) error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func copyExtensionJSON(destination, source any) error {
	contents, err := json.Marshal(source)
	if err != nil {
		return err
	}
	return json.Unmarshal(contents, destination)
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
