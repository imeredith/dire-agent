package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dire-kiwi/dire-agent/schedulestore"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

const (
	schedulerErrorRetry = 5 * time.Second
	schedulerClaimRetry = 250 * time.Millisecond
	maxScheduleName     = 120
	maxSchedulePrompt   = 1 << 20
)

var errScheduleNotDue = errors.New("daemon: scheduled prompt is not due")

func (m *Manager) startScheduler() error {
	base := m.config.Context
	if base == nil {
		base = context.Background()
	}
	m.schedulerCtx, m.schedulerCancel = context.WithCancel(base)
	if err := m.recoverInterruptedSchedules(m.schedulerCtx, time.Now().UTC()); err != nil {
		m.schedulerCancel()
		return fmt.Errorf("daemon: recover scheduled prompts: %w", err)
	}
	m.schedulerWG.Add(1)
	go func() {
		defer m.schedulerWG.Done()
		m.schedulerLoop(m.schedulerCtx)
	}()
	return nil
}

func (m *Manager) schedulerLoop(ctx context.Context) {
	for ctx.Err() == nil {
		now := time.Now().UTC()
		schedules, err := m.scheduleStore.List(ctx)
		if err != nil {
			if !m.waitForScheduler(ctx, schedulerErrorRetry) {
				return
			}
			continue
		}
		due := 0
		for _, schedule := range schedules {
			if !schedule.Enabled || schedule.NextRunAt == nil || schedule.NextRunAt.After(now) {
				continue
			}
			select {
			case m.schedulerSlots <- struct{}{}:
			default:
				continue
			}
			if !m.beginScheduledDispatch(schedule.ID) {
				<-m.schedulerSlots
				continue
			}
			due++
			scheduleID := schedule.ID
			m.schedulerDispatchWG.Add(1)
			go func() {
				defer m.schedulerDispatchWG.Done()
				defer m.endScheduledDispatch(scheduleID)
				defer func() { <-m.schedulerSlots }()
				triggered, triggerErr := m.triggerScheduledPrompt(ctx, scheduleID, true, now)
				if triggerErr != nil {
					if !errors.Is(triggerErr, errScheduleNotDue) && ctx.Err() == nil {
						m.recordSchedulerDispatchError(scheduleID, triggerErr)
					}
					return
				}
				if triggered.Pending {
					m.waitForScheduledPrompt(ctx, scheduleID)
				}
			}()
		}
		if due > 0 {
			if !m.waitForScheduler(ctx, schedulerClaimRetry) {
				return
			}
			continue
		}
		next := earliestScheduledTime(schedules)
		if next == nil {
			if !m.waitForScheduler(ctx, 0) {
				return
			}
			continue
		}
		delay := time.Until(*next)
		if delay < schedulerClaimRetry {
			delay = schedulerClaimRetry
		}
		if delay > time.Minute {
			delay = time.Minute
		}
		if !m.waitForScheduler(ctx, delay) {
			return
		}
	}
}

func (m *Manager) waitForScheduledPrompt(ctx context.Context, id string) {
	ticker := time.NewTicker(schedulerClaimRetry)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			schedule, err := m.scheduleStore.Get(ctx, id)
			if err != nil || !schedule.Pending {
				return
			}
		}
	}
}

func (m *Manager) beginScheduledDispatch(id string) bool {
	m.schedulerFlightMu.Lock()
	defer m.schedulerFlightMu.Unlock()
	if m.schedulerInFlight[id] {
		return false
	}
	m.schedulerInFlight[id] = true
	return true
}

func (m *Manager) endScheduledDispatch(id string) {
	m.schedulerFlightMu.Lock()
	delete(m.schedulerInFlight, id)
	m.schedulerFlightMu.Unlock()
	m.wakeScheduler()
}

func (m *Manager) recordSchedulerDispatchError(id string, dispatchErr error) {
	m.scheduleMu.Lock()
	updated, err := m.scheduleStore.Update(context.Background(), id, func(schedule *schedulestore.Schedule) error {
		now := time.Now().UTC()
		if schedule.Pending || !schedule.Enabled || schedule.NextRunAt == nil || schedule.NextRunAt.After(now) {
			return errScheduleNotDue
		}
		retryAt := now.Add(30 * time.Second)
		schedule.NextRunAt = &retryAt
		schedule.LastStatus = "failed"
		schedule.LastError = dispatchErr.Error()
		return nil
	})
	m.scheduleMu.Unlock()
	if err == nil {
		m.emitScheduleEvent("scheduled_prompt_failed", updated)
	}
}

