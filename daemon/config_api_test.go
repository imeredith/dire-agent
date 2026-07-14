package daemon_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/capability"
	"github.com/dire-kiwi/dire-agent/client"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestConfigurationWebSocketLifecycleAndDefaults(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	defaults := configuration.DefaultConfig(root)
	defaults.Global.MCP.Servers["remote"] = configuration.MCPServer{
		Transport: configuration.MCPStreamableHTTP,
		URL:       "https://example.test/mcp",
		Headers: map[string]string{
			"Authorization": "Bearer test-secret",
		},
		SecretHeaders: []string{"Authorization"},
		Approval:      configuration.ApprovalOnRequest,
		Enabled:       false,
	}
	configStore, err := configuration.NewStore(filepath.Join(root, "config.json"), defaults)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := configStore.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry(capability.RegistryConfig{Settings: configStore, Defaults: loaded.Global})
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root,
		DefaultModel: "fallback", Settings: configStore, Capabilities: registry,
		SupportedProviders: []string{"codex", "openrouter"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	server := httptest.NewServer((&daemon.Server{Manager: manager, Config: configStore}).Handler())
	defer server.Close()
	websocketURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	api, err := client.Dial(ctx, websocketURL)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	public, err := api.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := public.Global.MCP.Servers["remote"].Headers["Authorization"]; got != configuration.RedactedValue {
		t.Fatalf("configuration leaked credential: %q", got)
	}
	unsupported := public
	unsupported.Global.Model.Provider = "openroute"
	if err := api.ValidateConfig(ctx, unsupported); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported provider validation error = %v", err)
	}
	invalidOpenRouter := public
	invalidOpenRouter.Global.Model.Provider = "openrouter"
	invalidOpenRouter.Global.Model.ID = "openrouter//auto"
	if err := api.ValidateConfig(ctx, invalidOpenRouter); err == nil || !strings.Contains(err.Error(), "organization-qualified") {
		t.Fatalf("invalid OpenRouter model validation error = %v", err)
	}
	if _, err := api.UpdateConfig(ctx, unsupported); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported provider update error = %v", err)
	}
	stale := public
	public.Global.Model.ID = "configured-model"
	public.Global.Thinking.Level = configuration.ThinkingHigh
	updated, err := api.UpdateConfig(ctx, public)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != public.Revision+1 {
		t.Fatalf("revision = %d", updated.Revision)
	}
	if _, err := api.UpdateConfig(ctx, stale); err == nil {
		t.Fatal("stale configuration update succeeded")
	}

	project, err := api.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	if project.Model != "configured-model" || project.ThinkingLevel != "high" {
		t.Fatalf("project defaults = %+v", project)
	}
	effective, err := api.EffectiveConfig(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if effective.ProjectOverride || effective.Settings.Model.ID != "configured-model" {
		t.Fatalf("effective = %+v", effective)
	}
	sandbox, err := api.ProjectSandbox(ctx, project.ID)
	if err != nil || sandbox.Global != configuration.SandboxStrict || sandbox.Effective != configuration.SandboxStrict || sandbox.Override != nil {
		t.Fatalf("initial sandbox = %+v err=%v", sandbox, err)
	}
	off := configuration.SandboxOff
	sandbox, err = api.SetProjectSandbox(ctx, project.ID, &off)
	if err != nil || sandbox.Effective != configuration.SandboxOff || sandbox.Override == nil || *sandbox.Override != configuration.SandboxOff {
		t.Fatalf("disabled sandbox = %+v err=%v", sandbox, err)
	}
	effective, err = api.EffectiveConfig(ctx, project.ID)
	if err != nil || effective.Settings.Tools.Sandbox != configuration.SandboxOff {
		t.Fatalf("effective sandbox = %+v err=%v", effective.Settings.Tools, err)
	}
	if _, err := api.SetProjectSandbox(ctx, project.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Capabilities(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
}
