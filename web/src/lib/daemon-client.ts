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
  ProjectEnvironment,
  ProjectLauncher,
  ProjectSandboxSettings,
  ProjectWorkspaceInspection,
  RuntimeState,
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
    // Setup itself may consume ten minutes; leave room for checkout and
    // publication so the UI cannot time out while the daemon is still finishing.
    return this.request<Project>({ type: "create_project", options }, 15 * 60_000);
  }

  inspectProjectWorkspace(folder: string): Promise<ProjectWorkspaceInspection> {
    return this.request<ProjectWorkspaceInspection>({ type: "inspect_project_workspace", folder })
      .then((value) => ({
        ...value,
        branches: value.branches ?? [],
        environments: (value.environments ?? []).map(normalizeProjectEnvironment),
      }));
  }

  async getProjectEnvironments(scope: string | Conversation): Promise<ProjectEnvironment[]> {
    const value = await this.request<ProjectEnvironment[] | { environments?: ProjectEnvironment[] | null }>({
      type: "get_project_environments",
      ...projectEnvironmentScope(scope),
    });
    return (Array.isArray(value) ? value : value.environments ?? []).map(normalizeProjectEnvironment);
  }

  putProjectEnvironment(
    scope: string | Conversation,
    environment: ProjectEnvironment,
    expectedHash?: string,
  ): Promise<ProjectEnvironment> {
    return this.request<ProjectEnvironment>({
      type: "put_project_environment",
      ...projectEnvironmentScope(scope),
      environment_id: environment.id,
      environment,
      expected_hash: expectedHash,
    }).then(normalizeProjectEnvironment);
  }

  deleteProjectEnvironment(
    scope: string | Conversation,
    environmentID: string,
    expectedHash?: string,
  ): Promise<{ deleted: boolean; id: string }> {
    return this.request<{ deleted: boolean; id: string }>({
      type: "delete_project_environment",
      ...projectEnvironmentScope(scope),
      environment_id: environmentID,
      expected_hash: expectedHash,
    });
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
    return this.request<{
      capabilities?: CapabilityState["capabilities"] | null;
      skills?: CapabilityState["skills"] | null;
      skill_diagnostics?: CapabilityState["skill_diagnostics"] | null;
    } | null>({
      type: "get_capabilities",
      ...conversationScope(conversation),
    }).then((value) => ({
      capabilities: value?.capabilities ?? [],
      skills: value?.skills ?? [],
      skill_diagnostics: value?.skill_diagnostics ?? [],
    }));
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

  getProjectSandbox(project: Conversation): Promise<ProjectSandboxSettings> {
    return this.request<ProjectSandboxSettings>({
      type: "get_project_sandbox",
      ...conversationScope(project),
    });
  }

  setProjectSandbox(project: Conversation, sandbox: ProjectSandboxSettings["effective"] | "inherit"): Promise<ProjectSandboxSettings> {
    return this.request<ProjectSandboxSettings>({
      type: "set_project_sandbox",
      ...conversationScope(project),
      sandbox,
    });
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

function projectEnvironmentScope(scope: string | Conversation) {
  return typeof scope === "string" ? { folder: scope } : conversationScope(scope);
}

function normalizeProjectEnvironment(environment: ProjectEnvironment): ProjectEnvironment {
  return { ...environment, actions: environment.actions ?? [] };
}

export function unsupported(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /unknown command|unsupported|not supported/i.test(message);
}
