package daemon_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"net/url"
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

func TestTerminalWebSocketStartsInProjectAndRejectsChats(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
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
	server := httptest.NewServer((&daemon.Server{Manager: manager}).Handler())
	defer server.Close()
	base := "ws" + strings.TrimPrefix(server.URL, "http")
	terminalURL := base + "/terminal?project_id=" + url.QueryEscape(project.ID) + "&mode=shell&cols=90&rows=24"
	connection, _, err := websocket.Dial(ctx, terminalURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	if err := wsjson.Write(ctx, connection, map[string]any{
		"type": "input", "data": "printf 'DIRE_AGENT_CWD=%s\\nDIRE_AGENT_COLOR=%s|%s|%s\\n' \"$PWD\" \"$TERM\" \"$COLORTERM\" \"${NO_COLOR-unset}\"; exit\n",
	}); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	for !strings.Contains(output.String(), "DIRE_AGENT_CWD="+project.CWD) {
		var message struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}
		if err := wsjson.Read(ctx, connection, &message); err != nil {
			t.Fatalf("read terminal output: %v; output=%q", err, output.String())
		}
		if message.Type == "output" {
			decoded, err := base64.StdEncoding.DecodeString(message.Data)
			if err != nil {
				t.Fatal(err)
			}
			output.Write(decoded)
		}
	}
	if !strings.Contains(output.String(), "DIRE_AGENT_COLOR=xterm-256color|truecolor|unset") {
		t.Fatalf("terminal did not advertise true color or inherited NO_COLOR: output=%q", output.String())
	}

	chatURL := base + "/terminal?project_id=" + url.QueryEscape(chat.ID) + "&mode=shell"
	chatConnection, response, err := websocket.Dial(ctx, chatURL, nil)
	if chatConnection != nil {
		chatConnection.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != 404 {
		body, _ := json.Marshal(response)
		t.Fatalf("chat terminal handshake = err:%v response:%s", err, body)
	}
}

func TestTerminalWebSocketUsesConfiguredLauncherIDAndRejectsDesktopKind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	defaults := configuration.DefaultConfig(root)
	defaults.Global.Launchers = []configuration.ProjectLauncher{
		{ID: "where", Label: "Working directory", Kind: configuration.LauncherTerminal, Command: "pwd"},
		{ID: "desktop", Label: "Desktop", Kind: configuration.LauncherDesktop, Command: "true"},
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
	server := httptest.NewServer((&daemon.Server{Manager: manager, Config: configStore}).Handler())
	defer server.Close()
	base := "ws" + strings.TrimPrefix(server.URL, "http")
	terminalURL := base + "/terminal?project_id=" + url.QueryEscape(project.ID) + "&launcher_id=where"
	connection, _, err := websocket.Dial(ctx, terminalURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	var output strings.Builder
	var readyMode string
	for !strings.Contains(output.String(), project.CWD) {
		var message struct {
			Type string `json:"type"`
			Data string `json:"data"`
			Mode string `json:"mode"`
		}
		if err := wsjson.Read(ctx, connection, &message); err != nil {
			t.Fatalf("read terminal output: %v; output=%q", err, output.String())
		}
		if message.Type == "ready" {
			readyMode = message.Mode
		}
		if message.Type == "output" {
			decoded, err := base64.StdEncoding.DecodeString(message.Data)
			if err != nil {
				t.Fatal(err)
			}
			output.Write(decoded)
		}
	}
	if readyMode != "where" {
		t.Fatalf("ready mode = %q, want launcher id", readyMode)
	}

	desktopURL := base + "/terminal?project_id=" + url.QueryEscape(project.ID) + "&launcher_id=desktop"
	desktopConnection, response, err := websocket.Dial(ctx, desktopURL, nil)
	if desktopConnection != nil {
		desktopConnection.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != 400 {
		body, _ := json.Marshal(response)
		t.Fatalf("desktop terminal handshake = err:%v response:%s", err, body)
	}
}
