package daemon_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imeredith/dire-agent/capability"
	"github.com/imeredith/dire-agent/client"
	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
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
	if _, err := api.Capabilities(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
}
