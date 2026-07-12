import type {
  CapabilityState,
  CapabilityCommandInfo,
  CapabilityCommandResult,
  Command,
  ConnectionStatus,
  Conversation,
  CreateChatOptions,
  CreateProjectOptions,
  DaemonConfig,
  ModelInfo,
  ProjectLauncher,
  RuntimeState,
  ScheduledPrompt,
  ScheduledPromptInput,
  SpawnAgentOptions,
  StoredEvent,
  StoredMessage,
  SubagentInfo,
  WireEvent,
} from "../lib/protocol";
import { conversationKind, conversationScope } from "../lib/protocol";

export const projectFixture: Conversation = {
  id: "project_web_test",
  kind: "project",
  name: "Web project",
  category: "Internal",
  model: "gpt-test",
  cwd: "/workspace",
  additional_folders: [],
  thinking_level: "medium",
  steering_mode: "one-at-a-time",
  follow_up_mode: "one-at-a-time",
  tools: ["read"],
  status: "idle",
  created_at: "2026-07-10T00:00:00Z",
  updated_at: "2026-07-10T00:00:00Z",
};

export const chatFixture: Conversation = {
  ...projectFixture,
  id: "chat_web_test",
  kind: "chat",
  name: "Web chat",
  cwd: "",
  tools: [],
};

export const scheduledPromptFixture: ScheduledPrompt = {
  id: "schedule_web_test",
  name: "Weekday review",
  prompt: "Review the project status",
  target_type: "project",
  conversation_id: projectFixture.id,
  schedule_type: "cron",
  cron: "0 9 * * 1-5",
  timezone: "Pacific/Auckland",
  enabled: true,
  next_run_at: "2026-07-13T21:00:00Z",
  created_at: "2026-07-10T00:00:00Z",
  updated_at: "2026-07-10T00:00:00Z",
};

export function configFixture(): DaemonConfig {
  return {
    version: 1,
    revision: 3,
    global: {
      model: { provider: "codex", id: "gpt-5.6", context_window: 372_000 },
      thinking: { level: "medium" },
      tools: { enabled: ["read", "grep"], sandbox: "strict", approval: "on-request" },
      queues: { steering_mode: "one-at-a-time", follow_up_mode: "one-at-a-time", max_pending: 64 },
      skills: { roots: ["/Users/test/.agents/skills"], disabled: [], trust: "prompt" },
      mcp: { servers: {} },
      extensions: { sources: {}, allow_unsigned: false },
      subagents: {
        enabled: true,
        max_depth: 2,
        max_children: 8,
        max_concurrent: 4,
        allow_sibling_messages: true,
        auto_report: true,
        profiles: {
          general: { description: "General agent", thinking: "medium", tools: null, can_spawn: false },
        },
      },
      launchers: [
        { id: "shell", label: "Terminal", kind: "terminal", shortcut: "mod+backquote" },
        { id: "lazygit", label: "lazygit", kind: "terminal", command: "lazygit", shortcut: "mod+shift+g" },
        { id: "nvim", label: "nvim", kind: "terminal", command: "nvim", args: ["."], shortcut: "mod+shift+e" },
        { id: "finder", label: "Finder", kind: "desktop", command: "/usr/bin/open", args: ["."], shortcut: "mod+shift+f" },
      ],
      desktop: {
        codex_home: "/Users/test/.codex",
        desktop_config: "/Users/test/.codex/config.toml",
        sync_mode: "import",
        sync_skills: true,
        sync_mcp: true,
        sync_extensions: true,
        watch_for_changes: false,
      },
      standalone_chat: {
        model: "gpt-5.6",
        thinking: "medium",
        tools: [],
        instructions: "",
        persist_history: true,
      },
    },
    projects: {},
  };
}

