package daemon_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/schedulestore"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestScheduledPromptManagerCRUDAndValidation(t *testing.T) {
	root := t.TempDir()
	manager := openScheduleTestManager(t, root, true)
	t.Cleanup(func() { closeScheduleTestManager(t, manager) })
	ctx := context.Background()

	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := manager.CreateChat(ctx, daemon.CreateChatOptions{Name: "validation chat"})
	if err != nil {
		t.Fatal(err)
	}

	created, err := manager.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
		Name: "  Daily review  ", Prompt: "  Review the repository  ",
		TargetType: schedulestore.TargetProject, ConversationID: project.ID,
		ScheduleType: schedulestore.ScheduleCron, Cron: "0 9 * * mon-fri", Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Daily review" || created.Prompt != "Review the repository" {
		t.Fatalf("normalized schedule = %#v", created)
	}
	if !created.Enabled || created.NextRunAt == nil || created.TargetType != schedulestore.TargetProject {
		t.Fatalf("created schedule timing/target = %#v", created)
	}
	filtered, err := manager.ListScheduledPrompts(ctx, project.ID)
	if err != nil || len(filtered) != 1 || filtered[0].ID != created.ID {
		t.Fatalf("project schedules = %#v, err = %v", filtered, err)
	}
	if chatSchedules, err := manager.ListScheduledPrompts(ctx, chat.ID); err != nil || len(chatSchedules) != 0 {
		t.Fatalf("chat schedules before creation = %#v, err = %v", chatSchedules, err)
	}

	disabled := false
	updated, err := manager.UpdateScheduledPrompt(ctx, created.ID, daemon.ScheduledPromptInput{
		Name: "Updated review", Enabled: &disabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Updated review" || updated.Enabled || updated.NextRunAt != nil || updated.Prompt != created.Prompt {
		t.Fatalf("disabled patch update = %#v", updated)
	}
	enabled := true
	updated, err = manager.UpdateScheduledPrompt(ctx, created.ID, daemon.ScheduledPromptInput{Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled || updated.NextRunAt == nil {
		t.Fatalf("re-enabled schedule = %#v", updated)
	}

	future := time.Now().UTC().Add(time.Hour)
	past := time.Now().UTC().Add(-time.Minute)
	invalid := []struct {
		name  string
		input daemon.ScheduledPromptInput
		want  string
	}{
		{
			name: "missing name",
			input: daemon.ScheduledPromptInput{Prompt: "work", TargetType: schedulestore.TargetOneOff,
				ScheduleType: schedulestore.ScheduleCron, Cron: "0 * * * *", Timezone: "UTC"},
			want: "name is required",
		},
		{
			name: "missing prompt",
			input: daemon.ScheduledPromptInput{Name: "empty", TargetType: schedulestore.TargetOneOff,
				ScheduleType: schedulestore.ScheduleCron, Cron: "0 * * * *", Timezone: "UTC"},
			want: "prompt is empty",
		},
		{
			name: "one off with conversation",
			input: daemon.ScheduledPromptInput{Name: "bad", Prompt: "work", TargetType: schedulestore.TargetOneOff,
				ConversationID: project.ID, ScheduleType: schedulestore.ScheduleCron, Cron: "0 * * * *", Timezone: "UTC"},
			want: "must not specify a conversation",
		},
		{
			name: "target kind mismatch",
			input: daemon.ScheduledPromptInput{Name: "bad", Prompt: "work", TargetType: schedulestore.TargetChat,
				ConversationID: project.ID, ScheduleType: schedulestore.ScheduleCron, Cron: "0 * * * *", Timezone: "UTC"},
			want: "not a chat",
		},
		{
			name: "missing target conversation",
			input: daemon.ScheduledPromptInput{Name: "bad", Prompt: "work", TargetType: schedulestore.TargetProject,
				ScheduleType: schedulestore.ScheduleCron, Cron: "0 * * * *", Timezone: "UTC"},
			want: "conversation is required",
		},
		{
			name: "invalid cron",
			input: daemon.ScheduledPromptInput{Name: "bad", Prompt: "work", TargetType: schedulestore.TargetOneOff,
				ScheduleType: schedulestore.ScheduleCron, Cron: "61 * * * *", Timezone: "UTC"},
			want: "invalid cron minute",
		},
		{
			name: "invalid timezone",
			input: daemon.ScheduledPromptInput{Name: "bad", Prompt: "work", TargetType: schedulestore.TargetOneOff,
				ScheduleType: schedulestore.ScheduleCron, Cron: "0 * * * *", Timezone: "Mars/Olympus_Mons"},
			want: "invalid schedule timezone",
		},
		{
			name: "once missing run at",
			input: daemon.ScheduledPromptInput{Name: "bad", Prompt: "work", TargetType: schedulestore.TargetOneOff,
				ScheduleType: schedulestore.ScheduleOnce, Timezone: "UTC"},
			want: "run_at is required",
		},
		{
			name: "once in past",
			input: daemon.ScheduledPromptInput{Name: "bad", Prompt: "work", TargetType: schedulestore.TargetOneOff,
				ScheduleType: schedulestore.ScheduleOnce, RunAt: &past, Timezone: "UTC"},
			want: "must run in the future",
		},
		{
			name: "unsupported schedule type",
			input: daemon.ScheduledPromptInput{Name: "bad", Prompt: "work", TargetType: schedulestore.TargetOneOff,
				ScheduleType: "interval", RunAt: &future, Timezone: "UTC"},
			want: "schedule_type",
		},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			_, err := manager.CreateScheduledPrompt(ctx, test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CreateScheduledPrompt() error = %v, want substring %q", err, test.want)
			}
		})
	}

	if err := manager.DeleteScheduledPrompt(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := manager.ListScheduledPrompts(ctx, "")
	if err != nil || len(remaining) != 0 {
		t.Fatalf("schedules after delete = %#v, err = %v", remaining, err)
	}
}

func TestRunScheduledPromptNowTargetsProjectChatAndFreshOneOffChats(t *testing.T) {
	tests := []struct {
		name       string
		targetType string
	}{
		{name: "project", targetType: schedulestore.TargetProject},
		{name: "chat", targetType: schedulestore.TargetChat},
		{name: "one off", targetType: schedulestore.TargetOneOff},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			manager := openScheduleTestManager(t, root, true)
			t.Cleanup(func() { closeScheduleTestManager(t, manager) })
			ctx := context.Background()

			conversationID := ""
			switch test.targetType {
			case schedulestore.TargetProject:
				project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
				if err != nil {
					t.Fatal(err)
				}
				conversationID = project.ID
			case schedulestore.TargetChat:
				chat, err := manager.CreateChat(ctx, daemon.CreateChatOptions{Name: "scheduled chat"})
				if err != nil {
					t.Fatal(err)
				}
				conversationID = chat.ID
			}
			prompt := "scheduled prompt for " + test.targetType
			created, err := manager.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
				Name: "Run now", Prompt: prompt, TargetType: test.targetType, ConversationID: conversationID,
				ScheduleType: schedulestore.ScheduleCron, Cron: "0 0 1 1 *", Timezone: "UTC",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.RunScheduledPrompt(ctx, created.ID); err != nil {
				t.Fatal(err)
			}
			completed := waitForScheduleState(t, manager, created.ID, func(schedule daemon.ScheduledPrompt) bool {
				return !schedule.Pending && schedule.LastStatus == "completed"
			})
			if test.targetType == schedulestore.TargetOneOff {
				if completed.LastConversationID == "" || !strings.HasPrefix(completed.LastConversationID, "chat_") {
					t.Fatalf("one-off conversation id = %q", completed.LastConversationID)
				}
				conversationID = completed.LastConversationID
				chat, err := manager.Chat(ctx, conversationID)
				if err != nil || !strings.HasPrefix(chat.Name, "Run now · ") {
					t.Fatalf("one-off chat = %#v, err = %v", chat, err)
				}
			}
			if got := countUserPrompt(t, manager, conversationID, prompt); got != 1 {
				t.Fatalf("stored scheduled user prompts = %d, want 1", got)
			}

			if test.targetType == schedulestore.TargetOneOff {
				firstChatID := conversationID
				if _, err := manager.RunScheduledPrompt(ctx, created.ID); err != nil {
					t.Fatal(err)
				}
				second := waitForScheduleState(t, manager, created.ID, func(schedule daemon.ScheduledPrompt) bool {
					return !schedule.Pending && schedule.LastStatus == "completed" && schedule.LastConversationID != firstChatID
				})
				if !strings.HasPrefix(second.LastConversationID, "chat_") || second.LastConversationID == firstChatID {
					t.Fatalf("second one-off chat id = %q, first = %q", second.LastConversationID, firstChatID)
				}
				if got := countUserPrompt(t, manager, second.LastConversationID, prompt); got != 1 {
					t.Fatalf("second one-off chat prompts = %d, want 1", got)
				}
			}
		})
	}
}