func earliestScheduledTime(schedules []schedulestore.Schedule) *time.Time {
	var earliest *time.Time
	for _, schedule := range schedules {
		if !schedule.Enabled || schedule.NextRunAt == nil {
			continue
		}
		if earliest == nil || schedule.NextRunAt.Before(*earliest) {
			value := schedule.NextRunAt.UTC()
			earliest = &value
		}
	}
	return earliest
}

func (m *Manager) waitForScheduler(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return false
		case <-m.schedulerWake:
			return true
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-m.schedulerWake:
		return true
	case <-timer.C:
		return true
	}
}

func (m *Manager) wakeScheduler() {
	select {
	case m.schedulerWake <- struct{}{}:
	default:
	}
}

func (m *Manager) recoverInterruptedSchedules(ctx context.Context, now time.Time) error {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	schedules, err := m.scheduleStore.List(ctx)
	if err != nil {
		return err
	}
	for _, schedule := range schedules {
		if !schedule.Pending {
			continue
		}
		updated, updateErr := m.scheduleStore.Update(ctx, schedule.ID, func(current *schedulestore.Schedule) error {
			if !current.Pending {
				return nil
			}
			current.Pending = false
			if current.RetryPending {
				current.Enabled = true
				retry := now.UTC()
				current.NextRunAt = &retry
				current.LastError = "daemon stopped before the scheduled run settled; retrying"
			} else {
				current.LastError = "daemon stopped before the manually started run settled"
			}
			current.LastStatus = "interrupted"
			current.RetryPending = false
			return nil
		})
		if updateErr != nil {
			return updateErr
		}
		m.emitScheduleEvent("scheduled_prompt_updated", updated)
	}
	return nil
}

// ListScheduledPrompts lists all schedules, or only schedules attached to the
// supplied conversation when conversationID is non-empty.
func (m *Manager) ListScheduledPrompts(ctx context.Context, conversationID string) ([]schedulestore.Schedule, error) {
	schedules, err := m.scheduleStore.List(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]schedulestore.Schedule, 0, len(schedules))
	for _, schedule := range schedules {
		if conversationID == "" || schedule.ConversationID == conversationID {
			filtered = append(filtered, schedule)
		}
	}
	return filtered, nil
}

func (m *Manager) CreateScheduledPrompt(ctx context.Context, input ScheduledPromptInput) (schedulestore.Schedule, error) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	now := time.Now().UTC()
	schedule := schedulestore.Schedule{
		Name: input.Name, Prompt: input.Prompt, TargetType: input.TargetType,
		ConversationID: input.ConversationID, ScheduleType: input.ScheduleType,
		Cron: input.Cron, Timezone: input.Timezone, RunAt: cloneTime(input.RunAt), Enabled: true,
	}
	if input.Enabled != nil {
		schedule.Enabled = *input.Enabled
	}
	if err := m.normalizeAndValidateSchedule(ctx, &schedule, now); err != nil {
		return schedulestore.Schedule{}, err
	}
	if schedule.Enabled {
		next, err := nextScheduledPromptTime(schedule, now)
		if err != nil {
			return schedulestore.Schedule{}, err
		}
		schedule.NextRunAt = &next
	}
	id, err := newScheduleID()
	if err != nil {
		return schedulestore.Schedule{}, err
	}
	schedule.ID = id
	created, err := m.scheduleStore.Create(ctx, schedule)
	if err != nil {
		return schedulestore.Schedule{}, err
	}
	m.emitScheduleEvent("scheduled_prompt_created", created)
	m.wakeScheduler()
	return created, nil
}

