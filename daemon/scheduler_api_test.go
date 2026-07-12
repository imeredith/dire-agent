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

	"github.com/dire-kiwi/dire-agent/client"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/schedulestore"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestScheduledPromptWebSocketGoClientCRUDRunAndEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "input.txt"), []byte("scheduled API value"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root, DefaultModel: "fake-model", DisableScheduler: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeScheduleTestManager(t, manager) })
	server := httptest.NewServer((&daemon.Server{Manager: manager}).Handler())
	defer server.Close()
	api, err := client.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	project, err := api.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := api.SubscribeScheduledPrompts(ctx); err != nil {
		t.Fatal(err)
	}
	runAt := time.Now().UTC().Add(time.Hour)
	created, err := api.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
		Name: "API schedule", Prompt: "run through the Go client", ConversationID: project.ID,
		ScheduleType: schedulestore.ScheduleOnce, RunAt: &runAt, Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.TargetType != schedulestore.TargetProject || created.ConversationID != project.ID {
		t.Fatalf("server-inferred schedule target = %#v", created)
	}
	createdEvent := waitForWireScheduleEvent(t, ctx, api, created.ID, "scheduled_prompt_created")
	if createdEvent.Scope.Kind != "schedule" || createdEvent.Scope.ID != created.ID {
		t.Fatalf("created event scope = %#v", createdEvent.Scope)
	}

	projectSchedules, err := api.ListScheduledPrompts(ctx, project.ID)
	if err != nil || len(projectSchedules) != 1 || projectSchedules[0].ID != created.ID {
		t.Fatalf("project schedules over WebSocket = %#v, err = %v", projectSchedules, err)
	}
	allSchedules, err := api.ListScheduledPrompts(ctx, "")
	if err != nil || len(allSchedules) != 1 {
		t.Fatalf("all schedules over WebSocket = %#v, err = %v", allSchedules, err)
	}

	disabled := false
	updated, err := api.UpdateScheduledPrompt(ctx, created.ID, daemon.ScheduledPromptInput{Name: "Updated API schedule", Enabled: &disabled})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.NextRunAt != nil || updated.Name != "Updated API schedule" {
		t.Fatalf("updated schedule = %#v", updated)
	}
	waitForWireScheduleEvent(t, ctx, api, created.ID, "scheduled_prompt_updated")

	runResult, err := api.RunScheduledPrompt(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if runResult.LastConversationID != project.ID {
		t.Fatalf("run-now target = %q, want %q", runResult.LastConversationID, project.ID)
	}
	waitForWireScheduleEventTypes(t, ctx, api, created.ID, "scheduled_prompt_triggered", "scheduled_prompt_completed")
	completed, err := api.ListScheduledPrompts(ctx, project.ID)
	if err != nil || len(completed) != 1 || completed[0].Pending || completed[0].LastStatus != "completed" {
		t.Fatalf("completed schedule over WebSocket = %#v, err = %v", completed, err)
	}
	if got := countUserPrompt(t, manager, project.ID, "run through the Go client"); got != 1 {
		t.Fatalf("scheduled API prompt count = %d, want 1", got)
	}

	commands, err := api.Commands(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"list_scheduled_prompts", "create_scheduled_prompt", "update_scheduled_prompt",
		"delete_scheduled_prompt", "run_scheduled_prompt", "subscribe_scheduled_prompts", "unsubscribe_scheduled_prompts",
	} {
		if !containsString(commands, command) {
			t.Errorf("get_commands omitted %q: %#v", command, commands)
		}
	}

	if err := api.DeleteScheduledPrompt(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	waitForWireScheduleEvent(t, ctx, api, created.ID, "scheduled_prompt_deleted")
	remaining, err := api.ListScheduledPrompts(ctx, "")
	if err != nil || len(remaining) != 0 {
		t.Fatalf("schedules after WebSocket delete = %#v, err = %v", remaining, err)
	}

	oneOff, err := api.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
		Name: "API one off", Prompt: "create a discoverable fresh chat", TargetType: schedulestore.TargetOneOff,
		ScheduleType: schedulestore.ScheduleCron, Cron: "0 0 1 1 *", Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForWireScheduleEvent(t, ctx, api, oneOff.ID, "scheduled_prompt_created")
	if _, err := api.RunScheduledPrompt(ctx, oneOff.ID); err != nil {
		t.Fatal(err)
	}
	completedEvent := waitForWireScheduleEvent(t, ctx, api, oneOff.ID, "scheduled_prompt_completed")
	var completedOneOff daemon.ScheduledPrompt
	if err := json.Unmarshal(completedEvent.Data, &completedOneOff); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(completedOneOff.LastConversationID, "chat_") {
		t.Fatalf("one-off completion event conversation = %q", completedOneOff.LastConversationID)
	}
	createdChat, err := api.Chat(ctx, completedOneOff.LastConversationID)
	if err != nil || createdChat.ID != completedOneOff.LastConversationID {
		t.Fatalf("fresh chat from schedule event = %#v, err = %v", createdChat, err)
	}
	if err := api.DeleteScheduledPrompt(ctx, oneOff.ID); err != nil {
		t.Fatal(err)
	}
	waitForWireScheduleEvent(t, ctx, api, oneOff.ID, "scheduled_prompt_deleted")
	if err := api.UnsubscribeScheduledPrompts(ctx); err != nil {
		t.Fatal(err)
	}
}

func waitForWireScheduleEvent(t *testing.T, ctx context.Context, api *client.Client, scheduleID, eventType string) daemon.WireEvent {
	t.Helper()
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s for %s", eventType, scheduleID)
		case event := <-api.Events():
			if event.Type != eventType || event.Scope.Kind != "schedule" || event.Scope.ID != scheduleID {
				continue
			}
			var schedule daemon.ScheduledPrompt
			if err := json.Unmarshal(event.Data, &schedule); err != nil {
				t.Fatalf("decode %s event: %v", eventType, err)
			}
			if schedule.ID != scheduleID {
				t.Fatalf("%s event schedule = %q, want %q", eventType, schedule.ID, scheduleID)
			}
			return event
		}
	}
}

func waitForWireScheduleEventTypes(t *testing.T, ctx context.Context, api *client.Client, scheduleID string, eventTypes ...string) {
	t.Helper()
	wanted := make(map[string]bool, len(eventTypes))
	for _, eventType := range eventTypes {
		wanted[eventType] = true
	}
	for len(wanted) > 0 {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for schedule events %v for %s", wanted, scheduleID)
		case event := <-api.Events():
			if event.Scope.Kind == "schedule" && event.Scope.ID == scheduleID && wanted[event.Type] {
				delete(wanted, event.Type)
			}
		}
	}
}
