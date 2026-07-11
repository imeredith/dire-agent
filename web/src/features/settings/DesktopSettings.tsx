import { AppWindow, Copy, ExternalLink } from "lucide-react";
import { useState } from "react";
import type { DesktopSettings as DesktopSettingsValue } from "../../lib/protocol";
import { Field, SettingsSection, Toggle } from "./SettingsFields";

export function DesktopSettings(props: { value: DesktopSettingsValue; onChange: (value: DesktopSettingsValue) => void }) {
  const [copied, setCopied] = useState("");
  const set = <K extends keyof DesktopSettingsValue,>(key: K, value: DesktopSettingsValue[K]) => props.onChange({ ...props.value, [key]: value });
  const copy = async (label: string, value: string) => {
    await navigator.clipboard?.writeText(value);
    setCopied(label);
    window.setTimeout(() => setCopied(""), 1_500);
  };
  return (
    <SettingsSection
      id="desktop"
      eyebrow="DESKTOP BRIDGE"
      title="Codex and ChatGPT apps"
      description="Keep standard skill and MCP configuration aligned with OpenAI desktop surfaces."
    >
      <div className="settings-grid two">
        <Field label="Codex home" wide><input value={props.value.codex_home} onChange={(event) => set("codex_home", event.target.value)} spellCheck={false} /></Field>
        <Field label="Codex config path" wide><input value={props.value.desktop_config || ""} onChange={(event) => set("desktop_config", event.target.value)} spellCheck={false} /></Field>
        <Field label="Sync direction">
          <select value={props.value.sync_mode} onChange={(event) => set("sync_mode", event.target.value as DesktopSettingsValue["sync_mode"])}>
            <option value="off">Off</option><option value="import">Import from Codex</option><option value="export">Export to Codex</option><option value="bidirectional">Bidirectional</option>
          </select>
        </Field>
      </div>
      <div className="toggle-grid">
        <Toggle label="Sync skills" checked={props.value.sync_skills} onChange={(value) => set("sync_skills", value)} />
        <Toggle label="Sync MCP" checked={props.value.sync_mcp} onChange={(value) => set("sync_mcp", value)} />
        <Toggle label="Sync extensions" checked={props.value.sync_extensions} onChange={(value) => set("sync_extensions", value)} />
        <Toggle label="Watch for changes" checked={props.value.watch_for_changes} onChange={(value) => set("watch_for_changes", value)} />
      </div>
      <div className="desktop-guides">
        <article>
          <div className="guide-icon"><AppWindow size={17} /></div>
          <div>
            <strong>Codex desktop, CLI and IDE</strong>
            <p>They share <code>config.toml</code> and Agent Skills roots. Use import or bidirectional sync, then restart a running agent session to take a fresh capability snapshot.</p>
            <button onClick={() => void copy("codex", props.value.desktop_config || props.value.codex_home)}><Copy size={13} /> {copied === "codex" ? "Copied" : "Copy config path"}</button>
          </div>
        </article>
        <article>
          <div className="guide-icon chatgpt">AI</div>
          <div>
            <strong>ChatGPT desktop</strong>
            <p>Add the Dire Agent Streamable HTTP MCP endpoint from ChatGPT's connector settings when the bridge is enabled. Local stdio servers remain available to Codex through the generated plugin configuration.</p>
            <a href="https://learn.chatgpt.com/docs/extend/mcp" target="_blank" rel="noreferrer">MCP setup guide <ExternalLink size={12} /></a>
          </div>
        </article>
      </div>
    </SettingsSection>
  );
}