func (m *Manager) UpdateScheduledPrompt(ctx context.Context, id string, input ScheduledPromptInput) (schedulestore.Schedule, error) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	existing, err := m.scheduleStore.Get(ctx, id)
	if err != nil {
		return schedulestore.Schedule{}, err
	}
	next := mergeScheduledPromptInput(existing, input)
	if existing.Pending && scheduleMateriallyChanged(existing, next) {
		return schedulestore.Schedule{}, errors.New("daemon: a running or queued scheduled prompt cannot be edited")
	}
	now := time.Now().UTC()
	if err := m.normalizeAndValidateSchedule(ctx, &next, now); err != nil {
		return schedulestore.Schedule{}, err
	}
	timingChanged := scheduleTimingChanged(existing, next)
	switch {
	case !next.Enabled:
		next.NextRunAt = nil
	case !existing.Enabled || existing.NextRunAt == nil || timingChanged:
		value, nextErr := nextScheduledPromptTime(next, now)
		if nextErr != nil {
			return schedulestore.Schedule{}, nextErr
		}
		next.NextRunAt = &value
	default:
		next.NextRunAt = cloneTime(existing.NextRunAt)
	}
	updated, err := m.scheduleStore.Update(ctx, id, func(current *schedulestore.Schedule) error {
		createdAt, lastRunAt := current.CreatedAt, current.LastRunAt
		lastStatus, lastError := current.LastStatus, current.LastError
		lastConversationID, pending, retryPending := current.LastConversationID, current.Pending, current.RetryPending
		*current = next
		current.ID, current.CreatedAt = id, createdAt
		current.LastRunAt, current.LastStatus, current.LastError = lastRunAt, lastStatus, lastError
		current.LastConversationID, current.Pending, current.RetryPending = lastConversationID, pending, retryPending
		if current.Pending && input.Enabled != nil && !*input.Enabled {
			current.RetryPending = false
		}
		return nil
	})
	if err != nil {
		return schedulestore.Schedule{}, err
	}
	m.emitScheduleEvent("scheduled_prompt_updated", updated)
	m.wakeScheduler()
	return updated, nil
}

func (m *Manager) DeleteScheduledPrompt(ctx context.Context, id string) error {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	schedule, err := m.scheduleStore.Get(ctx, id)
	if err != nil {
		return err
	}
	if schedule.Pending {
		return errors.New("daemon: cannot delete a running or queued scheduled prompt")
	}
	if err := m.scheduleStore.Delete(ctx, id); err != nil {
		return err
	}
	m.emitScheduleEvent("scheduled_prompt_deleted", schedule)
	m.wakeScheduler()
	return nil
}

func (m *Manager) RunScheduledPrompt(ctx context.Context, id string) (schedulestore.Schedule, error) {
	return m.triggerScheduledPrompt(ctx, id, false, time.Now().UTC())
}