export const mockState = {
  requests: [] as Command[],
  eventListeners: [] as Array<(event: WireEvent) => void>,
  projects: [] as Conversation[],
  chats: [] as Conversation[],
  messages: {} as Record<string, StoredMessage[]>,
  messageWaiters: {} as Record<string, Promise<void>>,
  events: {} as Record<string, StoredEvent[]>,
  agents: [] as SubagentInfo[],
  config: configFixture(),
  configConflict: false,
  capabilityCommands: [] as CapabilityCommandInfo[],
  capabilityCommandResult: { output: "" } as CapabilityCommandResult,
  capabilityCommandError: "",
  schedules: [] as ScheduledPrompt[],
};

export function resetMockDaemon() {
  mockState.requests.length = 0;
  mockState.eventListeners.length = 0;
  mockState.projects = [];
  mockState.chats = [];
  mockState.messages = {};
  mockState.messageWaiters = {};
  mockState.events = {};
  mockState.agents = [];
  mockState.config = configFixture();
  mockState.configConflict = false;
  mockState.capabilityCommands = [];
  mockState.capabilityCommandResult = { output: "" };
  mockState.capabilityCommandError = "";
  mockState.schedules = [];
}

export class MockDaemonClient {
  isOpen = true;
  wasManuallyClosed = false;
  private statusListeners: Array<(status: ConnectionStatus, error?: Error) => void> = [];

