import type { DaemonConfig } from "./configuration";
export type {
  AgentProfile,
  DaemonConfig,
  DesktopSettings,
  ExtensionSourceConfig,
  GlobalSettings,
  MCPServerConfig,
  ProjectLauncher,
  ProjectLauncherKind,
  StandaloneChatSettings,
  SubagentSettings,
} from "./configuration";
export type {
  SpawnAgentOptions,
  SubagentDetail,
  SubagentInfo,
  SubagentMessage,
  SubagentStatus,
  SubagentWaitResult,
  WireSubagentInfo,
} from "./subagents";

export type ConnectionStatus = "connecting" | "online" | "offline";
export type ConversationKind = "project" | "chat";

export type ThinkingLevel =
  | "off"
  | "none"
  | "minimal"
  | "low"
  | "medium"
  | "high"
  | "xhigh"
  | "max";

export type QueueMode = "all" | "one-at-a-time";
export type ApprovalMode = "never" | "on-request" | "always";
export type TrustMode = "denied" | "prompt" | "trusted";
export type SandboxMode = "strict" | "workspace" | "off";

export interface Usage {
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  total_tokens: number;
  context_tokens: number;
  context_window: number;
}

export const emptyUsage: Usage = {
  input_tokens: 0,
  output_tokens: 0,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  total_tokens: 0,
  context_tokens: 0,
  context_window: 0,
};

export interface Conversation {
  id: string;
  kind?: ConversationKind;
  settings_id?: string;
  name?: string;
  category?: string;
  model: string;
  cwd?: string;
  additional_folders?: string[];
  instructions?: string;
  thinking_level: ThinkingLevel;
  steering_mode: QueueMode;
  follow_up_mode: QueueMode;
  tools: string[];
  worktree?: ProjectWorktree;
  usage?: Usage;
  status: "idle" | "running" | string;
  created_at: string;
  updated_at: string;
}

export interface ProjectWorktreeOptions {
  base_ref?: string;
  environment_id?: string;
  source_project_id?: string;
}

export interface ProjectWorktree {
  source_cwd: string;
  source_repository: string;
  path: string;
  project_relative_path?: string;
  base_ref: string;
  base_commit: string;
  environment_id?: string;
}

export type ProjectEnvironmentPlatform = "darwin" | "linux" | "win32";
export type ProjectEnvironmentActionIcon = "tool" | "run" | "debug" | "test";

export interface ProjectEnvironmentScript {
  script: string;
}

export interface ProjectEnvironmentLifecycle extends ProjectEnvironmentScript {
  darwin?: ProjectEnvironmentScript;
  linux?: ProjectEnvironmentScript;
  win32?: ProjectEnvironmentScript;
}

export interface ProjectEnvironmentAction {
  id?: string;
  name: string;
  icon?: ProjectEnvironmentActionIcon | null;
  command: string;
  platform?: ProjectEnvironmentPlatform;
}

export interface ProjectEnvironment {
  id: string;
  config_path?: string;
  hash?: string;
  version: number;
  name: string;
  setup: ProjectEnvironmentLifecycle;
  cleanup?: ProjectEnvironmentLifecycle;
  actions: ProjectEnvironmentAction[];
}

export interface ProjectWorkspaceInspection {
  folder: string;
  git_repository: boolean;
  repository_root?: string;
  project_relative_path?: string;
  head?: string;
  current_branch?: string;
  branches: string[];
  environments: ProjectEnvironment[];
}

export type Project = Conversation;
export type Chat = Conversation;
/** @deprecated The daemon keeps the thread alias for older clients. */
export type Thread = Conversation;

export interface CapabilityDescriptor {
  name: string;
  source: string;
  description?: string;
  enabled: boolean;
  status?: string;
}

export interface SkillInfo {
  name: string;
  description: string;
  path: string;
  directory: string;
  root: string;
  scope: "project" | "global" | "plugin" | string;
  plugin?: string;
  enabled: boolean;
  disabled_reason?: string;
}

