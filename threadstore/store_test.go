package threadstore_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/imeredith/dire-agent/threadstore"
)

func TestOneSQLiteFilePerThreadPersistsData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	directory := t.TempDir()
	store, err := threadstore.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Create(ctx, threadstore.Thread{
		ID: "thread_test", Model: "model-a", CWD: directory,
		ParentID: "parent", RootID: "root", AgentName: "reviewer", AgentRole: "review", Depth: 1,
		ThinkingLevel: "medium", SteeringMode: "one-at-a-time", FollowUpMode: "one-at-a-time",
		Tools: []string{"read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AppendMessage(ctx, threadstore.Message{Kind: "message", Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AppendEvent(ctx, threadstore.Event{Type: "agent_start", Data: json.RawMessage(`{"ok":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveState(ctx, threadstore.State{Provider: "fake", SessionID: "session-1", Data: json.RawMessage(`[{"x":1}]`)}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(directory, "thread_test.db")); err != nil {
		t.Fatalf("thread SQLite file: %v", err)
	}
	reopened, err := store.Open(ctx, "thread_test")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	messages, err := reopened.Messages(ctx, 0, 10)
	if err != nil || len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("messages = %#v, err = %v", messages, err)
	}
	events, err := reopened.Events(ctx, 0, 10)
	if err != nil || len(events) != 1 || events[0].Type != "agent_start" {
		t.Fatalf("events = %#v, err = %v", events, err)
	}
	state, err := reopened.LoadState(ctx)
	if err != nil || state.SessionID != "session-1" {
		t.Fatalf("state = %#v, err = %v", state, err)
	}
	threads, err := store.List(ctx)
	if err != nil || len(threads) != 1 || threads[0].ID != "thread_test" {
		t.Fatalf("threads = %#v, err = %v", threads, err)
	}
	if threads[0].ParentID != "parent" || threads[0].RootID != "root" || threads[0].AgentName != "reviewer" || threads[0].Depth != 1 {
		t.Fatalf("subagent metadata = %#v", threads[0])
	}
}
