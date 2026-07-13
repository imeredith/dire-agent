package daemon_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/provider/openrouter"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestCreateRequiresRestartAfterProviderSettingChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	defaults := configuration.DefaultConfig(root)
	defaults.Global.Model.Provider = "openrouter"
	defaults.Global.Model.ID = "openrouter/auto"
	configStore, err := configuration.NewStore(filepath.Join(root, "config.json"), defaults)
	if err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultProvider: "codex",
		DefaultModel: "gpt-5.6", DefaultCWD: root, Settings: configStore,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, err = manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root})
	if err == nil || !strings.Contains(err.Error(), "restart the daemon") {
		t.Fatalf("CreateProject error = %v", err)
	}
}

func TestExplicitProviderAndModelOverrideConfigurationDefaults(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	defaults := configuration.DefaultConfig(root)
	configStore, err := configuration.NewStore(filepath.Join(root, "config.json"), defaults)
	if err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultProvider: "openrouter",
		DefaultModel: "openrouter/auto", DefaultCWD: root, Settings: configStore,
		OverrideModelDefaults: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	if project.Model != "openrouter/auto" {
		t.Fatalf("project model = %q", project.Model)
	}
}

func TestOpenRouterChatFallsBackFromLegacyStandaloneModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	defaults := configuration.DefaultConfig(root)
	defaults.Global.Model.Provider = "openrouter"
	defaults.Global.Model.ID = "openrouter/auto"
	// This value exists in configuration documents created before OpenRouter
	// support. It must not be sent to OpenRouter without an organization prefix.
	defaults.Global.StandaloneChat.Model = "gpt-5.6"
	configStore, err := configuration.NewStore(filepath.Join(root, "config.json"), defaults)
	if err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "chats"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultProvider: "openrouter",
		DefaultModel: "openrouter/auto", DefaultCWD: root, Settings: configStore,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	chat, err := manager.CreateChat(ctx, daemon.CreateChatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if chat.Model != "openrouter/auto" {
		t.Fatalf("chat model = %q", chat.Model)
	}
}

func TestProviderStateMismatchIsReportedBeforeModelValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Create(ctx, threadstore.Thread{
		ID: "project_from_codex", Model: "gpt-5.6", CWD: root, Status: "idle",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveState(ctx, threadstore.State{
		Provider: "codex-subscription-direct", SessionID: "old-session", Data: json.RawMessage(`[]`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	provider, err := openrouter.New(ctx, openrouter.Config{APIKey: "test-key", DefaultModel: "openrouter/auto"})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: provider, DefaultProvider: "openrouter",
		DefaultModel: "openrouter/auto", DefaultCWD: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, err = manager.Thread(ctx, "project_from_codex")
	if err == nil || !strings.Contains(err.Error(), "cannot restore provider state") {
		t.Fatalf("Thread error = %v", err)
	}
}
