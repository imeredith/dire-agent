package client

import (
	"context"

	"github.com/dire-kiwi/dire-agent/daemon"
)

func (c *Client) ListScheduledPrompts(ctx context.Context, conversationID string) ([]daemon.ScheduledPrompt, error) {
	var schedules []daemon.ScheduledPrompt
	command := daemon.Command{Type: "list_scheduled_prompts"}
	if conversationID != "" {
		command.ConversationID = conversationID
		command.ThreadID = conversationID
	}
	err := c.call(ctx, command, &schedules)
	return schedules, err
}

func (c *Client) CreateScheduledPrompt(ctx context.Context, input daemon.ScheduledPromptInput) (daemon.ScheduledPrompt, error) {
	var schedule daemon.ScheduledPrompt
	command := daemon.Command{Type: "create_scheduled_prompt", Schedule: &input}
	if input.ConversationID != "" {
		command.ConversationID = input.ConversationID
		command.ThreadID = input.ConversationID
	}
	err := c.call(ctx, command, &schedule)
	return schedule, err
}

func (c *Client) UpdateScheduledPrompt(ctx context.Context, id string, input daemon.ScheduledPromptInput) (daemon.ScheduledPrompt, error) {
	var schedule daemon.ScheduledPrompt
	err := c.call(ctx, daemon.Command{Type: "update_scheduled_prompt", ScheduleID: id, Schedule: &input}, &schedule)
	return schedule, err
}

func (c *Client) DeleteScheduledPrompt(ctx context.Context, id string) error {
	return c.call(ctx, daemon.Command{Type: "delete_scheduled_prompt", ScheduleID: id}, nil)
}

func (c *Client) RunScheduledPrompt(ctx context.Context, id string) (daemon.ScheduledPrompt, error) {
	var schedule daemon.ScheduledPrompt
	err := c.call(ctx, daemon.Command{Type: "run_scheduled_prompt", ScheduleID: id}, &schedule)
	return schedule, err
}

func (c *Client) SubscribeScheduledPrompts(ctx context.Context) error {
	return c.call(ctx, daemon.Command{Type: "subscribe_scheduled_prompts"}, nil)
}

func (c *Client) UnsubscribeScheduledPrompts(ctx context.Context) error {
	return c.call(ctx, daemon.Command{Type: "unsubscribe_scheduled_prompts"}, nil)
}
