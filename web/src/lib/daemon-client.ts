import type {
  CapabilityState,
  CapabilityCommandInfo,
  CapabilityCommandResult,
  Chat,
  Conversation,
  CreateChatOptions,
  CreateProjectOptions,
  DaemonConfig,
  ModelInfo,
  Project,
  ProjectLauncher,
  RuntimeState,
  ScheduledPrompt,
  ScheduledPromptInput,
  StoredEvent,
  StoredMessage,
  SpawnAgentOptions,
  SubagentDetail,
  SubagentMessage,
  SubagentWaitResult,
  WireSubagentInfo,
} from "./protocol";
import { conversationKind, conversationScope } from "./protocol";
import { DaemonTransport } from "./daemon-transport";

export class DaemonClient extends DaemonTransport {
  async listProjects(): Promise<Project[]> {
    try {
      return (await this.request<Project[] | null>({ type: "list_projects" })) ?? [];
    } catch (error) {
      if (!unsupported(error)) throw error;
      const all = (await this.request<Project[] | null>({ type: "list_threads" })) ?? [];
      return all.filter((item) => conversationKind(item) === "project");
    }
  }

  async listChats(): Promise<Chat[]> {
    try {
      return (await this.request<Chat[] | null>({ type: "list_chats" })) ?? [];
    } catch (error) {
      if (!unsupported(error)) throw error;
      return [];
    }
  }

  createProject(options: CreateProjectOptions): Promise<Project> {
    return this.request<Project>({ type: "create_project", options });
  }

  createChat(options: CreateChatOptions): Promise<Chat> {
    return this.request<Chat>({ type: "create_chat", chat_options: options });
  }

  getConversation(conversation: Conversation): Promise<Conversation> {
    return this.request<Conversation>({
      type: conversationKind(conversation) === "chat" ? "get_chat" : "get_project",
      ...conversationScope(conversation),
    });
  }

  getState(conversation: Conversation): Promise<RuntimeState> {
    return this.request<RuntimeState>({
      type: "get_state",
      ...conversationScope(conversation),
    });
  }

  getMessages(conversation: Conversation, after = 0, limit = 10_000): Promise<StoredMessage[]> {
    return this.request<StoredMessage[] | null>({
      type: "get_messages",
      ...conversationScope(conversation),
      after,
      limit,
    }).then((value) => value ?? []);
  }

  getEvents(conversation: Conversation, after = 0, limit = 10_000): Promise<StoredEvent[]> {
    return this.request<StoredEvent[] | null>({
      type: "get_events",
      ...conversationScope(conversation),
      after,
      limit,
    }).then((value) => value ?? []);
  }

  subscribe(conversation: Conversation): Promise<void> {
    return this.request<void>({ type: "subscribe", ...conversationScope(conversation) });
  }

  unsubscribe(conversation: Conversation): Promise<void> {
    return this.request<void>({ type: "unsubscribe", ...conversationScope(conversation) });
  }

  deleteConversation(conversation: Conversation): Promise<void> {
    return this.request<void>({
      type: conversationKind(conversation) === "chat" ? "delete_chat" : "delete_project",
      ...conversationScope(conversation),
    });
  }

  getCapabilities(conversation: Conversation): Promise<CapabilityState> {
    return this.request<CapabilityState>({
      type: "get_capabilities",
      ...conversationScope(conversation),
    });
  }

  listCapabilityCommands(conversation: Conversation): Promise<CapabilityCommandInfo[]> {
    return this.request<CapabilityCommandInfo[] | null>({
      type: "list_capability_commands",
      ...conversationScope(conversation),
    }).then((value) => value ?? []);
  }

  executeCapabilityCommand(
    conversation: Conversation,
    commandName: string,
    commandArguments: string,
  ): Promise<CapabilityCommandResult> {
    return this.request<CapabilityCommandResult>({
      type: "execute_capability_command",
      ...conversationScope(conversation),
      command_name: commandName,
      arguments: commandArguments,
    });
  }

  getAvailableTools(): Promise<string[]> {
    return this.request<{ tools?: string[] | null }>({ type: "get_available_tools" })
      .then((value) => value.tools ?? []);
  }

  getAvailableModels(): Promise<ModelInfo[]> {
    return this.request<{ models?: ModelInfo[] | null }>({ type: "get_available_models" })
      .then((value) => value.models ?? []);
  }

  getProjectLaunchers(project: Conversation): Promise<ProjectLauncher[]> {
    return this.request<ProjectLauncher[] | null>({
      type: "get_project_launchers",
      ...conversationScope(project),
    }).then((value) => value ?? []);
  }

