package daemon

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// ScheduleEvent is a daemon-global scheduler lifecycle event. Unlike
// conversation events it is not written into a conversation history.
type ScheduleEvent struct {
	Type       string          `json:"type"`
	ScheduleID string          `json:"schedule_id"`
	Timestamp  time.Time       `json:"timestamp"`
	Data       json.RawMessage `json:"data,omitempty"`
}

func (m *Manager) SubscribeScheduledPrompts(_ context.Context) (<-chan ScheduleEvent, func()) {
	channel := make(chan ScheduleEvent, 256)
	subID := m.nextSubID.Add(1)
	m.scheduleSubMu.Lock()
	m.scheduleSubscribers[subID] = channel
	m.scheduleSubMu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			m.scheduleSubMu.Lock()
			delete(m.scheduleSubscribers, subID)
			m.scheduleSubMu.Unlock()
		})
	}
	return channel, cancel
}

func (m *Manager) emitScheduleEvent(eventType string, schedule ScheduledPrompt) {
	raw, err := json.Marshal(schedule)
	if err != nil {
		return
	}
	event := ScheduleEvent{
		Type: eventType, ScheduleID: schedule.ID, Timestamp: time.Now().UTC(), Data: raw,
	}
	m.scheduleSubMu.Lock()
	channels := make([]chan ScheduleEvent, 0, len(m.scheduleSubscribers))
	for _, channel := range m.scheduleSubscribers {
		channels = append(channels, channel)
	}
	m.scheduleSubMu.Unlock()
	for _, channel := range channels {
		select {
		case channel <- event:
		default:
			// Scheduler state is always recoverable through list_scheduled_prompts.
		}
	}
}
