package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestStoreCreates0600FileAndPersistsUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	store, err := NewStore(path, DefaultConfig(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %#o, want 0600", got)
	}

	loaded.Global.Model.ID = "gpt-5.6-sol"
	updated, err := store.Update(context.Background(), loaded.Revision, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != loaded.Revision+1 {
		t.Fatalf("revision = %d", updated.Revision)
	}
	reloaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Global.Model.ID != "gpt-5.6-sol" || reloaded.Revision != updated.Revision {
		t.Fatalf("update was not persisted: %+v", reloaded)
	}
}

func TestStoreRedactsSecretsAndPreservesPlaceholders(t *testing.T) {
	defaults := DefaultConfig(t.TempDir())
	defaults.Global.MCP.Servers["secrets"] = MCPServer{
		Transport: MCPStreamableHTTP,
		URL:       "https://example.test/mcp",
		Env: map[string]string{
			"PUBLIC_NAME": "visible",
			"OPAQUE":      "env-secret-value",
			"API_TOKEN":   "automatic-secret-value",
		},
		SecretEnv: []string{"OPAQUE"},
		Headers: map[string]string{
			"Authorization": "Bearer header-secret-value",
			"X-Visible":     "visible-header",
		},
		Approval: ApprovalOnRequest,
		Enabled:  true,
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertSecretsRedacted(t, loaded.Global.MCP.Servers["secrets"])

	loaded.Global.Thinking.Level = ThinkingHigh
	updated, err := store.Update(context.Background(), loaded.Revision, loaded)
	if err != nil {
		t.Fatal(err)
	}
	assertSecretsRedacted(t, updated.Global.MCP.Servers["secrets"])
	effective, found, err := store.Effective(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("unknown project reported as found")
	}
	assertSecretsRedacted(t, effective.MCP.Servers["secrets"])
	runtime, found, err := store.RuntimeSettings(context.Background(), "missing")
	if err != nil || found {
		t.Fatalf("RuntimeSettings: found=%v err=%v", found, err)
	}
	if got := runtime.MCP.Servers["secrets"].Headers["Authorization"]; got != "Bearer header-secret-value" {
		t.Fatalf("runtime credential = %q", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"env-secret-value", "automatic-secret-value", "header-secret-value"} {
		if !strings.Contains(string(data), value) {
			t.Fatalf("stored secret %q was not preserved", value)
		}
	}
	publicJSON, err := json.Marshal(updated)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"env-secret-value", "automatic-secret-value", "header-secret-value"} {
		if strings.Contains(string(publicJSON), value) {
			t.Fatalf("public view leaked %q", value)
		}
	}
}

func TestStoreRedactsSecretsInProjectPatches(t *testing.T) {
	home := t.TempDir()
	defaults := DefaultConfig(home)
	server := validStdioServer()
	server.Env = map[string]string{"PRIVATE": "project-secret-value"}
	server.SecretEnv = []string{"PRIVATE"}
	defaults.Projects["project"] = ProjectOverride{
		Folder: filepath.Join(home, "project"),
		Settings: SettingsPatch{MCP: &MCPPatch{Servers: map[string]MCPServerPatch{
			"local": patchForServer(server),
		}}},
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	patch := loaded.Projects["project"].Settings.MCP.Servers["local"]
	if patch.Env == nil || (*patch.Env)["PRIVATE"] != RedactedValue {
		t.Fatalf("patch leaked secret: %+v", patch)
	}

	loaded.Global.Thinking.Level = ThinkingLow
	if _, err := store.Update(context.Background(), loaded.Revision, loaded); err != nil {
		t.Fatal(err)
	}
	effective, found, err := store.Effective(context.Background(), "project")
	if err != nil || !found {
		t.Fatalf("Effective: found=%v err=%v", found, err)
	}
	if effective.MCP.Servers["local"].Env["PRIVATE"] != RedactedValue {
		t.Fatal("effective settings leaked project secret")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "project-secret-value") {
		t.Fatal("project secret placeholder was not restored")
	}
}

func TestStoreSetProjectSandboxOverridesAndInherits(t *testing.T) {
	home := t.TempDir()
	store, err := NewStore(filepath.Join(home, "settings.json"), DefaultConfig(home))
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(home, "project")
	mode := SandboxOff
	updated, err := store.SetProjectSandbox(context.Background(), "project", folder, &mode)
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.Projects["project"].Settings.Tools.Sandbox; got == nil || *got != SandboxOff {
		t.Fatalf("project sandbox = %v, want off", got)
	}
	effective, found, err := store.RuntimeSettings(context.Background(), "project")
	if err != nil || !found || effective.Tools.Sandbox != SandboxOff {
		t.Fatalf("effective sandbox = %q found=%v err=%v", effective.Tools.Sandbox, found, err)
	}

	updated, err = store.SetProjectSandbox(context.Background(), "project", folder, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := updated.Projects["project"]; exists {
		t.Fatal("inheritance-only project override was retained")
	}
	effective, found, err = store.RuntimeSettings(context.Background(), "project")
	if err != nil || found || effective.Tools.Sandbox != SandboxStrict {
		t.Fatalf("inherited sandbox = %q found=%v err=%v", effective.Tools.Sandbox, found, err)
	}
}

func assertSecretsRedacted(t *testing.T, server MCPServer) {
	t.Helper()
	if server.Env["OPAQUE"] != RedactedValue || server.Env["API_TOKEN"] != RedactedValue {
		t.Fatalf("environment was not redacted: %v", server.Env)
	}
	if server.Headers["Authorization"] != RedactedValue {
		t.Fatalf("headers were not redacted: %v", server.Headers)
	}
	if server.Env["PUBLIC_NAME"] != "visible" || server.Headers["X-Visible"] != "visible-header" {
		t.Fatal("non-secret values were redacted")
	}
}

func TestStoreRejectsStaleRevision(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "settings.json"), DefaultConfig(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(context.Background(), loaded.Revision, loaded); err != nil {
		t.Fatal(err)
	}
	_, err = store.Update(context.Background(), loaded.Revision, loaded)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("error = %v", err)
	}
	var conflict *RevisionConflictError
	if !errors.As(err, &conflict) || conflict.Actual != loaded.Revision+1 {
		t.Fatalf("conflict = %#v", conflict)
	}
}

func TestConcurrentStoresAllowOneOptimisticWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	defaults := DefaultConfig(t.TempDir())
	first, err := NewStore(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewStore(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := first.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	const writers = 24
	var successes atomic.Int32
	var conflicts atomic.Int32
	var unexpected atomic.Value
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			candidate, cloneErr := cloneConfig(loaded)
			if cloneErr != nil {
				unexpected.Store(cloneErr)
				return
			}
			candidate.Global.StandaloneChat.Instructions = string(rune('a' + index%26))
			target := first
			if index%2 == 1 {
				target = second
			}
			_, updateErr := target.Update(context.Background(), loaded.Revision, candidate)
			switch {
			case updateErr == nil:
				successes.Add(1)
			case errors.Is(updateErr, ErrRevisionConflict):
				conflicts.Add(1)
			default:
				unexpected.Store(updateErr)
			}
		}(index)
	}
	close(start)
	wait.Wait()
	if value := unexpected.Load(); value != nil {
		t.Fatalf("unexpected error: %v", value)
	}
	if successes.Load() != 1 || conflicts.Load() != writers-1 {
		t.Fatalf("successes=%d conflicts=%d", successes.Load(), conflicts.Load())
	}
}

func TestStoreHonorsCancelledContext(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "settings.json"), DefaultConfig(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Load(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}
