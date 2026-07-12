import { Plus, Server, Trash2 } from "lucide-react";
import { useState } from "react";
import type { GlobalSettings, MCPServerConfig } from "../../lib/protocol";
import { Field, JsonMapField, listText, parseList, SettingsSection, Toggle } from "./SettingsFields";

type MCPSettingsValue = GlobalSettings["mcp"];

export function MCPSettings(props: { value: MCPSettingsValue; onChange: (value: MCPSettingsValue) => void }) {
  const [name, setName] = useState("");
  const servers = props.value.servers ?? {};
  const update = (serverName: string, server: MCPServerConfig) =>
    props.onChange({ ...props.value, servers: { ...servers, [serverName]: server } });
  const remove = (serverName: string) => {
    const next = { ...servers };
    delete next[serverName];
    props.onChange({ ...props.value, servers: next });
  };
  const add = () => {
    const normalized = name.trim();
    if (!normalized || servers[normalized]) return;
    props.onChange({
      ...props.value,
      servers: {
        ...servers,
        [normalized]: {
          transport: "stdio",
          command: "",
          args: [],
          approval: "on-request",
          enabled: true,
        },
      },
    });
    setName("");
  };

  return (
    <SettingsSection
      id="mcp"
      eyebrow="GLOBAL MODEL CONTEXT PROTOCOL"
      title="Global MCP registry"
      description="Define stdio or Streamable HTTP servers once for the daemon. Projects and chats inherit each server's default and can override it in Conversation details."
    >
      <div className="inline-create">
        <label><span>Server name</span><input value={name} onChange={(event) => setName(event.target.value)} placeholder="filesystem" /></label>
        <button className="secondary-button" onClick={add} disabled={!name.trim() || Boolean(servers[name.trim()])}><Plus size={14} /> Add server</button>
      </div>
      <div className="integration-stack">
        {Object.entries(servers).map(([serverName, server]) => (
          <MCPServerCard key={serverName} name={serverName} value={server} onChange={(value) => update(serverName, value)} onRemove={() => remove(serverName)} />
        ))}
        {!Object.keys(servers).length && (
          <div className="integration-empty"><Server size={20} /><strong>No MCP servers in the global registry</strong><span>Add a trusted server here, then enable or disable it per conversation as needed.</span></div>
        )}
      </div>
    </SettingsSection>
  );
}

function MCPServerCard(props: { name: string; value: MCPServerConfig; onChange: (value: MCPServerConfig) => void; onRemove: () => void }) {
  const { value } = props;
  const set = <K extends keyof MCPServerConfig,>(key: K, next: MCPServerConfig[K]) => props.onChange({ ...value, [key]: next });
  return (
    <article className="integration-card">
      <header>
        <div className="integration-icon"><Server size={16} /></div>
        <div><strong>{props.name}</strong><span>{value.transport} · default {value.enabled ? "on" : "off"}</span></div>
        <button className="icon-button danger-icon" onClick={props.onRemove} aria-label={`Remove ${props.name}`}><Trash2 size={14} /></button>
      </header>
      <div className="settings-grid three">
        <Field label="Transport">
          <select value={value.transport} onChange={(event) => {
            const transport = event.target.value as MCPServerConfig["transport"];
            props.onChange({ ...value, transport, ...(transport === "stdio" ? { url: "" } : { command: "", args: [] }) });
          }}><option value="stdio">stdio</option><option value="streamable-http">Streamable HTTP</option></select>
        </Field>
        <Field label="Approval">
          <select value={value.approval} onChange={(event) => set("approval", event.target.value as MCPServerConfig["approval"])}>
            <option value="never">Never</option><option value="on-request">On request</option><option value="always">Always</option>
          </select>
        </Field>
        <div className="settings-field toggle-field"><Toggle label="Enabled by default" checked={value.enabled} onChange={(enabled) => set("enabled", enabled)} /></div>
        {value.transport === "stdio" ? (
          <>
            <Field label="Command" wide><input value={value.command || ""} onChange={(event) => set("command", event.target.value)} placeholder="npx" /></Field>
            <Field label="Arguments" hint="One per line." wide><textarea rows={3} value={listText(value.args)} onChange={(event) => set("args", parseList(event.target.value))} /></Field>
          </>
        ) : (
          <Field label="Server URL" wide><input type="url" value={value.url || ""} onChange={(event) => set("url", event.target.value)} placeholder="https://mcp.example.com/mcp" /></Field>
        )}
        <Field label="Enabled tools" hint="Leave empty to expose every discovered tool." wide>
          <textarea rows={3} value={listText(value.enabled_tools)} onChange={(event) => set("enabled_tools", parseList(event.target.value))} />
        </Field>
        <JsonMapField label="Environment" value={value.env} onChange={(env) => set("env", env)} />
        <Field label="Secret environment keys" hint="Values stay [redacted] in this UI.">
          <textarea rows={3} value={listText(value.secret_env)} onChange={(event) => set("secret_env", parseList(event.target.value))} />
        </Field>
        <JsonMapField label="HTTP headers" value={value.headers} onChange={(headers) => set("headers", headers)} />
        <Field label="Secret header keys" hint="Redacted placeholders preserve stored secrets.">
          <textarea rows={3} value={listText(value.secret_headers)} onChange={(event) => set("secret_headers", parseList(event.target.value))} />
        </Field>
      </div>
      <p className="secret-note">Secret fields returned by the daemon remain <code>[redacted]</code>. Saving that placeholder never overwrites the stored value.</p>
    </article>
  );
}
