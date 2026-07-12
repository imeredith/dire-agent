package daemon

import (
	"errors"
	"strings"
	"time"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

func (c *serverClient) handle(command Command) Response {
	response := Response{ID: command.ID, Type: "response", Command: command.Type, Success: true}
	fail := func(err error) Response {
		response.Success = false
		response.Error = err.Error()
		return response
	}
	resourceID := command.ConversationID
	if resourceID == "" {
		resourceID = command.ChatID
	}
	if resourceID == "" {
		resourceID = command.ProjectID
	}
	if resourceID == "" {
		resourceID = command.ThreadID
	}

	var err error
	switch command.Type {
	case "create_chat":
		chatOptions := command.ChatOptions
		if chatOptions == (CreateChatOptions{}) {
			chatOptions = CreateChatOptions{
				Name: command.Options.Name, Model: command.Options.Model,
				Instructions: command.Options.Instructions, ThinkingLevel: command.Options.ThinkingLevel,
			}
		}
		var chat threadstore.Chat
		chat, err = c.manager.CreateChat(c.ctx, chatOptions)
		response.Data = chat
		if err == nil {
			err = c.subscribe(chat.ID)
		}
	case "create_project", "create_thread", "new_session":
		var thread threadstore.Thread
		thread, err = c.manager.CreateProject(c.ctx, command.Options)
		response.Data = thread
		if err == nil {
			err = c.subscribe(thread.ID)
		}
	case "list_projects", "list_threads":
		if command.Type == "list_threads" {
			response.Data, err = c.manager.ListConversations(c.ctx)
		} else {
			response.Data, err = c.manager.ListProjects(c.ctx)
		}
	case "list_chats":
		response.Data, err = c.manager.ListChats(c.ctx)
	case "list_conversations":
		response.Data, err = c.manager.ListConversations(c.ctx)
	case "get_project", "get_thread":
		if command.Type == "get_thread" {
			response.Data, err = c.manager.Thread(c.ctx, resourceID)
		} else {
			response.Data, err = c.manager.Project(c.ctx, resourceID)
		}
	case "get_chat":
		response.Data, err = c.manager.Chat(c.ctx, resourceID)
	case "get_conversation":
		response.Data, err = c.manager.Thread(c.ctx, resourceID)
	case "get_project_state", "get_chat_state", "get_conversation_state", "get_state":
		response.Data, err = c.manager.State(c.ctx, resourceID)
	case "get_project_messages", "get_chat_messages", "get_conversation_messages", "get_messages":
		response.Data, err = c.manager.Messages(c.ctx, resourceID, command.After, command.Limit)
	case "get_project_events", "get_chat_events", "get_conversation_events", "get_events":
		response.Data, err = c.manager.Events(c.ctx, resourceID, command.After, command.Limit)
	case "prompt", "steer", "follow_up":
		err = c.subscribe(resourceID)
		if err == nil {
			switch command.Type {
			case "prompt":
				err = c.manager.PromptWithAttachments(c.ctx, resourceID, command.Message, command.StreamingBehavior, command.Attachments)
			case "steer":
				if len(command.Attachments) != 0 {
					err = errors.New("daemon: images can only be attached to a new prompt")
					break
				}
				err = c.manager.Steer(c.ctx, resourceID, command.Message)
			case "follow_up":
				if len(command.Attachments) != 0 {
					err = errors.New("daemon: images can only be attached to a new prompt")
					break
				}
				err = c.manager.FollowUp(c.ctx, resourceID, command.Message)
			}
		}
	case "abort":
		err = c.manager.Abort(c.ctx, resourceID)
	case "subscribe_chat", "subscribe_conversation", "subscribe_project", "subscribe":
		err = c.subscribe(resourceID)
	case "unsubscribe_chat", "unsubscribe_conversation", "unsubscribe_project", "unsubscribe":
		c.unsubscribe(resourceID)
	case "set_model":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{Model: &command.Model})
	case "set_chat_name", "set_conversation_name", "set_project_name", "set_thread_name", "set_session_name":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{Name: &command.Name})
	case "set_project_category":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{Category: &command.Category})
	case "set_project_sandbox_folders":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{AdditionalFolders: &command.AdditionalFolders})
	case "set_thinking_level":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{ThinkingLevel: &command.Level})
	case "set_steering_mode":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{SteeringMode: &command.Mode})
	case "set_follow_up_mode":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{FollowUpMode: &command.Mode})
	case "set_tools":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{Tools: &command.Tools})
	case "get_available_tools":
		response.Data = map[string]any{"tools": c.manager.AvailableTools()}
	case "get_capabilities":
		response.Data, err = c.manager.CapabilityState(c.ctx, resourceID)
	case "list_capability_commands":
		response.Data, err = c.manager.CapabilityCommands(c.ctx, resourceID)
	case "execute_capability_command":
		response.Data, err = c.manager.ExecuteCapabilityCommand(c.ctx, resourceID, command.CommandName, command.Arguments)
	case "get_available_models":
		response.Data = map[string]any{"models": c.manager.AvailableModels()}
	case "get_project_launchers":
		_, response.Data, err = projectLaunchers(c.ctx, c.manager, c.config, resourceID)
	case "launch_project_app":
		var launcher configuration.ProjectLauncher
		launcher, err = launchProjectDesktopApplication(c.ctx, c.manager, c.config, resourceID, command.LauncherID)
		if err == nil {
			response.Data = map[string]any{"launched": true, "id": launcher.ID, "label": launcher.Label}
		}
	case "spawn_agent":
		parentID := firstNonEmpty(command.ParentID, resourceID)
		var spawned agentteam.Agent
		spawned, err = c.manager.SpawnAgent(c.ctx, agentteam.SpawnRequest{
			ParentID: parentID, Name: firstNonEmpty(command.AgentName, command.Name),
			Profile: command.Profile, Role: command.AgentRole,
			Task: firstNonEmpty(command.Task, command.Message), Model: command.Model,
			Thinking: command.Level, Tools: command.Tools,
		})
		response.Data = spawned
		if err == nil {
			err = c.subscribe(spawned.ID)
		}
	case "list_agents":
		response.Data, err = c.manager.ListAgents(c.ctx, firstNonEmpty(command.ParentID, resourceID))
	case "get_agent":
		response.Data, err = c.manager.Agent(c.ctx, firstNonEmpty(command.AgentID, resourceID))
	case "send_agent_message":
		wake := command.Wake == nil || *command.Wake
		response.Data, err = c.manager.SendAgentMessage(c.ctx, firstNonEmpty(command.ParentID, resourceID), command.AgentID, command.Message, wake)
	case "wait_agents":
		response.Data, err = c.manager.WaitAgents(c.ctx, firstNonEmpty(command.ParentID, resourceID), command.AgentIDs, time.Duration(command.TimeoutMS)*time.Millisecond)
	case "interrupt_agent":
		err = c.manager.InterruptAgent(c.ctx, firstNonEmpty(command.ParentID, resourceID), command.AgentID)
		response.Data = map[string]bool{"interrupted": err == nil}
	case "delete_agent":
		err = c.manager.DeleteAgent(c.ctx, firstNonEmpty(command.ParentID, resourceID), command.AgentID)
	case "get_commands":
		response.Data = map[string]any{"commands": supportedCommands}
	case "list_scheduled_prompts":
		response.Data, err = c.manager.ListScheduledPrompts(c.ctx, resourceID)
	case "create_scheduled_prompt":
		if command.Schedule == nil {
			err = errors.New("daemon: schedule is required")
			break
		}
		input := *command.Schedule
		if input.ConversationID == "" && resourceID != "" && !strings.EqualFold(strings.TrimSpace(input.TargetType), "one_off") {
			input.ConversationID = resourceID
		}
		if input.TargetType == "" && input.ConversationID != "" {
			var resource threadstore.Thread
			resource, err = c.manager.Thread(c.ctx, input.ConversationID)
			if err == nil {
				input.TargetType = scheduleTargetKind(resource)
			}
		}
		if err == nil {
			response.Data, err = c.manager.CreateScheduledPrompt(c.ctx, input)
		}
	case "update_scheduled_prompt":
		if command.Schedule == nil {
			err = errors.New("daemon: schedule is required")
			break
		}
		response.Data, err = c.manager.UpdateScheduledPrompt(c.ctx, command.ScheduleID, *command.Schedule)
	case "delete_scheduled_prompt":
		err = c.manager.DeleteScheduledPrompt(c.ctx, command.ScheduleID)
		response.Data = map[string]bool{"deleted": err == nil}
	case "run_scheduled_prompt":
		response.Data, err = c.manager.RunScheduledPrompt(c.ctx, command.ScheduleID)
	case "subscribe_scheduled_prompts":
		c.subscribeScheduledPrompts()
	case "unsubscribe_scheduled_prompts":
		c.unsubscribeScheduledPrompts()
	case "config_get", "config_effective", "config_validate", "config_update":
		response.Data, err = c.handleConfig(command, resourceID)
	case "delete_chat", "delete_project", "delete_conversation", "delete_thread":
		c.unsubscribe(resourceID)
		switch command.Type {
		case "delete_chat":
			err = c.manager.DeleteChat(c.ctx, resourceID)
		case "delete_project":
			err = c.manager.DeleteProject(c.ctx, resourceID)
		default:
			err = c.manager.DeleteThread(c.ctx, resourceID)
		}
	default:
		err = errors.New("daemon: unknown command " + command.Type)
	}
	if err != nil {
		return fail(err)
	}
	return response
}

func (c *serverClient) handleConfig(command Command, resourceID string) (any, error) {
	if command.Type != "config_validate" && c.config == nil {
		return nil, errors.New("daemon: configuration store is unavailable")
	}
	switch command.Type {
	case "config_get":
		return c.config.Load(c.ctx)
	case "config_effective":
		settings, found, err := c.config.Effective(c.ctx, resourceID)
		return map[string]any{"settings": settings, "project_override": found}, err
	case "config_validate":
		if command.Config == nil {
			return nil, errors.New("daemon: config is required")
		}
		err := configuration.Validate(*command.Config)
		return map[string]bool{"valid": err == nil}, err
	case "config_update":
		if command.Config == nil {
			return nil, errors.New("daemon: config is required")
		}
		revision := command.ExpectedRevision
		if revision == 0 {
			revision = command.Config.Revision
		}
		updated, err := c.config.Update(c.ctx, revision, *command.Config)
		if err == nil {
			_ = c.manager.RefreshCapabilities(c.ctx)
		}
		return updated, err
	default:
		return nil, errors.New("daemon: unknown config command " + command.Type)
	}
}
