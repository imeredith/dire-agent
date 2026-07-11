package daemon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/imeredith/dire-agent/client"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

func TestProjectIDCommandsAndLegacyThreadAliases(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "input.txt"), []byte("project scoped value"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	server := httptest.NewServer((&daemon.Server{Manager: manager}).Handler())
	defer server.Close()
	websocketURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	api, err := client.Dial(ctx, websocketURL)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	project, err := api.CreateProject(ctx, daemon.CreateProjectOptions{
		Name: "project API", CWD: root, Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(project.ID, "project_") {
		t.Fatalf("CreateProject id = %q, want project_ prefix", project.ID)
	}

	// Send both identifiers with a deliberately invalid legacy value. This
	// proves project_id is accepted and takes precedence over thread_id.
	rawConnection, _, err := websocket.Dial(ctx, websocketURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rawConnection.CloseNow()
	const promptRequestID = "project-id-prompt"
	if err := wsjson.Write(ctx, rawConnection, daemon.Command{
		ID: promptRequestID, Type: "prompt", ProjectID: project.ID,
		ThreadID: "missing_legacy_thread", Message: "read the project file",
	}); err != nil {
		t.Fatal(err)
	}
	if err := waitForSuccessfulResponse(ctx, rawConnection, promptRequestID); err != nil {
		t.Fatal(err)
	}
	settled, err := api.WaitForSettled(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settled.ProjectID != project.ID || settled.ThreadID != project.ID {
		t.Fatalf("settled identifiers = project:%q thread:%q, want %q for both", settled.ProjectID, settled.ThreadID, project.ID)
	}

	projectMessages, err := api.ProjectMessages(ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	legacyMessages, err := api.Messages(ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(projectMessages) == 0 || len(projectMessages) != len(legacyMessages) {
		t.Fatalf("project messages = %d, legacy messages = %d", len(projectMessages), len(legacyMessages))
	}
	if got := projectMessages[len(projectMessages)-1].Content; !strings.Contains(got, "project scoped value") {
		t.Fatalf("last project message = %q, want tool result", got)
	}

	renamed, err := api.SetProjectName(ctx, project.ID, "renamed by project_id")
	if err != nil {
		t.Fatal(err)
	}
	legacyView, err := api.Thread(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "renamed by project_id" || legacyView.Name != renamed.Name {
		t.Fatalf("project/legacy names = %q/%q", renamed.Name, legacyView.Name)
	}
	if _, err := api.SetThreadName(ctx, project.ID, "renamed by thread_id"); err != nil {
		t.Fatal(err)
	}
	projectView, err := api.Project(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projectView.Name != "renamed by thread_id" {
		t.Fatalf("project view name = %q after legacy update", projectView.Name)
	}
	categorized, err := api.SetProjectCategory(ctx, project.ID, "Client A")
	if err != nil {
		t.Fatal(err)
	}
	if categorized.Category != "Client A" {
		t.Fatalf("project category = %q", categorized.Category)
	}
	includedFolder := t.TempDir()
	withFolders, err := api.SetProjectAdditionalFolders(ctx, project.ID, []string{includedFolder})
	if err != nil {
		t.Fatal(err)
	}
	canonicalIncluded, _ := filepath.EvalSymlinks(includedFolder)
	if len(withFolders.AdditionalFolders) != 1 || withFolders.AdditionalFolders[0] != canonicalIncluded {
		t.Fatalf("project additional folders = %q, want %q", withFolders.AdditionalFolders, canonicalIncluded)
	}
	projectState, err := api.ProjectState(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projectState.Project.ID != project.ID || projectState.Thread.ID != project.ID {
		t.Fatalf("project state identifiers = project:%q thread:%q", projectState.Project.ID, projectState.Thread.ID)
	}
	projectEvents, err := api.ProjectEvents(ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	legacyEvents, err := api.HistoryEvents(ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(projectEvents) == 0 || len(projectEvents) != len(legacyEvents) {
		t.Fatalf("project events = %d, legacy events = %d", len(projectEvents), len(legacyEvents))
	}
	if err := api.UnsubscribeProject(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
	if err := api.SubscribeProject(ctx, project.ID); err != nil {
		t.Fatal(err)
	}

	legacyCreated, err := api.CreateThread(ctx, daemon.CreateThreadOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	if projectViewOfLegacy, err := api.Project(ctx, legacyCreated.ID); err != nil || projectViewOfLegacy.ID != legacyCreated.ID {
		t.Fatalf("project view of legacy-created record = %#v, err = %v", projectViewOfLegacy, err)
	}
	projects, err := api.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	threads, err := api.ListThreads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !sameIDs(projects, threads) {
		t.Fatalf("project and legacy listings differ: projects=%#v threads=%#v", projects, threads)
	}

	models, err := api.AvailableModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !hasModel(models, "gpt-5.6", 372_000) {
		t.Fatalf("WebSocket model discovery missing gpt-5.6 context metadata: %#v", models)
	}
	commands, err := api.Commands(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"create_project", "list_projects", "get_project", "get_project_state",
		"get_project_messages", "get_project_events", "subscribe_project",
		"unsubscribe_project", "set_project_name", "set_project_category", "set_project_sandbox_folders", "delete_project",
		"create_thread", "list_threads", "get_thread", "delete_thread",
	} {
		if !containsString(commands, command) {
			t.Errorf("get_commands omitted %q: %#v", command, commands)
		}
	}

	if err := api.DeleteProject(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Thread(ctx, project.ID); err == nil {
		t.Fatal("legacy get_thread found a project deleted through delete_project")
	}
	if err := api.DeleteThread(ctx, legacyCreated.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Project(ctx, legacyCreated.ID); err == nil {
		t.Fatal("get_project found a project deleted through legacy delete_thread")
	}
}

func TestProjectFolderRejectsMissingPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "projects"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	missing := filepath.Join(root, "does-not-exist")
	if _, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: missing}); err == nil {
		t.Fatalf("CreateProject accepted missing folder %q", missing)
	}
}

func waitForSuccessfulResponse(ctx context.Context, connection *websocket.Conn, requestID string) error {
	for {
		var raw json.RawMessage
		if err := wsjson.Read(ctx, connection, &raw); err != nil {
			return fmt.Errorf("read %s response: %w", requestID, err)
		}
		var envelope struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return fmt.Errorf("decode response envelope: %w", err)
		}
		if envelope.Type != "response" || envelope.ID != requestID {
			continue
		}
		if !envelope.Success {
			return fmt.Errorf("command %s failed: %s", requestID, envelope.Error)
		}
		return nil
	}
}

func sameIDs(left, right []threadstore.Project) bool {
	if len(left) != len(right) {
		return false
	}
	leftIDs := make(map[string]bool, len(left))
	for _, item := range left {
		leftIDs[item.ID] = true
	}
	for _, item := range right {
		if !leftIDs[item.ID] {
			return false
		}
	}
	return true
}

func hasModel(models []daemon.ModelInfo, id string, contextWindow int64) bool {
	for _, model := range models {
		if model.ID == id && model.ContextWindow == contextWindow {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