func TestScheduledPromptQueuesFollowUpForBusyConversation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "input.txt"), []byte("scheduled value"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := newBlockingReadResolver()
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root, DefaultModel: "fake-model",
		Capabilities: resolver, DisableScheduler: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resolver.Unblock()
		closeScheduleTestManager(t, manager)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
		Name: "Queued schedule", Prompt: "run after the active task", TargetType: schedulestore.TargetProject,
		ConversationID: project.ID, ScheduleType: schedulestore.ScheduleCron, Cron: "0 0 1 1 *", Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Prompt(ctx, project.ID, "active task", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-resolver.started:
	case <-ctx.Done():
		t.Fatal("active project run never reached the blocking tool")
	}
	queued, err := manager.RunScheduledPrompt(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queued.LastStatus != "queued" {
		t.Fatalf("run-now status = %q, want queued", queued.LastStatus)
	}
	state, err := manager.State(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.FollowUpsQueued != 1 {
		t.Fatalf("follow-up queue size = %d, want 1", state.FollowUpsQueued)
	}
	resolver.Unblock()
	waitForScheduleState(t, manager, created.ID, func(schedule daemon.ScheduledPrompt) bool {
		return !schedule.Pending && schedule.LastStatus == "completed"
	})
	if got := countUserPrompt(t, manager, project.ID, "run after the active task"); got != 1 {
		t.Fatalf("queued scheduled prompts stored = %d, want 1", got)
	}
}

func TestAutomaticOnceScheduleFiresExactlyOnce(t *testing.T) {
	root := t.TempDir()
	manager := openScheduleTestManager(t, root, false)
	t.Cleanup(func() { closeScheduleTestManager(t, manager) })
	ctx := context.Background()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	runAt := time.Now().UTC().Add(400 * time.Millisecond)
	created, err := manager.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
		Name: "One time", Prompt: "automatic once prompt", TargetType: schedulestore.TargetProject,
		ConversationID: project.ID, ScheduleType: schedulestore.ScheduleOnce, RunAt: &runAt, Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := waitForScheduleState(t, manager, created.ID, func(schedule daemon.ScheduledPrompt) bool {
		return !schedule.Pending && schedule.LastStatus == "completed"
	})
	if completed.Enabled || completed.NextRunAt != nil {
		t.Fatalf("completed once schedule remained enabled: %#v", completed)
	}
	time.Sleep(600 * time.Millisecond)
	if got := countUserPrompt(t, manager, project.ID, "automatic once prompt"); got != 1 {
		t.Fatalf("automatic once prompt count = %d, want 1", got)
	}
}

func TestScheduledPromptPersistsAndRunsAfterManagerRestart(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	manager := openScheduleTestManager(t, root, true)
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	runAt := time.Now().UTC().Add(1200 * time.Millisecond)
	created, err := manager.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
		Name: "Restarted once", Prompt: "run after restart", TargetType: schedulestore.TargetProject,
		ConversationID: project.ID, ScheduleType: schedulestore.ScheduleOnce, RunAt: &runAt, Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	closeScheduleTestManager(t, manager)

	restarted := openScheduleTestManager(t, root, false)
	t.Cleanup(func() { closeScheduleTestManager(t, restarted) })
	persisted, err := restarted.ListScheduledPrompts(ctx, project.ID)
	if err != nil || len(persisted) != 1 || persisted[0].ID != created.ID {
		t.Fatalf("persisted schedules after restart = %#v, err = %v", persisted, err)
	}
	waitForScheduleState(t, restarted, created.ID, func(schedule daemon.ScheduledPrompt) bool {
		return !schedule.Pending && schedule.LastStatus == "completed" && !schedule.Enabled
	})
	if got := countUserPrompt(t, restarted, project.ID, "run after restart"); got != 1 {
		t.Fatalf("post-restart scheduled prompts = %d, want 1", got)
	}
}

func TestDeletingConversationCascadesAttachedSchedules(t *testing.T) {
	root := t.TempDir()
	manager := openScheduleTestManager(t, root, true)
	t.Cleanup(func() { closeScheduleTestManager(t, manager) })
	ctx := context.Background()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := manager.CreateChat(ctx, daemon.CreateChatOptions{Name: "cascade chat"})
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, target, conversationID string) daemon.ScheduledPrompt {
		t.Helper()
		schedule, err := manager.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
			Name: name, Prompt: name + " prompt", TargetType: target, ConversationID: conversationID,
			ScheduleType: schedulestore.ScheduleCron, Cron: "0 0 1 1 *", Timezone: "UTC",
		})
		if err != nil {
			t.Fatal(err)
		}
		return schedule
	}
	projectSchedule := create("project schedule", schedulestore.TargetProject, project.ID)
	chatSchedule := create("chat schedule", schedulestore.TargetChat, chat.ID)
	oneOffSchedule := create("one off schedule", schedulestore.TargetOneOff, "")

	if err := manager.DeleteProject(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
	if err := manager.DeleteChat(ctx, chat.ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := manager.ListScheduledPrompts(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].ID != oneOffSchedule.ID {
		t.Fatalf("schedules after conversation deletion = %#v; deleted project=%s chat=%s", remaining, projectSchedule.ID, chatSchedule.ID)
	}
}

func TestConcurrentRunNowSchedulesAgainstIdleTargetAreDeliveredExactlyOnce(t *testing.T) {
	root := t.TempDir()
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := newBlockingReadResolver()
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root, DefaultModel: "fake-model",
		Capabilities: resolver, DisableScheduler: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resolver.Unblock()
		closeScheduleTestManager(t, manager)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	create := func(name, prompt string) daemon.ScheduledPrompt {
		t.Helper()
		schedule, createErr := manager.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
			Name: name, Prompt: prompt, TargetType: schedulestore.TargetProject, ConversationID: project.ID,
			ScheduleType: schedulestore.ScheduleCron, Cron: "0 0 1 1 *", Timezone: "UTC",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return schedule
	}
	first := create("Concurrent first", "concurrent scheduled prompt one")
	second := create("Concurrent second", "concurrent scheduled prompt two")

	type runResult struct {
		schedule daemon.ScheduledPrompt
		err      error
	}
	start := make(chan struct{})
	results := make(chan runResult, 2)
	for _, id := range []string{first.ID, second.ID} {
		id := id
		go func() {
			<-start
			schedule, runErr := manager.RunScheduledPrompt(ctx, id)
			results <- runResult{schedule: schedule, err: runErr}
		}()
	}
	close(start)
	statuses := make(map[string]string, 2)
	for range 2 {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("concurrent RunScheduledPrompt(): %v", result.err)
			}
			statuses[result.schedule.ID] = result.schedule.LastStatus
		case <-ctx.Done():
			t.Fatal("concurrent schedule dispatches did not return")
		}
	}
	select {
	case <-resolver.started:
	case <-ctx.Done():
		t.Fatal("concurrent schedule run never reached the blocking tool")
	}
	state, err := manager.State(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.FollowUpsQueued != 1 {
		t.Fatalf("concurrent run follow-up queue = %d, want 1 (statuses %v)", state.FollowUpsQueued, statuses)
	}
	for id, status := range statuses {
		if status != "running" && status != "queued" {
			t.Fatalf("concurrent schedule %s status = %q, want running or queued", id, status)
		}
	}
	resolver.Unblock()
	for _, schedule := range []daemon.ScheduledPrompt{first, second} {
		finished := waitForScheduleState(t, manager, schedule.ID, func(current daemon.ScheduledPrompt) bool {
			return !current.Pending && current.LastStatus == "completed"
		})
		if finished.LastError != "" {
			t.Fatalf("schedule %s completed with error %q", schedule.ID, finished.LastError)
		}
	}
	if got := countUserPrompt(t, manager, project.ID, first.Prompt); got != 1 {
		t.Fatalf("first concurrent prompt count = %d, want 1", got)
	}
	if got := countUserPrompt(t, manager, project.ID, second.Prompt); got != 1 {
		t.Fatalf("second concurrent prompt count = %d, want 1", got)
	}
}

