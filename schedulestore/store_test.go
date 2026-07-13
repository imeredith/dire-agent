package schedulestore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dire-kiwi/dire-agent/schedulestore"
)

func TestStoreLifecycleAndPersistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "schedules.db")
	store, err := schedulestore.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Path() != path {
		t.Fatalf("Path() = %q, want %q", store.Path(), path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Fatalf("schedule database permissions = %o, want 600", permissions)
	}

	zone := time.FixedZone("test", 12*60*60)
	runAt := time.Date(2026, time.July, 20, 9, 30, 0, 0, zone)
	nextRunAt := runAt.Add(24 * time.Hour)
	lastRunAt := runAt.Add(-24 * time.Hour)
	created, err := store.Create(ctx, schedulestore.Schedule{
		ID: "schedule_daily", Name: "Daily summary", Prompt: "Summarize the project",
		TargetType: schedulestore.TargetProject, ConversationID: "project_123",
		ScheduleType: schedulestore.ScheduleCron, Cron: "30 9 * * *", Timezone: "Pacific/Auckland",
		RunAt: &runAt, Enabled: true, NextRunAt: &nextRunAt, LastRunAt: &lastRunAt,
		LastStatus: "succeeded", LastConversationID: "project_123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("timestamps were not set: %#v", created)
	}
	assertUTC(t, created.CreatedAt, "created_at")
	assertUTC(t, created.UpdatedAt, "updated_at")
	assertUTCPointer(t, created.RunAt, "run_at")
	assertUTCPointer(t, created.NextRunAt, "next_run_at")
	assertUTCPointer(t, created.LastRunAt, "last_run_at")

	got, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Fatalf("Get() = %#v, want %#v", got, created)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := schedulestore.New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = reopened.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, created) {
		t.Fatalf("reopened Get() = %#v, want %#v", got, created)
	}

	updated, err := reopened.Update(ctx, created.ID, func(schedule *schedulestore.Schedule) error {
		schedule.Name = "Updated daily summary"
		schedule.Enabled = false
		schedule.LastStatus = "failed"
		schedule.LastError = "network unavailable"
		schedule.ConversationID = "project_456"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Updated daily summary" || updated.Enabled || updated.LastError != "network unavailable" {
		t.Fatalf("Update() = %#v", updated)
	}
	if updated.ConversationID != "project_456" {
		t.Fatalf("updated conversation_id = %q", updated.ConversationID)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("created_at changed from %s to %s", created.CreatedAt, updated.CreatedAt)
	}
	if updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Fatalf("updated_at moved backwards from %s to %s", created.UpdatedAt, updated.UpdatedAt)
	}

	if err := reopened.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Get(ctx, created.ID); !errors.Is(err, schedulestore.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
	if err := reopened.Delete(ctx, created.ID); !errors.Is(err, schedulestore.ErrNotFound) {
		t.Fatalf("second Delete() error = %v, want ErrNotFound", err)
	}
}

func TestListOrdersByCreatedAtThenID(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	base := time.Date(2026, time.July, 12, 0, 0, 0, 0, time.UTC)
	for _, schedule := range []schedulestore.Schedule{
		{ID: "schedule_c", CreatedAt: base.Add(time.Second)},
		{ID: "schedule_b", CreatedAt: base},
		{ID: "schedule_a", CreatedAt: base},
	} {
		if _, err := store.Create(ctx, schedule); err != nil {
			t.Fatal(err)
		}
	}
	schedules, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(schedules))
	for i := range schedules {
		got[i] = schedules[i].ID
	}
	want := []string{"schedule_a", "schedule_b", "schedule_c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() IDs = %v, want %v", got, want)
	}
}

func TestDeleteForConversation(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	for _, schedule := range []schedulestore.Schedule{
		{ID: "schedule_project_a_1", ConversationID: "project_a"},
		{ID: "schedule_project_a_2", ConversationID: "project_a"},
		{ID: "schedule_project_b", ConversationID: "project_b"},
		{ID: "schedule_one_off", TargetType: schedulestore.TargetOneOff},
	} {
		if _, err := store.Create(ctx, schedule); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteForConversation(ctx, "project_a"); err != nil {
		t.Fatal(err)
	}
	schedules, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(schedules))
	for i := range schedules {
		got[i] = schedules[i].ID
	}
	want := []string{"schedule_project_b", "schedule_one_off"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining schedules = %v, want %v", got, want)
	}
	if err := store.DeleteForConversation(ctx, "project_missing"); err != nil {
		t.Fatalf("DeleteForConversation missing target: %v", err)
	}
	if err := store.DeleteForConversation(ctx, ""); err == nil {
		t.Fatal("DeleteForConversation accepted an empty ID")
	}
}

func TestUpdateFailureIsAtomicAndIDIsImmutable(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	created, err := store.Create(ctx, schedulestore.Schedule{ID: "schedule_atomic", Name: "before"})
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("stop update")
	if _, err := store.Update(ctx, created.ID, func(schedule *schedulestore.Schedule) error {
		schedule.Name = "after"
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("Update() callback error = %v, want %v", err, wantErr)
	}
	got, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "before" {
		t.Fatalf("failed update persisted name %q", got.Name)
	}
	if _, err := store.Update(ctx, created.ID, func(schedule *schedulestore.Schedule) error {
		schedule.ID = "schedule_replacement"
		return nil
	}); err == nil {
		t.Fatal("Update() allowed the schedule ID to change")
	}
	if _, err := store.Get(ctx, "schedule_replacement"); !errors.Is(err, schedulestore.ErrNotFound) {
		t.Fatalf("replacement schedule error = %v, want ErrNotFound", err)
	}
}

func TestRejectsUnsafeAndDuplicateIDs(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	invalid := []string{"", " schedule", ".hidden", "with/slash", "with space", strings.Repeat("a", 129)}
	for _, id := range invalid {
		if _, err := store.Create(ctx, schedulestore.Schedule{ID: id}); err == nil {
			t.Errorf("Create() accepted invalid ID %q", id)
		}
	}
	if _, err := store.Create(ctx, schedulestore.Schedule{ID: "schedule_valid-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, schedulestore.Schedule{ID: "schedule_valid-1"}); err == nil {
		t.Fatal("Create() accepted a duplicate ID")
	}
	if _, err := store.Get(ctx, "../schedule_valid-1"); err == nil {
		t.Fatal("Get() accepted an unsafe ID")
	}
}

func TestNewRejectsEmptyPath(t *testing.T) {
	if _, err := schedulestore.New("  "); err == nil {
		t.Fatal("New() accepted an empty path")
	}
}

func newStore(t *testing.T) *schedulestore.Store {
	t.Helper()
	store, err := schedulestore.New(filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close(): %v", err)
		}
	})
	return store
}

func assertUTC(t *testing.T, value time.Time, name string) {
	t.Helper()
	if value.Location() != time.UTC {
		t.Errorf("%s location = %v, want UTC", name, value.Location())
	}
}

func assertUTCPointer(t *testing.T, value *time.Time, name string) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s is nil", name)
	}
	assertUTC(t, *value, name)
}