export interface SkillDiagnostic {
  severity: "warning" | "error" | string;
  code: string;
  path?: string;
  line?: number;
  message: string;
}

export interface CapabilityState {
  capabilities: CapabilityDescriptor[];
  skills: SkillInfo[];
  skill_diagnostics?: SkillDiagnostic[];
}

export interface RuntimeState {
  kind: ConversationKind;
  conversation: Conversation;
  project?: Conversation;
  chat?: Conversation;
  thread?: Conversation;
  running: boolean;
  steering_queued: number;
  follow_ups_queued: number;
  usage?: Usage;
  capabilities?: CapabilityDescriptor[];
  skills?: SkillInfo[];
  skill_diagnostics?: SkillDiagnostic[];
}

export interface StoredMessage {
  sequence: number;
  kind: string;
  role?: string;
  content?: string;
  data?: Record<string, unknown>;
  created_at: string;
}

export interface StoredEvent {
  sequence: number;
  type: string;
  data?: Record<string, unknown>;
  created_at: string;
}

export interface WireEvent {
  type: string;
  scope?: { kind: ConversationKind | string; id: string };
  conversation_id?: string;
  chat_id?: string;
  project_id?: string;
  thread_id?: string;
  sequence?: number;
  timestamp: string;
  data?: Record<string, unknown>;
}

export interface CreateProjectOptions {
  name?: string;
  category?: string;
  model?: string;
  cwd?: string;
  additional_folders?: string[];
  instructions?: string;
  thinking_level?: ThinkingLevel;
  tools?: string[];
  worktree?: ProjectWorktreeOptions;
}

export interface CreateChatOptions {
  name?: string;
  model?: string;
  instructions?: string;
  thinking_level?: ThinkingLevel;
}

export interface ImageAttachment {
  name?: string;
  mime_type: string;
  data?: string;
  file?: string;
  size?: number;
}

export type CreateThreadOptions = CreateProjectOptions;

export interface Command {
  id?: string;
  type: string;
  conversation_id?: string;
  chat_id?: string;
  project_id?: string;
  thread_id?: string;
  message?: string;
  name?: string;
  category?: string;
  additional_folders?: string[];
  streaming_behavior?: string;
  options?: CreateProjectOptions;
  chat_options?: CreateChatOptions;
  after?: number;
  limit?: number;
  model?: string;
  level?: string;
  mode?: string;
  tools?: string[];
  config?: DaemonConfig;
  expected_revision?: number;
  agent_id?: string;
  agent_ids?: string[];
  parent_id?: string;
  root_id?: string;
  agent_name?: string;
  agent_role?: string;
  task?: string;
  profile?: string;
  wake?: boolean;
  timeout_ms?: number;
  command_name?: string;
  arguments?: string;
  launcher_id?: string;
  attachments?: ImageAttachment[];
  folder?: string;
  environment_id?: string;
  environment?: ProjectEnvironment;
  expected_hash?: string;
}

export interface ResponseEnvelope<T = unknown> {
  id: string;
  type: "response";
  command: string;
  success: boolean;
  data?: T;
  error?: string;
}

export interface ModelInfo {
  provider: string;
  id: string;
  context_window?: number;
}

export interface CapabilityCommandInfo {
  name: string;
  description?: string;
  source?: string;
}

export interface CapabilityCommandResult {
  output?: string;
  prompt?: string;
  is_error?: boolean;
}

function nonNegativeInteger(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value)
    ? Math.max(0, Math.trunc(value))
    : 0;
}

export function normalizeUsage(value: unknown): Usage {
  const source = value && typeof value === "object" ? (value as Record<string, unknown>) : {};
  const input = nonNegativeInteger(source.input_tokens);
  const output = nonNegativeInteger(source.output_tokens);
  return {
    input_tokens: input,
    output_tokens: output,
    cache_read_tokens: nonNegativeInteger(source.cache_read_tokens),
    cache_write_tokens: nonNegativeInteger(source.cache_write_tokens),
    total_tokens: nonNegativeInteger(source.total_tokens) || input + output,
    context_tokens: nonNegativeInteger(source.context_tokens),
    context_window: nonNegativeInteger(source.context_window),
  };
}

