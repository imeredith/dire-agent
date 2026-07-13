package daemon

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/agentloop"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func (m *Manager) Subscribe(ctx context.Context, id string) (<-chan Event, func(), error) {
	if _, err := m.getRuntime(ctx, id); err != nil {
		return nil, nil, err
	}
	channel := make(chan Event, 1024)
	subID := m.nextSubID.Add(1)
	m.subMu.Lock()
	if m.subscribers[id] == nil {
		m.subscribers[id] = make(map[uint64]chan Event)
	}
	m.subscribers[id][subID] = channel
	m.subMu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			m.subMu.Lock()
			delete(m.subscribers[id], subID)
			m.subMu.Unlock()
		})
	}
	return channel, cancel, nil
}

func (m *Manager) handleLoopEvent(runtime *threadRuntime, event agentloop.Event) {
	if event.Type == "tool_execution_end" || event.Type == "reasoning_end" {
		data, _ := json.Marshal(event)
		kind, role, content := "tool", "tool", event.Output
		if event.Type == "reasoning_end" {
			kind, role, content = "reasoning", "reasoning", event.Text
		}
		_, _ = runtime.db.AppendMessage(context.Background(), threadstore.Message{
			Kind: kind, Role: role, Content: content, Data: data,
		})
	}
	m.emit(context.Background(), runtime, event.Type, event)
	if event.Type == "message_end" && event.Usage != nil && usagePresent(*event.Usage) {
		runtime.mu.Lock()
		runtime.thread.Usage = accumulateUsage(runtime.thread.Usage, *event.Usage)
		usage := runtime.thread.Usage
		runtime.mu.Unlock()
		_ = runtime.persistThread(context.Background())
		m.emit(context.Background(), runtime, "usage_update", usage)
	}
}

func (m *Manager) emitResourceUpdated(runtime *threadRuntime, resource threadstore.Thread) {
	if resource.ResourceKind() == threadstore.KindChat {
		m.emit(context.Background(), runtime, "chat_updated", resource)
		m.emit(context.Background(), runtime, "conversation_updated", resource)
		return
	}
	m.emit(context.Background(), runtime, "project_updated", resource)
	m.emit(context.Background(), runtime, "thread_updated", resource)
}

func (m *Manager) emit(ctx context.Context, runtime *threadRuntime, eventType string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	stored, err := runtime.db.AppendEvent(ctx, threadstore.Event{Type: eventType, Data: raw})
	if err != nil {
		return
	}
	resource := runtime.snapshotThread()
	event := Event{
		Type: eventType, ConversationID: resource.ID, ThreadID: resource.ID,
		Scope:    ConversationScope{Kind: resource.ResourceKind(), ID: resource.ID},
		Sequence: stored.Sequence, Timestamp: stored.CreatedAt, Data: raw,
	}
	if resource.ResourceKind() == threadstore.KindChat {
		event.ChatID = resource.ID
	} else {
		event.ProjectID = resource.ID
	}
	m.subMu.Lock()
	channels := make([]chan Event, 0, len(m.subscribers[resource.ID]))
	for _, channel := range m.subscribers[resource.ID] {
		channels = append(channels, channel)
	}
	m.subMu.Unlock()
	for _, channel := range channels {
		select {
		case channel <- event:
		default:
			// Events remain available in SQLite for catch-up through get_events.
		}
	}
}

func usagePresent(usage agent.Usage) bool {
	return usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.CacheReadTokens != 0 ||
		usage.CacheWriteTokens != 0 || usage.TotalTokens != 0 || usage.ContextTokens != 0 || usage.ContextWindow != 0
}

func accumulateUsage(total, current agent.Usage) agent.Usage {
	total.InputTokens += current.InputTokens
	total.OutputTokens += current.OutputTokens
	total.CacheReadTokens += current.CacheReadTokens
	total.CacheWriteTokens += current.CacheWriteTokens
	currentTotal := current.TotalTokens
	if currentTotal == 0 {
		currentTotal = current.InputTokens + current.OutputTokens
	}
	total.TotalTokens += currentTotal
	contextTokens := current.ContextTokens
	if contextTokens == 0 {
		contextTokens = current.InputTokens + current.OutputTokens
	}
	if contextTokens != 0 {
		total.ContextTokens = contextTokens
	}
	if current.ContextWindow != 0 {
		total.ContextWindow = current.ContextWindow
	}
	return total
}