func (m *Manager) triggerScheduledPrompt(ctx context.Context, id string, automatic bool, now time.Time) (schedulestore.Schedule, error) {
	now = now.UTC()
	coalesced := false
	m.scheduleMu.Lock()
	claimed, err := m.scheduleStore.Update(ctx, id, func(schedule *schedulestore.Schedule) error {
		if automatic && (!schedule.Enabled || schedule.NextRunAt == nil || schedule.NextRunAt.After(now)) {
			return errScheduleNotDue
		}
		if schedule.Pending {
			if !automatic {
				return errors.New("daemon: scheduled prompt is already running or queued")
			}
			if schedule.ScheduleType == schedulestore.ScheduleCron {
				next, nextErr := nextCronTime(schedule.Cron, schedule.Timezone, now)
				if nextErr != nil {
					return nextErr
				}
				schedule.NextRunAt = &next
			} else {
				retryAt := now.Add(schedulerErrorRetry)
				schedule.NextRunAt = &retryAt
			}
			coalesced = true
			return nil
		}
		if automatic {
			switch schedule.ScheduleType {
			case schedulestore.ScheduleCron:
				next, nextErr := nextCronTime(schedule.Cron, schedule.Timezone, now)
				if nextErr != nil {
					return nextErr
				}
				schedule.NextRunAt = &next
			case schedulestore.ScheduleOnce:
				schedule.Enabled = false
				schedule.NextRunAt = nil
			}
		}
		runAt := now
		schedule.LastRunAt = &runAt
		schedule.LastStatus = "dispatching"
		schedule.LastError = ""
		schedule.Pending = true
		schedule.RetryPending = automatic
		return nil
	})
	m.scheduleMu.Unlock()
	if err != nil {
		return schedulestore.Schedule{}, err
	}
	if coalesced {
		m.emitScheduleEvent("scheduled_prompt_coalesced", claimed)
		m.wakeScheduler()
		return claimed, nil
	}

	dispatched, dispatchErr := m.dispatchScheduledPrompt(ctx, claimed, now)
	if dispatchErr != nil {
		if automatic && errors.Is(dispatchErr, context.Canceled) {
			retrying, updateErr := m.scheduleStore.Update(context.Background(), id, func(schedule *schedulestore.Schedule) error {
				shouldRetry := schedule.RetryPending
				retryAt := time.Now().UTC()
				schedule.Pending = false
				schedule.RetryPending = false
				schedule.LastStatus = "interrupted"
				if shouldRetry {
					schedule.Enabled = true
					schedule.NextRunAt = &retryAt
					schedule.LastError = "daemon stopped before dispatch; retrying after restart"
				} else {
					schedule.LastError = "scheduled prompt was disabled before dispatch"
				}
				if dispatched.LastConversationID != "" {
					schedule.LastConversationID = dispatched.LastConversationID
				}
				return nil
			})
			if updateErr == nil {
				m.emitScheduleEvent("scheduled_prompt_updated", retrying)
				return retrying, nil
			}
		}
		if automatic && strings.Contains(dispatchErr.Error(), "conversation is settling") {
			retrying, updateErr := m.scheduleStore.Update(context.Background(), id, func(schedule *schedulestore.Schedule) error {
				shouldRetry := schedule.RetryPending
				retryAt := time.Now().UTC().Add(schedulerErrorRetry)
				schedule.Pending = false
				schedule.RetryPending = false
				if shouldRetry {
					schedule.Enabled = true
					schedule.NextRunAt = &retryAt
					schedule.LastStatus = "waiting"
					schedule.LastError = "conversation is settling; retrying shortly"
				} else {
					schedule.LastStatus = "interrupted"
					schedule.LastError = "scheduled prompt was disabled before dispatch"
				}
				return nil
			})
			if updateErr == nil {
				m.emitScheduleEvent("scheduled_prompt_updated", retrying)
				m.wakeScheduler()
				return retrying, nil
			}
		}
		failed, updateErr := m.scheduleStore.Update(context.Background(), id, func(schedule *schedulestore.Schedule) error {
			schedule.Pending = false
			schedule.RetryPending = false
			schedule.LastStatus = "failed"
			schedule.LastError = dispatchErr.Error()
			if dispatched.LastConversationID != "" {
				schedule.LastConversationID = dispatched.LastConversationID
			}
			return nil
		})
		if updateErr == nil {
			m.emitScheduleEvent("scheduled_prompt_failed", failed)
			m.wakeScheduler()
			return failed, dispatchErr
		}
		return dispatched, errors.Join(dispatchErr, updateErr)
	}
	updated, err := m.scheduleStore.Get(context.Background(), id)
	if err != nil {
		return schedulestore.Schedule{}, err
	}
	m.wakeScheduler()
	return updated, nil
}

func (m *Manager) dispatchScheduledPrompt(ctx context.Context, schedule schedulestore.Schedule, now time.Time) (schedulestore.Schedule, error) {
	switch schedule.TargetType {
	case schedulestore.TargetProject, schedulestore.TargetChat:
		schedule.LastConversationID = schedule.ConversationID
		if err := m.promptWithAttachments(ctx, schedule.ConversationID, schedule.Prompt, "followUp", nil, func(queued bool) error {
			status := "running"
			if queued {
				status = "queued"
			}
			accepted, err := m.markScheduledPromptAccepted(schedule.ID, schedule.LastConversationID, status)
			if err == nil {
				schedule = accepted
			}
			return err
		}); err != nil {
			return schedule, err
		}
		return schedule, nil
	case schedulestore.TargetOneOff:
		location, _, err := loadScheduleLocation(schedule.Timezone)
		if err != nil {
			return schedule, err
		}
		name := fmt.Sprintf("%s · %s", schedule.Name, now.In(location).Format("2006-01-02 15:04"))
		chat, err := m.CreateChat(ctx, CreateChatOptions{Name: name})
		if err != nil {
			return schedule, err
		}
		schedule.LastConversationID = chat.ID
		if err := m.promptWithAttachments(ctx, chat.ID, schedule.Prompt, "", nil, func(queued bool) error {
			accepted, acceptedErr := m.markScheduledPromptAccepted(schedule.ID, chat.ID, "running")
			if acceptedErr == nil {
				schedule = accepted
			}
			return acceptedErr
		}); err != nil {
			return schedule, err
		}
		return schedule, nil
	default:
		return schedule, fmt.Errorf("daemon: unsupported schedule target type %q", schedule.TargetType)
	}
}

