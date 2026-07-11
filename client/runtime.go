package client

import (
	"context"

	"github.com/imeredith/dire-agent/daemon"
	"github.com/imeredith/dire-agent/threadstore"
)

func (c *Client) Prompt(ctx context.Context, threadID, message, streamingBehavior string) error {
	return c.call(ctx, daemon.Command{Type: "prompt", ProjectID: threadID, ThreadID: threadID, Message: message, StreamingBehavior: streamingBehavior}, nil)
}

func (c *Client) ProjectPrompt(ctx context.Context, projectID, message, streamingBehavior string) error {
	return c.Prompt(ctx, projectID, message, streamingBehavior)
}

func (c *Client) Steer(ctx context.Context, threadID, message string) error {
	return c.call(ctx, daemon.Command{Type: "steer", ProjectID: threadID, ThreadID: threadID, Message: message}, nil)
}

func (c *Client) ProjectSteer(ctx context.Context, projectID, message string) error {
	return c.Steer(ctx, projectID, message)
}

func (c *Client) FollowUp(ctx context.Context, threadID, message string) error {
	return c.call(ctx, daemon.Command{Type: "follow_up", ProjectID: threadID, ThreadID: threadID, Message: message}, nil)
}

func (c *Client) ProjectFollowUp(ctx context.Context, projectID, message string) error {
	return c.FollowUp(ctx, projectID, message)
}

func (c *Client) Abort(ctx context.Context, threadID string) error {
	return c.call(ctx, daemon.Command{Type: "abort", ProjectID: threadID, ThreadID: threadID}, nil)
}

func (c *Client) AbortProject(ctx context.Context, projectID string) error {
	return c.Abort(ctx, projectID)
}

func (c *Client) Subscribe(ctx context.Context, threadID string) error {
	return c.call(ctx, daemon.Command{Type: "subscribe", ProjectID: threadID, ThreadID: threadID}, nil)
}

func (c *Client) SubscribeProject(ctx context.Context, projectID string) error {
	return c.call(ctx, daemon.Command{Type: "subscribe_project", ProjectID: projectID}, nil)
}

func (c *Client) Unsubscribe(ctx context.Context, threadID string) error {
	return c.call(ctx, daemon.Command{Type: "unsubscribe", ProjectID: threadID, ThreadID: threadID}, nil)
}

func (c *Client) UnsubscribeProject(ctx context.Context, projectID string) error {
	return c.call(ctx, daemon.Command{Type: "unsubscribe_project", ProjectID: projectID}, nil)
}

func (c *Client) SetModel(ctx context.Context, threadID, model string) (threadstore.Thread, error) {
	var thread threadstore.Thread
	err := c.call(ctx, daemon.Command{Type: "set_model", ProjectID: threadID, ThreadID: threadID, Model: model}, &thread)
	return thread, err
}

func (c *Client) SetProjectModel(ctx context.Context, projectID, model string) (threadstore.Project, error) {
	return c.SetModel(ctx, projectID, model)
}

func (c *Client) SetThreadName(ctx context.Context, threadID, name string) (threadstore.Thread, error) {
	var thread threadstore.Thread
	err := c.call(ctx, daemon.Command{Type: "set_thread_name", ProjectID: threadID, ThreadID: threadID, Name: name}, &thread)
	return thread, err
}

func (c *Client) SetProjectName(ctx context.Context, projectID, name string) (threadstore.Project, error) {
	var project threadstore.Project
	err := c.call(ctx, daemon.Command{Type: "set_project_name", ProjectID: projectID, Name: name}, &project)
	return project, err
}

func (c *Client) SetProjectCategory(ctx context.Context, projectID, category string) (threadstore.Project, error) {
	var project threadstore.Project
	err := c.call(ctx, daemon.Command{Type: "set_project_category", ProjectID: projectID, Category: category}, &project)
	return project, err
}

