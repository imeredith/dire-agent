import { Boxes, FileCode2, FolderOpen, Server, ShieldCheck, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { arraysEqual, mergeModelOptions, shortID, thinkingLevels, usageContextWindow } from "../../lib/display";
import {
  emptyUsage,
  normalizeUsage,
  type CapabilityState,
  type Command,
  type Conversation,
  type ModelInfo,
  type QueueMode,
  type RuntimeState,
} from "../../lib/protocol";
import { UsageSummary } from "./UsageSummary";
import { SubagentPanel } from "./SubagentPanel";
import type { SubagentController } from "../../hooks/useSubagents";
import type { CapabilityCommandController } from "../../hooks/useCapabilityCommands";
import { CapabilityCommandPanel } from "./CapabilityCommandPanel";
import {
  formatAdditionalFolders,
  parseAdditionalFolders,
  sameAdditionalFolders,
} from "../../lib/sandbox-folders";

interface DrawerProps {
  open: boolean;
  resource: Conversation | null;
  runtime: RuntimeState | null;
  capabilities: CapabilityState;
  models: ModelInfo[];
  tools: string[];
  subagents: SubagentController;
  capabilityCommands: CapabilityCommandController;
  onClose: () => void;
  onUpdate: (command: Omit<Command, "id">, notice?: string) => Promise<Conversation | null>;
  onDelete: (conversation: Conversation) => Promise<void>;
}

export function ConversationDrawer(props: DrawerProps) {
  const { open, resource, runtime } = props;
  const [toolDraft, setToolDraft] = useState<string[]>([]);
  const [folderDraft, setFolderDraft] = useState("");
  useEffect(() => setToolDraft(resource?.tools ?? []), [resource?.id, resource?.tools]);
  useEffect(() => {
    setFolderDraft(formatAdditionalFolders(resource?.additional_folders));
  }, [resource?.id, resource?.additional_folders]);
  useEffect(() => {
    if (!open) return;
    const close = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") props.onClose();
    };
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [open, props.onClose]);

  const modelOptions = useMemo(
    () => mergeModelOptions(props.models, resource?.model),
    [props.models, resource?.model],
  );
  const mcpServers = useMemo(
    () => props.capabilities.capabilities.filter(isMCPServerDescriptor),
    [props.capabilities.capabilities],
  );
  const otherCapabilities = useMemo(
    () => props.capabilities.capabilities.filter((item) => !isMCPServerDescriptor(item)),
    [props.capabilities.capabilities],
  );
  if (!open) return null;

  const running = Boolean(runtime?.running);
  const isChat = runtime?.kind === "chat" || resource?.kind === "chat" || resource?.id.startsWith("chat_");
  const usage = normalizeUsage(runtime?.usage || emptyUsage);
  const contextWindow = resource ? usageContextWindow(usage, props.models, resource.model) : 0;
  const parsedFolderDraft = parseAdditionalFolders(folderDraft);
  return (
    <>
      <button className="drawer-scrim" onClick={props.onClose} aria-label="Close conversation details" />
      <aside className="conversation-drawer" aria-label="Conversation details">
        <div className="drawer-heading">
          <div>
            <span className="eyebrow">{isChat ? "CHAT DETAILS" : "PROJECT DETAILS"}</span>
            <strong>Conversation controls</strong>
          </div>
          <button className="icon-button" onClick={props.onClose} aria-label="Close conversation details"><X size={17} /></button>
        </div>
        {resource && runtime ? (
          <div className="drawer-scroll">
            <UsageSummary usage={usage} contextWindow={contextWindow} />
            <fieldset className="field-stack" disabled={running}>
              <label>
                <span>{isChat ? "Chat name" : "Project name"}</span>
                <input
                  key={`${resource.id}-${resource.name}`}
                  defaultValue={resource.name || ""}
                  placeholder={isChat ? "Untitled chat" : "Unnamed project"}
                  onKeyDown={(event) => event.key === "Enter" && event.currentTarget.blur()}
                  onBlur={(event) => {
                    const name = event.target.value.trim();
                    if (name !== (resource.name || "")) {
                      void props.onUpdate({ type: "set_conversation_name", name }, "Conversation renamed");
                    }
                  }}
                />
              </label>
              {!isChat && (
                <label>
                  <span>Project category</span>
                  <input
                    key={`${resource.id}-${resource.category || "uncategorized"}`}
                    defaultValue={resource.category || ""}
                    placeholder="Uncategorized"
                    aria-label="Project category"
                    maxLength={80}
                    onKeyDown={(event) => event.key === "Enter" && event.currentTarget.blur()}
                    onBlur={(event) => {
                      const category = event.target.value.trim();
                      if (category !== (resource.category || "")) {
                        void props.onUpdate({ type: "set_project_category", category }, "Project category updated");
                      }
                    }}
                  />
                </label>
              )}
              <label>
                <span>Model</span>
                <select
                  aria-label="Model"
                  value={resource.model}
                  onChange={(event) => void props.onUpdate({ type: "set_model", model: event.target.value }, "Model updated")}
                >
                  {modelOptions.map((model) => (
                    <option key={model.id} value={model.id}>{model.id}{model.provider ? ` · ${model.provider}` : ""}</option>
                  ))}
                </select>
              </label>
              <label>
                <span>Thinking level</span>
                <select
                  aria-label="Thinking level"
                  value={resource.thinking_level}
                  onChange={(event) => void props.onUpdate({ type: "set_thinking_level", level: event.target.value }, "Thinking updated")}
                >
                  {thinkingLevels.map((level) => <option key={level}>{level}</option>)}
                </select>
              </label>
              <div className="field-grid">
                <QueueField
                  label="Steering queue"
                  value={resource.steering_mode}
                  onChange={(mode) => void props.onUpdate({ type: "set_steering_mode", mode }, "Queue updated")}
                />
                <QueueField
                  label="Follow-up queue"
                  value={resource.follow_up_mode}
                  onChange={(mode) => void props.onUpdate({ type: "set_follow_up_mode", mode }, "Queue updated")}
                />
              </div>
            </fieldset>

            {!isChat && (
              <section className="drawer-section">
                <div className="section-title">
                  <span>Sandbox folders</span>
                  <small>{1 + (resource.additional_folders?.length ?? 0)} total</small>
                </div>
                <div className="resource-card sandbox-main-folder">
                  <FolderOpen size={15} />
                  <div><strong>{resource.cwd}</strong><small>Main project folder · relative paths start here</small></div>
                </div>
                <label className="settings-field sandbox-folder-editor">
                  <span>Additional folders</span>
                  <textarea
                    aria-label="Additional sandbox folders"
                    value={folderDraft}
                    onChange={(event) => setFolderDraft(event.target.value)}
                    placeholder={"/absolute/path/to/shared\n/absolute/path/to/docs"}
                    rows={4}
                    spellCheck={false}
                    disabled={running}
                  />
                  <small>One absolute folder per line. These folders join the sandbox without replacing the main project folder.</small>
                </label>
                <button
                  className="secondary-button full-width"
                  disabled={running || sameAdditionalFolders(parsedFolderDraft, resource.additional_folders ?? [])}
                  onClick={() => void props.onUpdate({
                    type: "set_project_sandbox_folders",
                    additional_folders: parsedFolderDraft,
                  }, "Sandbox folders updated")}
                >Save sandbox folders</button>
              </section>
            )}

            {!isChat && (
              <section className="drawer-section">
                <div className="section-title"><span>Folder tools</span><small>{toolDraft.length}/{props.tools.length}</small></div>
                <div className="tool-grid">
                  {props.tools.map((tool) => (
                    <label key={tool} className={toolDraft.includes(tool) ? "tool-chip checked" : "tool-chip"}>
                      <input
                        type="checkbox"
                        checked={toolDraft.includes(tool)}
                        disabled={running}
                        onChange={() => setToolDraft((current) => current.includes(tool)
                          ? current.filter((value) => value !== tool)
                          : [...current, tool])}
                      />
                      <FileCode2 size={13} /> {tool}
                    </label>
                  ))}
                </div>
                <button
                  className="secondary-button full-width"
                  disabled={running || arraysEqual(toolDraft, resource.tools)}
                  onClick={() => void props.onUpdate({ type: "set_tools", tools: toolDraft }, "Tools updated")}
                >Save tool access</button>
              </section>
            )}

            <section className="drawer-section">
              <div className="section-title"><span>MCP registry</span><small>{mcpServers.length} global</small></div>
              <p className="quiet-copy mcp-registry-help">
                Inherit uses the global registry default. On or Off applies only to this {isChat ? "chat" : "project"}.
              </p>
              <div className="mcp-registry-list">
                {mcpServers.map((server) => {
                  const serverName = server.name.slice("mcp:".length);
                  const override = mcpOverrideValue(resource, serverName);
                  return (
                    <div className="mcp-registry-row" key={server.name} role="group" aria-label={`${serverName} MCP server`}>
                      <Server size={15} />
                      <div className="mcp-registry-copy">
                        <strong>{serverName}</strong>
                        <small title={server.description}>{server.description || "Global MCP server"}</small>
                        <small className="mcp-registry-status">
                          <span className={server.enabled && server.status === "ready" ? "status-ready" : "status-muted"} />
                          {server.enabled ? "Enabled" : "Disabled"} · {server.status || (server.enabled ? "ready" : "disabled")}
                        </small>
                      </div>
                      <select
                        aria-label={`MCP override for ${serverName}`}
                        value={override}
                        disabled={running}
                        onChange={(event) => {
                          const value = event.target.value as MCPOverrideValue;
                          const enabled = value === "inherit" ? null : value === "on";
                          void props.onUpdate({
                            type: "set_mcp_server_enabled",
                            mcp_server: serverName,
                            enabled,
                          }, enabled === null ? `${serverName} now inherits the global MCP registry` : `${serverName} override updated`);
                        }}
                      >
                        <option value="inherit">Inherit</option>
                        <option value="on">On</option>
                        <option value="off">Off</option>
                      </select>
                    </div>
                  );
                })}
                {!mcpServers.length && <p className="quiet-copy">No MCP servers are configured in the global registry.</p>}
              </div>
            </section>

            <section className="drawer-section">
              <div className="section-title"><span>Capabilities</span><small>{otherCapabilities.length}</small></div>
              <div className="capability-list">
                {otherCapabilities.map((item) => (
                  <div className="capability-row" key={`${item.source}:${item.name}`}>
                    <Boxes size={14} />
                    <div><strong>{item.name}</strong><small>{item.source} · {item.status || (item.enabled ? "ready" : "disabled")}</small></div>
                    <span className={item.enabled ? "status-ready" : "status-muted"} />
                  </div>
                ))}
                {!otherCapabilities.length && <p className="quiet-copy">No capabilities discovered.</p>}
              </div>
            </section>

            <section className="drawer-section">
              <div className="section-title"><span>Skills</span><small>{props.capabilities.skills.length}</small></div>
              <div className="capability-list">
                {props.capabilities.skills.map((skill) => (
                  <div className="capability-row" key={skill.path}>
                    <ShieldCheck size={14} />
                    <div><strong>{skill.name}</strong><small>{skill.scope} · {skill.enabled ? "enabled" : skill.disabled_reason || "disabled"}</small></div>
                    <span className={skill.enabled ? "status-ready" : "status-muted"} />
                  </div>
                ))}
                {!props.capabilities.skills.length && <p className="quiet-copy">No skills discovered in configured roots.</p>}
              </div>
            </section>

            <CapabilityCommandPanel controller={props.capabilityCommands} />

            <SubagentPanel controller={props.subagents} models={props.models} />

            <div className="resource-card">
              {isChat ? <ShieldCheck size={15} /> : <FolderOpen size={15} />}
              <div><strong>{isChat ? "Pathless chat" : resource.cwd}</strong><small>{shortID(resource.id)} · SQLite persisted</small></div>
            </div>
            <button className="danger-button" disabled={running} onClick={() => void props.onDelete(resource)}>
              <Trash2 size={14} /> Delete {isChat ? "chat" : "project"} and history
            </button>
            {running && <p className="quiet-copy">Controls unlock when the current run settles.</p>}
          </div>
        ) : <div className="drawer-empty">Select a conversation to inspect it.</div>}
      </aside>
    </>
  );
}

function QueueField({ label, value, onChange }: { label: string; value: QueueMode; onChange: (value: QueueMode) => void }) {
  return (
    <label>
      <span>{label}</span>
      <select value={value} onChange={(event) => onChange(event.target.value as QueueMode)}>
        <option value="one-at-a-time">One at a time</option>
        <option value="all">All at once</option>
      </select>
    </label>
  );
}

type MCPOverrideValue = "inherit" | "on" | "off";

function isMCPServerDescriptor(item: CapabilityState["capabilities"][number]) {
  return item.source === "mcp" && item.name.startsWith("mcp:");
}

function mcpOverrideValue(resource: Conversation, serverName: string): MCPOverrideValue {
  if (!resource.mcp_server_overrides || !Object.prototype.hasOwnProperty.call(resource.mcp_server_overrides, serverName)) return "inherit";
  return resource.mcp_server_overrides?.[serverName] ? "on" : "off";
}
