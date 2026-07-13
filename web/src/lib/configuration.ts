import type {
  ApprovalMode,
  QueueMode,
  SandboxMode,
  ThinkingLevel,
  TrustMode,
} from "./protocol";

export interface DaemonConfig {
  version: number;
  revision: number;
  global: GlobalSettings;
  projects?: Record<string, unknown>;
}

export interface GlobalSettings {
  model: { provider: string; id: string; context_window?: number };
  thinking: { level: ThinkingLevel };
  tools: { enabled: string[]; sandbox: SandboxMode; approval: ApprovalMode };
  queues: { steering_mode: QueueMode; follow_up_mode: QueueMode; max_pending: number };
  skills: { roots: string[]; disabled?: string[]; trust: TrustMode };
  mcp: { servers?: Record<string, MCPServerConfig> };
  extensions: { sources?: Record<string, ExtensionSourceConfig>; allow_unsigned: boolean };
  subagents: SubagentSettings;
  /** Undefined is a legacy v1 document and receives the daemon defaults. */
  launchers?: ProjectLauncher[] | null;
  desktop: DesktopSettings;
  standalone_chat: StandaloneChatSettings;
}

export type ProjectLauncherKind = "terminal" | "desktop";

export interface ProjectLauncher {
  id: string;
  label: string;
  kind: ProjectLauncherKind;
  icon?: "tool" | "run" | "debug" | "test";
  command?: string;
  args?: string[];
  shortcut?: string;
}

export interface MCPServerConfig {
  transport: "stdio" | "streamable-http";
  command?: string;
  args?: string[];
  inherit_env?: boolean;
  url?: string;
  env?: Record<string, string>;
  secret_env?: string[];
  headers?: Record<string, string>;
  secret_headers?: string[];
  enabled_tools?: string[];
  approval: ApprovalMode;
  tool_approvals?: Record<string, ApprovalMode>;
  enabled: boolean;
}

export interface ExtensionSourceConfig {
  kind: "local" | "git" | "registry";
  location: string;
  ref?: string;
  trust: TrustMode;
  enabled: boolean;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  secret_env?: string[];
  inherit_env?: boolean;
}

export interface SubagentSettings {
  enabled: boolean;
  max_depth: number;
  max_children: number;
  max_concurrent: number;
  allow_sibling_messages: boolean;
  auto_report: boolean;
  profiles: Record<string, AgentProfile>;
}

export interface AgentProfile {
  description: string;
  instructions?: string;
  model?: string;
  thinking?: ThinkingLevel;
  tools?: string[] | null;
  can_spawn: boolean;
}

export interface DesktopSettings {
  codex_home: string;
  desktop_config?: string;
  sync_mode: "off" | "import" | "export" | "bidirectional";
  sync_skills: boolean;
  sync_mcp: boolean;
  sync_extensions: boolean;
  watch_for_changes: boolean;
}

export interface StandaloneChatSettings {
  model: string;
  thinking: ThinkingLevel;
  tools: string[];
  instructions?: string;
  persist_history: boolean;
}