func (c *Client) SetProjectAdditionalFolders(ctx context.Context, projectID string, folders []string) (threadstore.Project, error) {
	var project threadstore.Project
	err := c.call(ctx, daemon.Command{
		Type: "set_project_sandbox_folders", ProjectID: projectID,
		AdditionalFolders: append([]string(nil), folders...),
	}, &project)
	return project, err
}

func (c *Client) SetThinkingLevel(ctx context.Context, threadID, level string) (threadstore.Thread, error) {
	var thread threadstore.Thread
	err := c.call(ctx, daemon.Command{Type: "set_thinking_level", ProjectID: threadID, ThreadID: threadID, Level: level}, &thread)
	return thread, err
}

func (c *Client) SetProjectThinkingLevel(ctx context.Context, projectID, level string) (threadstore.Project, error) {
	return c.SetThinkingLevel(ctx, projectID, level)
}

func (c *Client) SetSteeringMode(ctx context.Context, threadID, mode string) (threadstore.Thread, error) {
	var thread threadstore.Thread
	err := c.call(ctx, daemon.Command{Type: "set_steering_mode", ProjectID: threadID, ThreadID: threadID, Mode: mode}, &thread)
	return thread, err
}

func (c *Client) SetProjectSteeringMode(ctx context.Context, projectID, mode string) (threadstore.Project, error) {
	return c.SetSteeringMode(ctx, projectID, mode)
}

func (c *Client) SetFollowUpMode(ctx context.Context, threadID, mode string) (threadstore.Thread, error) {
	var thread threadstore.Thread
	err := c.call(ctx, daemon.Command{Type: "set_follow_up_mode", ProjectID: threadID, ThreadID: threadID, Mode: mode}, &thread)
	return thread, err
}

func (c *Client) SetProjectFollowUpMode(ctx context.Context, projectID, mode string) (threadstore.Project, error) {
	return c.SetFollowUpMode(ctx, projectID, mode)
}

func (c *Client) SetTools(ctx context.Context, threadID string, tools []string) (threadstore.Thread, error) {
	var thread threadstore.Thread
	err := c.call(ctx, daemon.Command{Type: "set_tools", ProjectID: threadID, ThreadID: threadID, Tools: tools}, &thread)
	return thread, err
}

func (c *Client) SetProjectTools(ctx context.Context, projectID string, tools []string) (threadstore.Project, error) {
	return c.SetTools(ctx, projectID, tools)
}

func (c *Client) AvailableTools(ctx context.Context) ([]string, error) {
	var result struct {
		Tools []string `json:"tools"`
	}
	err := c.call(ctx, daemon.Command{Type: "get_available_tools"}, &result)
	return result.Tools, err
}

func (c *Client) AvailableModels(ctx context.Context) ([]daemon.ModelInfo, error) {
	var result struct {
		Models []daemon.ModelInfo `json:"models"`
	}
	err := c.call(ctx, daemon.Command{Type: "get_available_models"}, &result)
	return result.Models, err
}

func (c *Client) Commands(ctx context.Context) ([]string, error) {
	var result struct {
		Commands []string `json:"commands"`
	}
	err := c.call(ctx, daemon.Command{Type: "get_commands"}, &result)
	return result.Commands, err
}

// WaitForSettled waits for the Pi-style agent_settled event for a conversation.
func (c *Client) WaitForSettled(ctx context.Context, threadID string) (daemon.WireEvent, error) {
	for {
		select {
		case <-ctx.Done():
			return daemon.WireEvent{}, ctx.Err()
		case <-c.done:
			return daemon.WireEvent{}, c.Err()
		case event, ok := <-c.events:
			if !ok {
				return daemon.WireEvent{}, c.Err()
			}
			if (event.ConversationID == threadID || event.ChatID == threadID || event.ProjectID == threadID || event.ThreadID == threadID) && event.Type == "agent_settled" {
				return event, nil
			}
		}
	}
}
