package daemon_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/imeredith/dire-agent/configuration"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

type launcherCommandResponse struct {
	ID      string          `json:"id"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

func TestProjectLauncherCommandsListAndLaunchConfiguredDesktopApplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	defaults := configuration.DefaultConfig(root)
	defaults.Global.Launchers = []configuration.ProjectLauncher{
		{ID: "terminal", Label: "Terminal", Kind: configuration.LauncherTerminal},
		{ID: "marker", Label: "Marker app", Kind: configuration.LauncherDesktop, Command: "touch", Args: []string{"desktop-launched"}},
	}
	configStore, err := configuration.NewStore(filepath.Join(root, "config.json"), defaults)
	if err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{Store: store, Provider: &fakeProvider{}, DefaultCWD: root})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := manager.CreateChat(ctx, daemon.CreateChatOptions{Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&daemon.Server{Manager: manager, Config: configStore}).Handler())
	defer server.Close()
	connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()

	if err := wsjson.Write(ctx, connection, daemon.Command{ID: "list", Type: "get_project_launchers", ProjectID: project.ID}); err != nil {
		t.Fatal(err)
	}
	response := readLauncherCommandResponse(t, ctx, connection, "list")
	if !response.Success {
		t.Fatalf("list launchers: %s", response.Error)
	}
	var launchers []configuration.ProjectLauncher
	if err := json.Unmarshal(response.Data, &launchers); err != nil {
		t.Fatal(err)
	}
	if len(launchers) != 2 || launchers[1].ID != "marker" || launchers[1].Command != "touch" {
		t.Fatalf("launchers = %#v", launchers)
	}

	if err := wsjson.Write(ctx, connection, daemon.Command{
		ID: "launch", Type: "launch_project_app", ProjectID: project.ID, LauncherID: "marker",
	}); err != nil {
		t.Fatal(err)
	}
	response = readLauncherCommandResponse(t, ctx, connection, "launch")
	if !response.Success {
		t.Fatalf("launch desktop app: %s", response.Error)
	}
	var result struct {
		Launched bool   `json:"launched"`
		ID       string `json:"id"`
		Label    string `json:"label"`
	}
	if err := json.Unmarshal(response.Data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Launched || result.ID != "marker" || result.Label != "Marker app" {
		t.Fatalf("launch result = %#v", result)
	}
	marker := filepath.Join(root, "desktop-launched")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("desktop launcher did not run in project folder: %s", marker)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := wsjson.Write(ctx, connection, daemon.Command{
		ID: "wrong-kind", Type: "launch_project_app", ProjectID: project.ID, LauncherID: "terminal",
	}); err != nil {
		t.Fatal(err)
	}
	response = readLauncherCommandResponse(t, ctx, connection, "wrong-kind")
	if response.Success || !strings.Contains(response.Error, "not a desktop application") {
		t.Fatalf("terminal launcher as desktop = success:%v error:%q", response.Success, response.Error)
	}

	if err := wsjson.Write(ctx, connection, daemon.Command{ID: "chat", Type: "get_project_launchers", ProjectID: chat.ID}); err != nil {
		t.Fatal(err)
	}
	response = readLauncherCommandResponse(t, ctx, connection, "chat")
	if response.Success || !strings.Contains(response.Error, "top-level project") {
		t.Fatalf("chat launcher list = success:%v error:%q", response.Success, response.Error)
	}
}

func readLauncherCommandResponse(t *testing.T, ctx context.Context, connection *websocket.Conn, id string) launcherCommandResponse {
	t.Helper()
	for {
		var response launcherCommandResponse
		if err := wsjson.Read(ctx, connection, &response); err != nil {
			t.Fatalf("read %s response: %v", id, err)
		}
		if response.ID == id {
			return response
		}
	}
}
