package daemon_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imeredith/dire-agent/client"
	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

func TestStandaloneChatLifecycleAndIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
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
	server := httptest.NewServer((&daemon.Server{Manager: manager}).Handler())
	defer server.Close()
	api, err := client.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	chat, err := api.CreateChat(ctx, daemon.CreateChatOptions{Name: "general"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(chat.ID, "chat_") || chat.ResourceKind() != threadstore.KindChat {
		t.Fatalf("chat metadata = %#v", chat)
	}
	if chat.CWD != "" || len(chat.Tools) != 0 {
		t.Fatalf("standalone chat unexpectedly has project capabilities: cwd=%q tools=%v", chat.CWD, chat.Tools)
	}
	state, err := api.ChatState(ctx, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Kind != threadstore.KindChat || state.Chat.ID != chat.ID || state.Conversation.ID != chat.ID || state.Project.ID != "" {
		t.Fatalf("chat state = %#v", state)
	}
	if _, err := api.SetTools(ctx, chat.ID, []string{"read"}); err == nil {
		t.Fatal("standalone chat accepted a project file tool")
	}
	if _, err := api.Project(ctx, chat.ID); err == nil {
		t.Fatal("get_project accepted a standalone chat")
	}

	if err := api.ChatPrompt(ctx, chat.ID, "hello without a folder", ""); err != nil {
		t.Fatal(err)
	}
	settled, err := api.WaitForSettled(ctx, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settled.ChatID != chat.ID || settled.ConversationID != chat.ID || settled.ProjectID != "" {
		t.Fatalf("chat event identifiers = %#v", settled)
	}
	if settled.Scope.Kind != threadstore.KindChat || settled.Scope.ID != chat.ID {
		t.Fatalf("chat event scope = %#v", settled.Scope)
	}
	messages, err := api.ChatMessages(ctx, chat.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) < 2 || messages[0].Role != "user" || messages[len(messages)-1].Role != "assistant" {
		t.Fatalf("chat messages = %#v", messages)
	}

	chats, err := api.ListChats(ctx)
	if err != nil || len(chats) != 1 || chats[0].ID != chat.ID {
		t.Fatalf("ListChats = %#v, %v", chats, err)
	}
	projects, err := api.ListProjects(ctx)
	if err != nil || len(projects) != 0 {
		t.Fatalf("ListProjects included chat: %#v, %v", projects, err)
	}
	conversations, err := api.ListConversations(ctx)
	if err != nil || len(conversations) != 1 || conversations[0].ID != chat.ID {
		t.Fatalf("ListConversations = %#v, %v", conversations, err)
	}

	if err := api.DeleteChat(ctx, chat.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Chat(ctx, chat.ID); err == nil {
		t.Fatal("deleted chat remained accessible")
	}
}
