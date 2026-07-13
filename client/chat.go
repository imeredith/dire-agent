package client

import (
	"context"

	"github.com/dire-kiwi/dire-agent/daemon"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func (c *Client) CreateChat(ctx context.Context, options daemon.CreateChatOptions) (threadstore.Chat, error) {
	var chat threadstore.Chat
	err := c.call(ctx, daemon.Command{Type: "create_chat", ChatOptions: options}, &chat)
	return chat, err
}

func (c *Client) ListChats(ctx context.Context) ([]threadstore.Chat, error) {
	var chats []threadstore.Chat
	err := c.call(ctx, daemon.Command{Type: "list_chats"}, &chats)
	return chats, err
}

func (c *Client) ListConversations(ctx context.Context) ([]threadstore.Conversation, error) {
	var conversations []threadstore.Conversation
	err := c.call(ctx, daemon.Command{Type: "list_conversations"}, &conversations)
	return conversations, err
}

func (c *Client) Chat(ctx context.Context, chatID string) (threadstore.Chat, error) {
	var chat threadstore.Chat
	err := c.call(ctx, chatCommand("get_chat", chatID), &chat)
	return chat, err
}

func (c *Client) Conversation(ctx context.Context, id string) (threadstore.Conversation, error) {
	var conversation threadstore.Conversation
	err := c.call(ctx, conversationCommand("get_conversation", id), &conversation)
	return conversation, err
}

func (c *Client) ChatState(ctx context.Context, chatID string) (daemon.RuntimeState, error) {
	var state daemon.RuntimeState
	err := c.call(ctx, chatCommand("get_chat_state", chatID), &state)
	return state, err
}

func (c *Client) ChatMessages(ctx context.Context, chatID string, after int64, limit int) ([]threadstore.Message, error) {
	var messages []threadstore.Message
	command := chatCommand("get_chat_messages", chatID)
	command.After, command.Limit = after, limit
	err := c.call(ctx, command, &messages)
	return messages, err
}

func (c *Client) ChatEvents(ctx context.Context, chatID string, after int64, limit int) ([]threadstore.Event, error) {
	var events []threadstore.Event
	command := chatCommand("get_chat_events", chatID)
	command.After, command.Limit = after, limit
	err := c.call(ctx, command, &events)
	return events, err
}

func (c *Client) ChatPrompt(ctx context.Context, chatID, message, streamingBehavior string) error {
	command := chatCommand("prompt", chatID)
	command.Message, command.StreamingBehavior = message, streamingBehavior
	return c.call(ctx, command, nil)
}

func (c *Client) SubscribeChat(ctx context.Context, chatID string) error {
	return c.call(ctx, chatCommand("subscribe_chat", chatID), nil)
}

func (c *Client) UnsubscribeChat(ctx context.Context, chatID string) error {
	return c.call(ctx, chatCommand("unsubscribe_chat", chatID), nil)
}

func (c *Client) SetChatName(ctx context.Context, chatID, name string) (threadstore.Chat, error) {
	var chat threadstore.Chat
	command := chatCommand("set_chat_name", chatID)
	command.Name = name
	err := c.call(ctx, command, &chat)
	return chat, err
}

func (c *Client) DeleteChat(ctx context.Context, chatID string) error {
	return c.call(ctx, chatCommand("delete_chat", chatID), nil)
}

func chatCommand(commandType, chatID string) daemon.Command {
	return daemon.Command{Type: commandType, ConversationID: chatID, ChatID: chatID, ThreadID: chatID}
}

func conversationCommand(commandType, id string) daemon.Command {
	return daemon.Command{Type: commandType, ConversationID: id, ThreadID: id}
}
