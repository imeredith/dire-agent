package daemon

import (
	"errors"
	"time"

	"github.com/dire-kiwi/dire-agent/agentteam"
	"github.com/dire-kiwi/dire-agent/configuration"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

// ProjectSandboxSettings reports the global default, the effective value for
// one project, and the optional project-specific override.
type ProjectSandboxSettings struct {
	Global    configuration.SandboxMode  `json:"global"`
	Effective configuration.SandboxMode  `json:"effective"`
	Override  *configuration.SandboxMode `json:"override,omitempty"`
}

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
		if chatOptions.Name == "" && chatOptions.Model == "" && chatOptions.Instructions == "" &&
			chatOptions.ThinkingLevel == "" && len(chatOptions.MCPServerOverrides) == 0 {
			chatOptions = CreateChatOptions{
				Name: command.Options.Name, Model: command.Options.Model,
				Instructions: command.Options.Instructions, ThinkingLevel: command.Options.ThinkingLevel,
				MCPServerOverrides: cloneBoolMap(command.Options.MCPServerOverrides),
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
	case "get_project_sandbox", "set_project_sandbox":
		response.Data, err = c.handleProjectSandbox(command.Type, resourceID, command.Sandbox)
	case "set_thinking_level":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{ThinkingLevel: &command.Level})
	case "set_steering_mode":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{SteeringMode: &command.Mode})
	case "set_follow_up_mode":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{FollowUpMode: &command.Mode})
	case "set_tools":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{Tools: &command.Tools})
	case "set_mcp_server_enabled":
		response.Data, err = c.manager.UpdateSettings(c.ctx, resourceID, SettingsUpdate{MCPServer: &MCPServerUpdate{
			Name: command.MCPServer, Enabled: command.Enabled,
		}})
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
	case "inspect_project_workspace":
		response.Data, err = c.manager.InspectProjectWorkspace(c.ctx, resourceID, command.Folder)
	case "get_project_environments":
		response.Data, err = c.manager.ProjectEnvironments(c.ctx, resourceID, command.Folder)
	case "put_project_environment":
		if command.Environment == nil {
			err = errors.New("daemon: environment is required")
			break
		}
		environment := *command.Environment
		if command.EnvironmentID != "" {
			if environment.ID != "" && environment.ID != command.EnvironmentID {
				err = errors.New("daemon: environment_id does not match environment.id")
				break
			}
			environment.ID = command.EnvironmentID
		}
		response.Data, err = c.manager.PutProjectEnvironment(c.ctx, resourceID, command.Folder, environment, command.ExpectedHash)
	case "delete_project_environment":
		err = c.manager.DeleteProjectEnvironment(c.ctx, resourceID, command.Folder, command.EnvironmentID, command.ExpectedHash)
		if err == nil {
			response.Data = map[string]any{"deleted": true, "id": command.EnvironmentID}
		}
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

func (c *serverClient) handleProjectSandbox(commandType, resourceID, requested string) (ProjectSandboxSettings, error) {
	if c.config == nil {
		return ProjectSandboxSettings{}, errors.New("daemon: configuration store is unavailable")
	}
	scopeID, folder, err := c.projectSandboxTarget(resourceID)
	if err != nil {
		return ProjectSandboxSettings{}, err
	}
	if commandType == "set_project_sandbox" {
		mode, modeErr := projectSandboxOverride(requested)
		if modeErr != nil {
			return ProjectSandboxSettings{}, modeErr
		}
		if _, err = c.config.SetProjectSandbox(c.ctx, scopeID, folder, mode); err != nil {
			return ProjectSandboxSettings{}, err
		}
		// Settings are read as each capability snapshot is built. Refresh idle
		// conversations so the new policy applies before their next tool call.
		_ = c.manager.RefreshCapabilities(c.ctx)
	}

	config, err := c.config.Load(c.ctx)
	if err != nil {
		return ProjectSandboxSettings{}, err
	}
	state := ProjectSandboxSettings{Global: config.Global.Tools.Sandbox, Effective: config.Global.Tools.Sandbox}
	if project, exists := config.Projects[scopeID]; exists && project.Settings.Tools != nil && project.Settings.Tools.Sandbox != nil {
		override := *project.Settings.Tools.Sandbox
		state.Override = &override
		state.Effective = override
	}
	return state, nil
}

func (c *serverClient) projectSandboxTarget(resourceID string) (scopeID, folder string, err error) {
	resource, err := c.manager.Project(c.ctx, resourceID)
	if err != nil {
		return "", "", err
	}
	if resource.IsSubagent() {
		return "", "", errors.New("daemon: sandbox policy is only available for top-level projects")
	}
	scopeID = configScopeID(resource)
	folder = resource.CWD
	if scopeID != resource.ID {
		if source, sourceErr := c.manager.Project(c.ctx, scopeID); sourceErr == nil {
			folder = source.CWD
		}
	}
	return scopeID, folder, nil
}

func projectSandboxOverride(value string) (*configuration.SandboxMode, error) {
	if value == "inherit" {
		return nil, nil
	}
	mode := configuration.SandboxMode(value)
	switch mode {
	case configuration.SandboxStrict, configuration.SandboxWorkspace, configuration.SandboxOff:
		return &mode, nil
	default:
		return nil, errors.New("daemon: sandbox mode must be inherit, strict, workspace, or off")
	}
}

func (c *serverClient) handleConfig(command Command, resourceID string) (any, error) {
	if command.Type != "config_validate" && c.config == nil {
		return nil, errors.New("daemon: configuration store is unavailable")
	}
	switch command.Type {
	case "config_get":
		return c.config.Load(c.ctx)
	case "config_effective":
		scopeID := resourceID
		if resource, resourceErr := c.manager.Thread(c.ctx, resourceID); resourceErr == nil {
			scopeID = configScopeID(resource)
		}
		settings, found, err := c.config.Effective(c.ctx, scopeID)
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