  launchProjectApp(project: Conversation, launcherID: string): Promise<{ launched: boolean; id: string; label?: string }> {
    return this.request<{ launched: boolean; id: string; label?: string }>({
      type: "launch_project_app",
      ...conversationScope(project),
      launcher_id: launcherID,
    });
  }

  getConfig(): Promise<DaemonConfig> {
    return this.request<DaemonConfig>({ type: "config_get" });
  }

  validateConfig(config: DaemonConfig): Promise<{ valid: boolean }> {
    return this.request<{ valid: boolean }>({ type: "config_validate", config });
  }

  updateConfig(config: DaemonConfig, expectedRevision: number): Promise<DaemonConfig> {
    return this.request<DaemonConfig>({
      type: "config_update",
      config,
      expected_revision: expectedRevision,
    });
  }

  listScheduledPrompts(): Promise<ScheduledPrompt[]> {
    return this.request<ScheduledPrompt[] | null>({ type: "list_scheduled_prompts" })
      .then((value) => value ?? []);
  }

  createScheduledPrompt(schedule: ScheduledPromptInput): Promise<ScheduledPrompt> {
    return this.request<ScheduledPrompt>({ type: "create_scheduled_prompt", schedule });
  }

  updateScheduledPrompt(scheduleID: string, schedule: ScheduledPromptInput): Promise<ScheduledPrompt> {
    return this.request<ScheduledPrompt>({
      type: "update_scheduled_prompt",
      schedule_id: scheduleID,
      schedule,
    });
  }

  deleteScheduledPrompt(scheduleID: string): Promise<void> {
    return this.request<void>({ type: "delete_scheduled_prompt", schedule_id: scheduleID });
  }

  runScheduledPrompt(scheduleID: string): Promise<ScheduledPrompt | undefined> {
    return this.request<ScheduledPrompt | undefined>({ type: "run_scheduled_prompt", schedule_id: scheduleID });
  }

  subscribeScheduledPrompts(): Promise<void> {
    return this.request<void>({ type: "subscribe_scheduled_prompts" });
  }

  unsubscribeScheduledPrompts(): Promise<void> {
    return this.request<void>({ type: "unsubscribe_scheduled_prompts" });
  }

  async listAgents(conversation: Conversation): Promise<WireSubagentInfo[]> {
    const value = await this.request<WireSubagentInfo[] | { agents?: WireSubagentInfo[] }>({
      type: "list_agents",
      ...conversationScope(conversation),
      parent_id: conversation.id,
    });
    return Array.isArray(value) ? value : value.agents ?? [];
  }

  spawnAgent(conversation: Conversation, options: SpawnAgentOptions): Promise<WireSubagentInfo> {
    return this.request<WireSubagentInfo>({
      type: "spawn_agent",
      ...conversationScope(conversation),
      ...options,
      parent_id: options.parent_id || conversation.id,
    });
  }

  getAgent(conversation: Conversation, agentID: string): Promise<SubagentDetail | WireSubagentInfo> {
    return this.request<SubagentDetail | WireSubagentInfo>({
      type: "get_agent",
      ...conversationScope(conversation),
      agent_id: agentID,
    });
  }

  getAgentMessages(agentID: string): Promise<StoredMessage[]> {
    return this.request<StoredMessage[] | null>({
      type: "get_messages",
      conversation_id: agentID,
      thread_id: agentID,
      after: 0,
      limit: 10_000,
    }).then((value) => value ?? []);
  }

  sendAgentMessage(conversation: Conversation, agentID: string, message: string, wake = true): Promise<SubagentMessage> {
    return this.request<SubagentMessage>({
      type: "send_agent_message",
      ...conversationScope(conversation),
      parent_id: conversation.id,
      agent_id: agentID,
      message,
      wake,
    });
  }

  interruptAgent(conversation: Conversation, agentID: string): Promise<void> {
    return this.request<void>({
      type: "interrupt_agent",
      ...conversationScope(conversation),
      parent_id: conversation.id,
      agent_id: agentID,
    });
  }

  deleteAgent(conversation: Conversation, agentID: string): Promise<void> {
    return this.request<void>({
      type: "delete_agent",
      ...conversationScope(conversation),
      parent_id: conversation.id,
      agent_id: agentID,
    });
  }

  waitAgents(conversation: Conversation, agentIDs: string[] = [], timeoutMs = 30_000): Promise<SubagentWaitResult> {
    return this.request<SubagentWaitResult>({
      type: "wait_agents",
      ...conversationScope(conversation),
      parent_id: conversation.id,
      agent_ids: agentIDs,
      timeout_ms: timeoutMs,
    }, timeoutMs + 5_000);
  }
}

export function unsupported(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /unknown command|unsupported|not supported/i.test(message);
}