func (m *Manager) markScheduledPromptAccepted(id, conversationID, status string) (schedulestore.Schedule, error) {
	updated, err := m.scheduleStore.Update(context.Background(), id, func(schedule *schedulestore.Schedule) error {
		if !schedule.Pending {
			return errors.New("daemon: scheduled prompt dispatch was canceled")
		}
		schedule.LastConversationID = conversationID
		schedule.LastStatus = status
		schedule.LastError = ""
		return nil
	})
	if err == nil {
		m.emitScheduleEvent("scheduled_prompt_triggered", updated)
	}
	return updated, err
}

func (m *Manager) completeScheduledPrompts(conversationID string, runErr error) {
	schedules, err := m.scheduleStore.List(context.Background())
	if err != nil {
		return
	}
	for _, schedule := range schedules {
		if !scheduleReadyForCompletion(schedule, conversationID) {
			continue
		}
		updated, updateErr := m.scheduleStore.Update(context.Background(), schedule.ID, func(current *schedulestore.Schedule) error {
			if !scheduleReadyForCompletion(*current, conversationID) {
				return nil
			}
			current.Pending = false
			current.RetryPending = false
			if runErr != nil {
				current.LastStatus = "failed"
				current.LastError = runErr.Error()
			} else {
				current.LastStatus = "completed"
				current.LastError = ""
			}
			return nil
		})
		if updateErr == nil {
			eventType := "scheduled_prompt_completed"
			if runErr != nil {
				eventType = "scheduled_prompt_failed"
			}
			m.emitScheduleEvent(eventType, updated)
		}
	}
	m.wakeScheduler()
}

func scheduleReadyForCompletion(schedule schedulestore.Schedule, conversationID string) bool {
	if !schedule.Pending || schedule.LastConversationID != conversationID {
		return false
	}
	return schedule.LastStatus == "running" || schedule.LastStatus == "queued"
}