export function addUsage(current: Usage | undefined, next: unknown): Usage {
  const left = normalizeUsage(current);
  const right = normalizeUsage(next);
  return {
    input_tokens: left.input_tokens + right.input_tokens,
    output_tokens: left.output_tokens + right.output_tokens,
    cache_read_tokens: left.cache_read_tokens + right.cache_read_tokens,
    cache_write_tokens: left.cache_write_tokens + right.cache_write_tokens,
    total_tokens: left.total_tokens + right.total_tokens,
    context_tokens: right.context_tokens || left.context_tokens,
    context_window: right.context_window || left.context_window,
  };
}

export function conversationKind(value: Pick<Conversation, "id" | "kind">): ConversationKind {
  if (value.kind === "chat" || value.kind === "project") return value.kind;
  return value.id.startsWith("chat_") ? "chat" : "project";
}

function validConversation(value: unknown): value is Conversation {
  return Boolean(
    value &&
      typeof value === "object" &&
      typeof (value as Record<string, unknown>).id === "string" &&
      (value as Record<string, unknown>).id,
  );
}

export function normalizeRuntimeState(value: RuntimeState, fallback: Conversation): RuntimeState {
  const kind = value.kind === "chat" || value.kind === "project"
    ? value.kind
    : conversationKind(fallback);
  const ordered = kind === "chat"
    ? [value.conversation, value.chat, value.thread, fallback, value.project]
    : [value.conversation, value.project, value.thread, fallback, value.chat];
  const conversation = ordered.find(validConversation) ?? fallback;
  return {
    ...value,
    kind,
    conversation,
    thread: conversation,
    project: kind === "project" ? conversation : undefined,
    chat: kind === "chat" ? conversation : undefined,
    usage: normalizeUsage(value.usage || conversation.usage),
    capabilities: value.capabilities ?? [],
    skills: value.skills ?? [],
    skill_diagnostics: value.skill_diagnostics ?? [],
  };
}

export function wireConversationID(event: WireEvent): string {
  return event.conversation_id || event.chat_id || event.project_id || event.thread_id || event.scope?.id || "";
}

/** @deprecated Use wireConversationID. */
export const wireProjectID = wireConversationID;

export function conversationScope(
  conversation: Pick<Conversation, "id" | "kind"> | string,
  kind?: ConversationKind,
): Pick<Command, "conversation_id" | "chat_id" | "project_id" | "thread_id"> {
  const id = typeof conversation === "string" ? conversation : conversation.id;
  const resourceKind = kind ?? (typeof conversation === "string"
    ? (id.startsWith("chat_") ? "chat" : "project")
    : conversationKind(conversation));
  return {
    conversation_id: id,
    ...(resourceKind === "chat" ? { chat_id: id } : { project_id: id }),
    thread_id: id,
  };
}

export function normalizeHistoricalEvent(
  conversation: Pick<Conversation, "id" | "kind">,
  event: StoredEvent,
): WireEvent {
  return {
    type: event.type,
    ...conversationScope(conversation),
    sequence: event.sequence,
    timestamp: event.created_at,
    data: event.data,
  };
}

export function defaultWebSocketURL(): string {
  const configured = import.meta.env.VITE_DAEMON_URL as string | undefined;
  if (configured) return configured;
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws`;
}

export function attachmentHTTPURL(endpoint: string, projectID: string, file: string): string {
  const url = new URL(endpoint);
  url.protocol = url.protocol === "wss:" ? "https:" : "http:";
  url.pathname = `/attachments/${encodeURIComponent(projectID)}/${encodeURIComponent(file)}`;
  url.search = "";
  url.hash = "";
  return url.toString();
}