  constructor(readonly url: string) {}
  onEvent(listener: (event: WireEvent) => void) { mockState.eventListeners.push(listener); return () => undefined; }
  onStatus(listener: (status: ConnectionStatus, error?: Error) => void) { this.statusListeners.push(listener); return () => undefined; }
  async connect() { this.statusListeners.forEach((listener) => listener("online")); }
  close() {
    this.wasManuallyClosed = true;
    this.isOpen = false;
    this.statusListeners.forEach((listener) => listener("offline"));
  }
  async listProjects() { return mockState.projects; }
  async listChats() { return mockState.chats; }
  async getAvailableTools() { return ["read", "write", "bash"]; }
  async getAvailableModels(): Promise<ModelInfo[]> {
    return [
      { provider: "test", id: "gpt-test", context_window: 1_000 },
      { provider: "test", id: "server-special", context_window: 2_000 },
    ];
  }
  async getProjectLaunchers(project: Conversation): Promise<ProjectLauncher[]> {
    this.record({ type: "get_project_launchers", ...conversationScope(project) });
    return structuredClone(mockState.config.global.launchers ?? []);
  }
  async launchProjectApp(project: Conversation, launcherID: string) {
    this.record({ type: "launch_project_app", ...conversationScope(project), launcher_id: launcherID });
    const launcher = mockState.config.global.launchers?.find((item) => item.id === launcherID);
    return { launched: true, id: launcherID, label: launcher?.label };
  }
  async createProject(options: CreateProjectOptions) {
    this.record({ type: "create_project", options });
    const project = {
      ...projectFixture,
      name: options.name,
      category: options.category,
      cwd: options.cwd || "/workspace",
      additional_folders: options.additional_folders ?? [],
    };
    mockState.projects = [project, ...mockState.projects];
    return project;
  }
  async createChat(options: CreateChatOptions) {
    this.record({ type: "create_chat", chat_options: options });
    const chat = { ...chatFixture, name: options.name };
    mockState.chats = [chat, ...mockState.chats];
    return chat;
  }
  async getConversation(conversation: Conversation) { return this.find(conversation.id); }
  async getState(conversation: Conversation): Promise<RuntimeState> {
    const canonical = this.find(conversation.id);
    const kind = conversationKind(canonical);
    return {
      kind,
      conversation: canonical,
      thread: canonical,
      ...(kind === "chat" ? { chat: canonical } : { project: canonical }),
      running: canonical.status === "running",
      steering_queued: 0,
      follow_ups_queued: 0,
      usage: {
        input_tokens: 10,
        output_tokens: 20,
        cache_read_tokens: 30,
        cache_write_tokens: 40,
        total_tokens: 30,
        context_tokens: 100,
        context_window: 1_000,
      },
    };
  }
  async getMessages(conversation: Conversation) {
    await mockState.messageWaiters[conversation.id];
    return mockState.messages[conversation.id] ?? [];
  }
  async getEvents(conversation: Conversation) { return mockState.events[conversation.id] ?? []; }
  async subscribe(conversation: Conversation) { this.record({ type: "subscribe", ...conversationScope(conversation) }); }
  async unsubscribe(conversation: Conversation) { this.record({ type: "unsubscribe", ...conversationScope(conversation) }); }
  async deleteConversation(conversation: Conversation) {
    this.record({ type: conversationKind(conversation) === "chat" ? "delete_chat" : "delete_project", ...conversationScope(conversation) });
    mockState.projects = mockState.projects.filter((item) => item.id !== conversation.id);
    mockState.chats = mockState.chats.filter((item) => item.id !== conversation.id);
  }
  async getCapabilities(): Promise<CapabilityState> {
    return {
      capabilities: [{ name: "read", source: "builtin", enabled: true, status: "ready" }],
      skills: [{ name: "review", description: "Review", path: "/skills/review/SKILL.md", directory: "/skills/review", root: "/skills", scope: "global", enabled: true }],
      skill_diagnostics: [],
    };
  }
  async listCapabilityCommands(conversation: Conversation) {
    this.record({ type: "list_capability_commands", ...conversationScope(conversation) });
    return mockState.capabilityCommands;
  }
  async executeCapabilityCommand(conversation: Conversation, commandName: string, commandArguments: string) {
    this.record({
      type: "execute_capability_command",
      ...conversationScope(conversation),
      command_name: commandName,
      arguments: commandArguments,
    });
    if (mockState.capabilityCommandError) throw new Error(mockState.capabilityCommandError);
    return mockState.capabilityCommandResult;
  }
  async getConfig() { this.record({ type: "config_get" }); return structuredClone(mockState.config); }
  async validateConfig(config: DaemonConfig) { this.record({ type: "config_validate", config }); return { valid: true }; }
  async updateConfig(config: DaemonConfig, expectedRevision: number) {
    this.record({ type: "config_update", config, expected_revision: expectedRevision });
    if (mockState.configConflict) throw new Error(`configuration: revision conflict: expected ${expectedRevision}, actual ${expectedRevision + 1}`);
    mockState.config = { ...structuredClone(config), revision: expectedRevision + 1 };
    return structuredClone(mockState.config);
  }
  async listScheduledPrompts() {
    this.record({ type: "list_scheduled_prompts" });
    return structuredClone(mockState.schedules);
  }
  async createScheduledPrompt(schedule: ScheduledPromptInput) {
    this.record({ type: "create_scheduled_prompt", schedule });
    const created: ScheduledPrompt = {
      ...schedule,
      id: `schedule_${mockState.schedules.length + 1}`,
      next_run_at: schedule.enabled ? (schedule.run_at || "2026-07-13T21:00:00Z") : undefined,
      created_at: "2026-07-12T00:00:00Z",
      updated_at: "2026-07-12T00:00:00Z",
    };
    mockState.schedules = [...mockState.schedules, created];
    return structuredClone(created);
  }
  async updateScheduledPrompt(scheduleID: string, schedule: ScheduledPromptInput) {
    this.record({ type: "update_scheduled_prompt", schedule_id: scheduleID, schedule });
    const current = mockState.schedules.find((item) => item.id === scheduleID)!;
    const updated: ScheduledPrompt = {
      ...current,
      ...schedule,
      next_run_at: schedule.enabled ? (schedule.run_at || current.next_run_at || "2026-07-13T21:00:00Z") : undefined,
      updated_at: "2026-07-12T00:00:01Z",
    };
    mockState.schedules = mockState.schedules.map((item) => item.id === scheduleID ? updated : item);
    return structuredClone(updated);
  }
  async deleteScheduledPrompt(scheduleID: string) {
    this.record({ type: "delete_scheduled_prompt", schedule_id: scheduleID });
    mockState.schedules = mockState.schedules.filter((item) => item.id !== scheduleID);
  }
  async runScheduledPrompt(scheduleID: string) {
    this.record({ type: "run_scheduled_prompt", schedule_id: scheduleID });
    const current = mockState.schedules.find((item) => item.id === scheduleID)!;
    const updated: ScheduledPrompt = {
      ...current,
      last_run_at: "2026-07-12T00:00:02Z",
      last_status: "started",
      updated_at: "2026-07-12T00:00:02Z",
    };
    mockState.schedules = mockState.schedules.map((item) => item.id === scheduleID ? updated : item);
    return structuredClone(updated);
  }
  async subscribeScheduledPrompts() { this.record({ type: "subscribe_scheduled_prompts" }); }
  async unsubscribeScheduledPrompts() { this.record({ type: "unsubscribe_scheduled_prompts" }); }
  async listAgents(conversation: Conversation) { this.record({ type: "list_agents", ...conversationScope(conversation), parent_id: conversation.id }); return mockState.agents; }
  async spawnAgent(conversation: Conversation, options: SpawnAgentOptions) {
    this.record({ type: "spawn_agent", ...conversationScope(conversation), ...options, parent_id: options.parent_id || conversation.id });
    const agent: SubagentInfo = {
      id: "agent_review",
      conversation_id: conversation.id,
      parent_id: options.parent_id,
      root_id: "agent_review",
      agent_name: options.agent_name,
      agent_role: options.agent_role,
      task: options.task,
      model: options.model,
      depth: options.parent_id ? 2 : 1,
      status: "running",
    };
    mockState.agents = [...mockState.agents, agent];
    return agent;
  }
  async getAgent(_conversation: Conversation, agentID: string): Promise<SubagentInfo> {
    return mockState.agents.find((item) => item.id === agentID)!;
  }
  async getAgentMessages(agentID: string) { return mockState.messages[agentID] ?? []; }
  async sendAgentMessage(conversation: Conversation, agentID: string, message: string) {
    this.record({ type: "send_agent_message", ...conversationScope(conversation), parent_id: conversation.id, agent_id: agentID, message, wake: true });
    return { id: "agentmsg_out", agent_id: agentID, from_id: conversation.id, to_id: agentID, content: message, created_at: "now" };
  }
  async interruptAgent(conversation: Conversation, agentID: string) {
    this.record({ type: "interrupt_agent", ...conversationScope(conversation), parent_id: conversation.id, agent_id: agentID });
  }
  async deleteAgent(conversation: Conversation, agentID: string) {
    this.record({ type: "delete_agent", ...conversationScope(conversation), parent_id: conversation.id, agent_id: agentID });
    mockState.agents = mockState.agents.filter((item) => item.id !== agentID);
  }