func (m *Manager) normalizeAndValidateSchedule(ctx context.Context, schedule *schedulestore.Schedule, now time.Time) error {
	schedule.Name = strings.TrimSpace(schedule.Name)
	schedule.Prompt = strings.TrimSpace(schedule.Prompt)
	schedule.TargetType = strings.ToLower(strings.TrimSpace(schedule.TargetType))
	schedule.ScheduleType = strings.ToLower(strings.TrimSpace(schedule.ScheduleType))
	schedule.Cron = strings.TrimSpace(schedule.Cron)
	schedule.ConversationID = strings.TrimSpace(schedule.ConversationID)
	if schedule.Name == "" {
		return errors.New("daemon: scheduled prompt name is required")
	}
	if utf8.RuneCountInString(schedule.Name) > maxScheduleName {
		return fmt.Errorf("daemon: scheduled prompt name must be %d characters or fewer", maxScheduleName)
	}
	for _, value := range schedule.Name {
		if value < ' ' || value == 0x7f {
			return errors.New("daemon: scheduled prompt name must not contain control characters")
		}
	}
	if schedule.Prompt == "" {
		return errors.New("daemon: scheduled prompt is empty")
	}
	if len(schedule.Prompt) > maxSchedulePrompt {
		return fmt.Errorf("daemon: scheduled prompt must be %d bytes or fewer", maxSchedulePrompt)
	}
	switch schedule.TargetType {
	case schedulestore.TargetOneOff:
		if schedule.ConversationID != "" {
			return errors.New("daemon: one-off scheduled prompts must not specify a conversation")
		}
	case schedulestore.TargetProject, schedulestore.TargetChat:
		if schedule.ConversationID == "" {
			return errors.New("daemon: scheduled prompt conversation is required")
		}
		resource, err := m.Thread(ctx, schedule.ConversationID)
		if err != nil {
			return err
		}
		if resource.IsSubagent() {
			return errors.New("daemon: scheduled prompts cannot target child agents")
		}
		if resource.ResourceKind() != schedule.TargetType {
			return fmt.Errorf("daemon: schedule target %q is a %s, not a %s", schedule.ConversationID, resource.ResourceKind(), schedule.TargetType)
		}
	default:
		return fmt.Errorf("daemon: schedule target_type must be %q, %q, or %q", schedulestore.TargetProject, schedulestore.TargetChat, schedulestore.TargetOneOff)
	}

	_, timezone, err := loadScheduleLocation(schedule.Timezone)
	if err != nil {
		return err
	}
	schedule.Timezone = timezone
	switch schedule.ScheduleType {
	case schedulestore.ScheduleCron:
		if schedule.Cron == "" {
			return errors.New("daemon: cron expression is required")
		}
		if _, err := parseCronExpression(schedule.Cron); err != nil {
			return err
		}
		schedule.RunAt = nil
	case schedulestore.ScheduleOnce:
		if schedule.RunAt == nil || schedule.RunAt.IsZero() {
			return errors.New("daemon: run_at is required for a one-time scheduled prompt")
		}
		runAt := schedule.RunAt.UTC()
		schedule.RunAt = &runAt
		schedule.Cron = ""
		if schedule.Enabled && !runAt.After(now) {
			return errors.New("daemon: one-time scheduled prompt must run in the future")
		}
	default:
		return fmt.Errorf("daemon: schedule_type must be %q or %q", schedulestore.ScheduleCron, schedulestore.ScheduleOnce)
	}
	return nil
}

func nextScheduledPromptTime(schedule schedulestore.Schedule, after time.Time) (time.Time, error) {
	if schedule.ScheduleType == schedulestore.ScheduleOnce {
		if schedule.RunAt == nil {
			return time.Time{}, errors.New("daemon: run_at is required for a one-time scheduled prompt")
		}
		return schedule.RunAt.UTC(), nil
	}
	return nextCronTime(schedule.Cron, schedule.Timezone, after)
}

func mergeScheduledPromptInput(current schedulestore.Schedule, input ScheduledPromptInput) schedulestore.Schedule {
	next := current
	if strings.TrimSpace(input.Name) != "" {
		next.Name = input.Name
	}
	if strings.TrimSpace(input.Prompt) != "" {
		next.Prompt = input.Prompt
	}
	if strings.TrimSpace(input.TargetType) != "" {
		next.TargetType = input.TargetType
	}
	if strings.TrimSpace(input.ConversationID) != "" {
		next.ConversationID = input.ConversationID
	}
	if strings.TrimSpace(input.ScheduleType) != "" {
		next.ScheduleType = input.ScheduleType
	}
	if strings.TrimSpace(input.Cron) != "" {
		next.Cron = input.Cron
	}
	if strings.TrimSpace(input.Timezone) != "" {
		next.Timezone = input.Timezone
	}
	if input.RunAt != nil {
		next.RunAt = cloneTime(input.RunAt)
	}
	if input.Enabled != nil {
		next.Enabled = *input.Enabled
	}
	if strings.EqualFold(strings.TrimSpace(input.TargetType), schedulestore.TargetOneOff) {
		next.ConversationID = ""
	}
	return next
}

func scheduleMateriallyChanged(left, right schedulestore.Schedule) bool {
	return left.Prompt != right.Prompt || left.TargetType != right.TargetType || left.ConversationID != right.ConversationID || scheduleTimingChanged(left, right)
}

func scheduleTimingChanged(left, right schedulestore.Schedule) bool {
	return left.ScheduleType != right.ScheduleType || left.Cron != right.Cron || left.Timezone != right.Timezone || !sameTime(left.RunAt, right.RunAt)
}

func sameTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func newScheduleID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "schedule_" + hex.EncodeToString(value[:]), nil
}

func scheduleTargetKind(resource threadstore.Thread) string {
	if resource.ResourceKind() == threadstore.KindChat {
		return schedulestore.TargetChat
	}
	return schedulestore.TargetProject
}
