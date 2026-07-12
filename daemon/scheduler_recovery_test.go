package daemon

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/schedulestore"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func TestRecoverInterruptedOnceSchedulesRetriesOnlyAutomaticClaims(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := schedulestore.New(filepath.Join(t.TempDir(), "schedules.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runAt := time.Date(2026, time.July, 12, 9, 0, 0, 0, time.UTC)
	for _, schedule := range []schedulestore.Schedule{
		{
			ID: "schedule_automatic_interrupted", Name: "Automatic", Prompt: "retry me",
			TargetType: schedulestore.TargetOneOff, ScheduleType: schedulestore.ScheduleOnce,
			Timezone: "UTC", RunAt: &runAt, Enabled: false, Pending: true, RetryPending: true,
			LastStatus: "running",
		},
		{
			ID: "schedule_manual_interrupted", Name: "Manual", Prompt: "do not enable me",
			TargetType: schedulestore.TargetOneOff, ScheduleType: schedulestore.ScheduleOnce,
			Timezone: "UTC", RunAt: &runAt, Enabled: false, Pending: true, RetryPending: false,
			LastStatus: "running",
		},
	} {
		if _, err := store.Create(ctx, schedule); err != nil {
			t.Fatal(err)
		}
	}
	manager := &Manager{
		scheduleStore:       store,
		schedulerWake:       make(chan struct{}, 1),
		scheduleSubscribers: make(map[uint64]chan ScheduleEvent),
	}
	recoveredAt := time.Date(2026, time.July, 12, 10, 0, 0, 0, time.UTC)
	if err := manager.recoverInterruptedSchedules(ctx, recoveredAt); err != nil {
		t.Fatal(err)
	}

	automatic, err := store.Get(ctx, "schedule_automatic_interrupted")
	if err != nil {
		t.Fatal(err)
	}
	if automatic.Pending || automatic.RetryPending || !automatic.Enabled || automatic.NextRunAt == nil || !automatic.NextRunAt.Equal(recoveredAt) {
		t.Fatalf("recovered automatic once schedule = %#v", automatic)
	}
	if automatic.LastStatus != "interrupted" || !strings.Contains(automatic.LastError, "retrying") {
		t.Fatalf("automatic recovery status/error = %q/%q", automatic.LastStatus, automatic.LastError)
	}

	manual, err := store.Get(ctx, "schedule_manual_interrupted")
	if err != nil {
		t.Fatal(err)
	}
	if manual.Pending || manual.RetryPending || manual.Enabled || manual.NextRunAt != nil {
		t.Fatalf("recovered manual disabled once schedule = %#v", manual)
	}
	if manual.LastStatus != "interrupted" || !strings.Contains(manual.LastError, "manually started") {
		t.Fatalf("manual recovery status/error = %q/%q", manual.LastStatus, manual.LastError)
	}
}

func TestAutomaticScheduleSettlingRetryDeliversClaimedPrompt(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conversations, err := threadstore.New(filepath.Join(root, "conversations"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(ManagerConfig{
		Store: conversations, Provider: schedulerEchoProvider{}, DefaultCWD: root,
		DefaultProvider: "test", DefaultModel: "test-model", DisableScheduler: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	project, err := manager.CreateProject(ctx, CreateProjectOptions{CWD: root})
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(time.Hour)
	schedule, err := manager.CreateScheduledPrompt(ctx, ScheduledPromptInput{
		Name: "Settling retry", Prompt: "deliver after settling", TargetType: schedulestore.TargetProject,
		ConversationID: project.ID, ScheduleType: schedulestore.ScheduleOnce, RunAt: &future, Timezone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC()
	if _, err := manager.scheduleStore.Update(ctx, schedule.ID, func(current *schedulestore.Schedule) error {
		current.NextRunAt = &due
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	runtime, err := manager.getRuntime(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	runtime.finishing = true
	runtime.mu.Unlock()
	waiting, err := manager.triggerScheduledPrompt(ctx, schedule.ID, true, due)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Pending || !waiting.Enabled || waiting.NextRunAt == nil || waiting.LastStatus != "waiting" {
		t.Fatalf("schedule after settling collision = %#v", waiting)
	}
	if !strings.Contains(waiting.LastError, "retrying") {
		t.Fatalf("settling collision error = %q", waiting.LastError)
	}
	messages, err := manager.Messages(ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if message.Role == "user" && message.Content == schedule.Prompt {
			t.Fatal("settling collision dispatched the prompt before retry")
		}
	}
	runtime.mu.Lock()
	runtime.finishing = false
	runtime.mu.Unlock()
	retryAt := waiting.NextRunAt.Add(time.Nanosecond)
	if _, err := manager.triggerScheduledPrompt(ctx, schedule.ID, true, retryAt); err != nil {
		t.Fatal(err)
	}
	runtime.runWG.Wait()
	completed, err := manager.scheduleStore.Get(ctx, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Pending || completed.Enabled || completed.LastStatus != "completed" || completed.LastError != "" {
		t.Fatalf("schedule after settling retry = %#v", completed)
	}
	messages, err = manager.Messages(ctx, project.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	delivered := 0
	for _, message := range messages {
		if message.Role == "user" && message.Content == schedule.Prompt {
			delivered++
		}
	}
	if delivered != 1 {
		t.Fatalf("settling-retry prompt deliveries = %d, want 1", delivered)
	}
}

type schedulerEchoProvider struct{}

func (schedulerEchoProvider) OpenSession(context.Context, agent.SessionOptions) (agent.Session, error) {
	return &schedulerEchoSession{id: "scheduler-echo"}, nil
}

func (schedulerEchoProvider) OpenSessionFromState(_ context.Context, _ agent.SessionOptions, state agent.SessionState) (agent.Session, error) {
	return &schedulerEchoSession{id: state.ID}, nil
}

func (schedulerEchoProvider) Close() error { return nil }

type schedulerEchoSession struct{ id string }

func (s *schedulerEchoSession) ID() string { return s.id }

func (s *schedulerEchoSession) Run(ctx context.Context, prompt string) (agent.Result, error) {
	step, err := s.Step(ctx, agent.StepRequest{UserMessages: []string{prompt}})
	return step.Result, err
}

func (s *schedulerEchoSession) Step(_ context.Context, request agent.StepRequest) (agent.StepResult, error) {
	text := "done"
	if len(request.UserMessages) > 0 {
		text = "done: " + request.UserMessages[len(request.UserMessages)-1]
	}
	return agent.StepResult{Result: agent.Result{Text: text, SessionID: s.id}}, nil
}

func (s *schedulerEchoSession) State() (agent.SessionState, error) {
	return agent.SessionState{ID: s.id, Provider: "test", Data: json.RawMessage(`[]`)}, nil
}