  async request<T>(command: Command): Promise<T> {
    this.record(command);
    const id = command.conversation_id || command.chat_id || command.project_id || command.thread_id || "";
    if (["set_model", "set_thinking_level", "set_conversation_name", "set_project_category", "set_project_sandbox_folders", "set_tools", "set_steering_mode", "set_follow_up_mode"].includes(command.type)) {
      const current = this.find(id);
      const updated: Conversation = {
        ...current,
        ...(command.model ? { model: command.model } : {}),
        ...(command.level ? { thinking_level: command.level as Conversation["thinking_level"] } : {}),
        ...(command.name !== undefined ? { name: command.name } : {}),
        ...(command.category !== undefined ? { category: command.category } : {}),
        ...(command.additional_folders ? { additional_folders: command.additional_folders } : {}),
        ...(command.tools ? { tools: command.tools } : {}),
        ...(command.type === "set_steering_mode" ? { steering_mode: command.mode as Conversation["steering_mode"] } : {}),
        ...(command.type === "set_follow_up_mode" ? { follow_up_mode: command.mode as Conversation["follow_up_mode"] } : {}),
      };
      this.replace(updated);
      return updated as T;
    }
    return undefined as T;
  }

  private record(command: Command) { mockState.requests.push(command); }
  private find(id: string) { return [...mockState.chats, ...mockState.projects].find((item) => item.id === id)!; }
  private replace(value: Conversation) {
    mockState.chats = mockState.chats.map((item) => item.id === value.id ? value : item);
    mockState.projects = mockState.projects.map((item) => item.id === value.id ? value : item);
  }
}