func TestAutomaticScheduleRacingConversationSettlementIsNotDropped(t *testing.T) {
	root := t.TempDir()
	conversationStore, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	scheduleStore, err := schedulestore.New(filepath.Join(root, "schedules.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := newBlockingReadResolver()
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: conversationStore, ScheduleStore: scheduleStore, Provider: &fakeProvider{},
		DefaultCWD: root, DefaultModel: "fake-model", Capabilities: resolver,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resolver.Unblock()
		closeScheduleTestManager(t, manager)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	runAt := time.Now().UTC().Add(500 * time.Millisecond)
	schedule, err := manager.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
		Name: "Settlement race", Prompt: "prompt racing settlement", TargetType: schedulestore.TargetProject,
		ConversationID: project.ID, ScheduleType: schedulestore.ScheduleOnce, RunAt: &runAt, Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	storeHeld := make(chan struct{})
	releaseStore := make(chan struct{})
	storeUpdateDone := make(chan error, 1)
	go func() {
		_, updateErr := scheduleStore.Update(context.Background(), schedule.ID, func(*schedulestore.Schedule) error {
			close(storeHeld)
			<-releaseStore
			return nil
		})
		storeUpdateDone <- updateErr
	}()
	select {
	case <-storeHeld:
	case <-ctx.Done():
		t.Fatal("schedule store transaction was not acquired")
	}
	if err := manager.Prompt(ctx, project.ID, "active prompt before settlement", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-resolver.started:
	case <-ctx.Done():
		t.Fatal("active run never reached the blocking tool")
	}
	if delay := time.Until(runAt.Add(100 * time.Millisecond)); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			t.Fatal("timed out waiting for schedule to become due")
		}
	}
	resolver.Unblock()
	waitForConversationRunningState(t, ctx, manager, project.ID, false)
	close(releaseStore)
	select {
	case updateErr := <-storeUpdateDone:
		if updateErr != nil {
			t.Fatal(updateErr)
		}
	case <-ctx.Done():
		t.Fatal("schedule store transaction did not release")
	}
	completed := waitForScheduleState(t, manager, schedule.ID, func(current daemon.ScheduledPrompt) bool {
		return !current.Pending && current.LastStatus == "completed"
	})
	if completed.LastError != "" {
		t.Fatalf("settlement-race schedule error = %q", completed.LastError)
	}
	if got := countUserPrompt(t, manager, project.ID, schedule.Prompt); got != 1 {
		t.Fatalf("settlement-race prompt count = %d, want 1", got)
	}
}

