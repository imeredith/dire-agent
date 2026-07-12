package daemon

import (
	"context"
	"errors"
	"time"
)

func (c *serverClient) subscribe(threadID string) error {
	if threadID == "" {
		return errors.New("daemon: conversation_id, chat_id, or project_id is required")
	}
	c.mu.Lock()
	if _, exists := c.subscriptions[threadID]; exists {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	events, unsubscribe, err := c.manager.Subscribe(c.ctx, threadID)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if existing := c.subscriptions[threadID]; existing != nil {
		c.mu.Unlock()
		unsubscribe()
		return nil
	}
	c.subscriptions[threadID] = unsubscribe
	c.mu.Unlock()
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				return
			case event := <-events:
				wire := WireEvent{
					Type: event.Type, Scope: event.Scope, ConversationID: event.ConversationID, ChatID: event.ChatID,
					ProjectID: event.ProjectID, ThreadID: event.ThreadID, Sequence: event.Sequence,
					Timestamp: event.Timestamp.Format(time.RFC3339Nano), Data: event.Data,
				}
				select {
				case c.outbound <- wire:
				case <-c.ctx.Done():
					return
				}
			}
		}
	}()
	return nil
}

func (c *serverClient) unsubscribe(threadID string) {
	c.mu.Lock()
	unsubscribe := c.subscriptions[threadID]
	delete(c.subscriptions, threadID)
	c.mu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
}

func (c *serverClient) closeSubscriptions() {
	c.mu.Lock()
	subscriptions := c.subscriptions
	c.subscriptions = make(map[string]func())
	scheduleSubscription := c.scheduleSubscription
	c.scheduleSubscription = nil
	c.mu.Unlock()
	for _, unsubscribe := range subscriptions {
		unsubscribe()
	}
	if scheduleSubscription != nil {
		scheduleSubscription()
	}
}

func (c *serverClient) subscribeScheduledPrompts() {
	c.mu.Lock()
	if c.scheduleSubscription != nil {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	events, unsubscribe := c.manager.SubscribeScheduledPrompts(c.ctx)
	forwardContext, stopForwarding := context.WithCancel(c.ctx)
	combinedUnsubscribe := func() {
		stopForwarding()
		unsubscribe()
	}
	c.mu.Lock()
	if c.scheduleSubscription != nil {
		c.mu.Unlock()
		combinedUnsubscribe()
		return
	}
	c.scheduleSubscription = combinedUnsubscribe
	c.mu.Unlock()
	go func() {
		for {
			select {
			case <-forwardContext.Done():
				return
			case event := <-events:
				wire := WireEvent{
					Type:      event.Type,
					Scope:     ConversationScope{Kind: "schedule", ID: event.ScheduleID},
					Timestamp: event.Timestamp.Format(time.RFC3339Nano), Data: event.Data,
				}
				select {
				case c.outbound <- wire:
				case <-forwardContext.Done():
					return
				}
			}
		}
	}()
}

func (c *serverClient) unsubscribeScheduledPrompts() {
	c.mu.Lock()
	unsubscribe := c.scheduleSubscription
	c.scheduleSubscription = nil
	c.mu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
}

var supportedCommands = []string{
	"create_chat", "list_chats", "get_chat", "get_chat_state", "get_chat_messages", "get_chat_events", "delete_chat",
	"list_conversations", "get_conversation", "get_conversation_state", "get_conversation_messages", "get_conversation_events", "delete_conversation",
	"subscribe_chat", "unsubscribe_chat", "subscribe_conversation", "unsubscribe_conversation",
	"create_project", "list_projects", "get_project", "get_project_state", "get_project_messages", "get_project_events",
	"subscribe_project", "unsubscribe_project", "set_project_name", "set_project_category", "set_project_sandbox_folders", "delete_project",
	"create_thread", "new_session", "list_threads", "get_thread", "get_state", "get_messages", "get_events",
	"prompt", "steer", "follow_up", "abort", "subscribe", "unsubscribe",
	"set_model", "set_thread_name", "set_session_name", "set_thinking_level", "set_steering_mode", "set_follow_up_mode", "set_tools",
	"set_chat_name", "set_conversation_name", "get_available_tools", "get_available_models", "get_commands", "delete_thread",
	"get_project_launchers", "launch_project_app",
	"get_capabilities", "config_get", "config_effective", "config_validate", "config_update",
	"spawn_agent", "list_agents", "get_agent", "send_agent_message", "wait_agents", "interrupt_agent", "delete_agent",
	"list_capability_commands", "execute_capability_command",
	"list_scheduled_prompts", "create_scheduled_prompt", "update_scheduled_prompt", "delete_scheduled_prompt", "run_scheduled_prompt",
	"subscribe_scheduled_prompts", "unsubscribe_scheduled_prompts",
}
