package daemon_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

func TestProjectCategoryPersistsAndChatsRejectIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root, DefaultModel: "fake-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Category: "  Client A  "})
	if err != nil {
		t.Fatal(err)
	}
	if project.Category != "Client A" {
		t.Fatalf("created category = %q", project.Category)
	}
	next := "Client B"
	project, err = manager.UpdateSettings(ctx, project.ID, daemon.SettingsUpdate{Category: &next})
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	persisted, err := database.Thread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Category != "Client B" {
		t.Fatalf("persisted category = %q", persisted.Category)
	}

	chat, err := manager.CreateChat(ctx, daemon.CreateChatOptions{Name: "pathless"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateSettings(ctx, chat.ID, daemon.SettingsUpdate{Category: &next}); err == nil {
		t.Fatal("standalone chat accepted a project category")
	}
	tooLong := strings.Repeat("x", 81)
	if _, err := manager.UpdateSettings(ctx, project.ID, daemon.SettingsUpdate{Category: &tooLong}); err == nil {
		t.Fatal("project accepted an overlong category")
	}
}