func TestDeleteTargetRacingScheduleCreateLeavesNoDanglingSchedule(t *testing.T) {
	root := t.TempDir()
	manager := openScheduleTestManager(t, root, true)
	t.Cleanup(func() { closeScheduleTestManager(t, manager) })
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	for iteration := range 32 {
		project, err := manager.CreateProject(ctx, daemon.CreateProjectOptions{CWD: root, Tools: []string{"read"}})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		createDone := make(chan error, 1)
		deleteDone := make(chan error, 1)
		go func() {
			<-start
			_, createErr := manager.CreateScheduledPrompt(ctx, daemon.ScheduledPromptInput{
				Name: "Delete race", Prompt: "must not dangle", TargetType: schedulestore.TargetProject,
				ConversationID: project.ID, ScheduleType: schedulestore.ScheduleCron, Cron: "0 0 1 1 *", Timezone: "UTC",
			})
			createDone <- createErr
		}()
		go func() {
			<-start
			deleteDone <- manager.DeleteProject(ctx, project.ID)
		}()
		close(start)
		createErr := <-createDone
		if deleteErr := <-deleteDone; deleteErr != nil {
			t.Fatalf("iteration %d DeleteProject(): %v (create error %v)", iteration, deleteErr, createErr)
		}
		schedules, err := manager.ListScheduledPrompts(ctx, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(schedules) != 0 {
			t.Fatalf("iteration %d left schedules for deleted project %s: %#v", iteration, project.ID, schedules)
		}
		if _, err := manager.Project(ctx, project.ID); err == nil {
			t.Fatalf("iteration %d deleted project remained accessible", iteration)
		}
	}
}

func TestAutomaticSchedulerHonorsDispatchConcurrencyLimit(t *testing.T) {
	root := t.TempDir()
	conversationStore, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	scheduleStore, err := schedulestore.New(filepath.Join(root, "schedules.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for index := range 5 {
		runAt := now.Add(-time.Minute)
		next := now.Add(-time.Second)
		_, err := scheduleStore.Create(context.Background(), schedulestore.Schedule{
			ID: "schedule_limit_" + string(rune('a'+index)), Name: "Limited dispatch", Prompt: "limited prompt",
			TargetType: schedulestore.TargetOneOff, ScheduleType: schedulestore.ScheduleOnce,
			Timezone: "UTC", RunAt: &runAt, Enabled: true, NextRunAt: &next,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	provider := newDispatchLimitProvider(5)
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: conversationStore, ScheduleStore: scheduleStore, Provider: provider,
		DefaultCWD: root, DefaultModel: "fake-model", MaxScheduledDispatches: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		provider.releaseAll()
		closeScheduleTestManager(t, manager)
	})
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for range 2 {
		select {
		case <-provider.started:
		case <-deadline.C:
			t.Fatal("scheduler did not fill both configured dispatch slots")
		}
	}
	select {
	case <-provider.started:
		t.Fatal("scheduler exceeded MaxScheduledDispatches while both slots were blocked")
	case <-time.After(250 * time.Millisecond):
	}
	if got := provider.maximum.Load(); got != 2 {
		t.Fatalf("maximum concurrent scheduled dispatches = %d, want 2", got)
	}
	provider.releaseAll()
	for index := range 5 {
		id := "schedule_limit_" + string(rune('a'+index))
		waitForScheduleState(t, manager, id, func(schedule daemon.ScheduledPrompt) bool {
			return !schedule.Pending && schedule.LastStatus == "completed"
		})
	}
	if got := provider.maximum.Load(); got > 2 {
		t.Fatalf("maximum concurrent scheduled dispatches after completion = %d, limit 2", got)
	}
}

func openScheduleTestManager(t *testing.T, root string, disableScheduler bool) *daemon.Manager {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "input.txt"), []byte("scheduled value"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := daemon.NewManager(daemon.ManagerConfig{
		Store: store, Provider: &fakeProvider{}, DefaultCWD: root, DefaultModel: "fake-model",
		DisableScheduler: disableScheduler,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func closeScheduleTestManager(t *testing.T, manager *daemon.Manager) {
	t.Helper()
	if err := manager.Close(); err != nil {
		t.Errorf("Manager.Close(): %v", err)
	}
}

func waitForScheduleState(t *testing.T, manager *daemon.Manager, id string, ready func(daemon.ScheduledPrompt) bool) daemon.ScheduledPrompt {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		schedules, err := manager.ListScheduledPrompts(context.Background(), "")
		if err != nil {
			t.Fatal(err)
		}
		for _, schedule := range schedules {
			if schedule.ID == id && ready(schedule) {
				return schedule
			}
		}
		select {
		case <-deadline.C:
			t.Fatalf("schedule %s did not reach the requested state; last list: %#v", id, schedules)
		case <-ticker.C:
		}
	}
}

func waitForConversationRunningState(t *testing.T, ctx context.Context, manager *daemon.Manager, id string, running bool) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := manager.State(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if state.Running == running {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("conversation %s running remained %v, want %v", id, state.Running, running)
		case <-ticker.C:
		}
	}
}

type dispatchLimitProvider struct {
	base        fakeProvider
	started     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	active      atomic.Int64
	maximum     atomic.Int64
}

func newDispatchLimitProvider(count int) *dispatchLimitProvider {
	return &dispatchLimitProvider{started: make(chan struct{}, count), release: make(chan struct{})}
}

func (p *dispatchLimitProvider) OpenSession(ctx context.Context, options agent.SessionOptions) (agent.Session, error) {
	active := p.active.Add(1)
	for {
		maximum := p.maximum.Load()
		if active <= maximum || p.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	p.started <- struct{}{}
	select {
	case <-p.release:
	case <-ctx.Done():
		p.active.Add(-1)
		return nil, ctx.Err()
	}
	p.active.Add(-1)
	return p.base.OpenSession(ctx, options)
}

func (p *dispatchLimitProvider) OpenSessionFromState(ctx context.Context, options agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	return p.base.OpenSessionFromState(ctx, options, state)
}

func (p *dispatchLimitProvider) Close() error { return p.base.Close() }

func (p *dispatchLimitProvider) releaseAll() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func countUserPrompt(t *testing.T, manager *daemon.Manager, conversationID, prompt string) int {
	t.Helper()
	messages, err := manager.Messages(context.Background(), conversationID, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, message := range messages {
		if message.Role == "user" && message.Content == prompt {
			count++
		}
	}
	return count
}
