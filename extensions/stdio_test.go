package extensions

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStdioProtocolAndConcurrentCalls(t *testing.T) {
	literal := `literal;$(echo not-a-shell)`
	client, err := Open(context.Background(), LaunchConfig{
		ID: "stdio", Enabled: true, Trust: TrustTrusted,
		Process: ProcessSpec{
			Command: os.Args[0],
			Args:    []string{"-test.run=^TestExtensionHelperProcess$", "--", literal},
			Dir:     t.TempDir(),
			Env:     map[string]string{"DIRE_AGENT_EXTENSION_HELPER": "1"},
		},
	}, OpenOptions{Limits: Limits{
		InitializeTimeout: 2 * time.Second, CallTimeout: 2 * time.Second,
		CloseTimeout: 2 * time.Second, MaxStderrBytes: 64,
	}})
	if err != nil {
		t.Fatal(err)
	}
	tools := client.AgentTools()
	if _, ok := tools["ext__stdio__echo"]; !ok {
		t.Fatalf("tools = %#v", tools)
	}

	var wait sync.WaitGroup
	for index := 0; index < 16; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			arguments, _ := json.Marshal(map[string]string{"value": fmt.Sprint(index)})
			result, callErr := client.CallTool(context.Background(), "echo", arguments)
			if callErr != nil {
				t.Errorf("call %d: %v", index, callErr)
				return
			}
			if !strings.Contains(result.Output, fmt.Sprint(index)) || !strings.Contains(result.Output, literal) {
				t.Errorf("call %d output = %q", index, result.Output)
			}
		}()
	}
	wait.Wait()

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(client.Stderr(), "truncated") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stderr := client.Stderr(); len(stderr) > 64 || !strings.Contains(stderr, "truncated") {
		t.Fatalf("stderr = %q (%d bytes)", stderr, len(stderr))
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestExtensionHelperProcess(t *testing.T) {
	if os.Getenv("DIRE_AGENT_EXTENSION_HELPER") != "1" {
		return
	}
	_, _ = fmt.Fprintln(os.Stderr, strings.Repeat("extension-log-", 32))
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(2)
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		switch request.Method {
		case "initialize":
			response["result"] = map[string]any{
				"protocol_version": ProtocolVersion,
				"server":           map[string]string{"name": "test-helper", "version": "1"},
			}
		case "list_tools":
			response["result"] = map[string]any{"tools": []any{map[string]any{
				"name": "echo", "description": "Echo a value.",
				"input_schema": map[string]any{"type": "object"},
			}}}
		case "call_tool":
			var params struct {
				Arguments struct {
					Value string `json:"value"`
				} `json:"arguments"`
			}
			_ = json.Unmarshal(request.Params, &params)
			response["result"] = ToolResult{
				Output: params.Arguments.Value + " " + os.Args[len(os.Args)-1],
			}
		case "shutdown":
			response["result"] = map[string]any{}
			if encoder.Encode(response) != nil {
				os.Exit(3)
			}
			os.Exit(0)
		default:
			response["error"] = map[string]any{"code": -32601, "message": "unknown method"}
		}
		if encoder.Encode(response) != nil {
			os.Exit(3)
		}
	}
	os.Exit(0)
}

func TestModelNameIsStableAndBounded(t *testing.T) {
	if got := ModelName("Acme Plugin", "do thing"); got != "ext__acme_plugin__do_thing" {
		t.Fatalf("name = %q", got)
	}
	long := ModelName(strings.Repeat("extension", 20), strings.Repeat("tool", 20))
	if len(long) != maxModelToolName || !strings.HasPrefix(long, "ext__") {
		t.Fatalf("long model name = %q (%d)", long, len(long))
	}
	if long != ModelName(strings.Repeat("extension", 20), strings.Repeat("tool", 20)) {
		t.Fatal("model name is unstable")
	}
}

func TestChildEnvironmentIsExplicitByDefault(t *testing.T) {
	t.Setenv("DIRE_AGENT_PARENT_ONLY", "secret")
	withoutInheritance := mergedEnvironment(map[string]string{"EXPLICIT": "yes"}, false)
	if containsEnvironment(withoutInheritance, "DIRE_AGENT_PARENT_ONLY=secret") ||
		!containsEnvironment(withoutInheritance, "EXPLICIT=yes") {
		t.Fatalf("isolated environment = %#v", withoutInheritance)
	}
	withInheritance := mergedEnvironment(nil, true)
	if !containsEnvironment(withInheritance, "DIRE_AGENT_PARENT_ONLY=secret") {
		t.Fatalf("inherited environment omitted parent value")
	}
}

func TestSandboxedChildEnvironmentStripsLoaderControls(t *testing.T) {
	t.Setenv("LD_PRELOAD", "./project-owned.so")
	environment := processEnvironment(ProcessSpec{
		Env:        map[string]string{"DYLD_INSERT_LIBRARIES": "./project-owned.dylib", "SAFE": "value"},
		InheritEnv: true, Sandboxed: true,
	})
	for _, entry := range environment {
		if strings.HasPrefix(entry, "LD_") || strings.HasPrefix(entry, "DYLD_") {
			t.Fatalf("sandbox wrapper inherited loader control: %q", entry)
		}
	}
	if !containsEnvironment(environment, "SAFE=value") {
		t.Fatalf("safe environment was removed: %#v", environment)
	}
}

func containsEnvironment(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
