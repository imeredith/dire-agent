import { AlertTriangle, RefreshCw, Save } from "lucide-react";
import type { SettingsController } from "../../hooks/useSettings";
import { DesktopSettings } from "./DesktopSettings";
import { ExtensionSettings } from "./ExtensionSettings";
import { GeneralSettings } from "./GeneralSettings";
import { MCPSettings } from "./MCPSettings";
import { SkillsSettings } from "./SkillsSettings";
import { SubagentSettings } from "./SubagentSettings";
import { WorkspaceLaunchersSettings } from "./WorkspaceLaunchersSettings";
import { effectiveProjectLaunchers } from "../../lib/terminal";

interface SettingsPageProps {
  controller: SettingsController;
  online: boolean;
  onSaved: () => void;
}

const links = [
  ["general", "General"],
  ["tools", "Tools & queues"],
  ["standalone", "Standalone chats"],
  ["skills", "Skills"],
  ["mcp", "MCP registry"],
  ["extensions", "Extensions"],
  ["subagents", "Subagents"],
  ["workspace-launchers", "Workspace tabs"],
  ["desktop", "Desktop apps"],
];

export function SettingsPage({ controller, online, onSaved }: SettingsPageProps) {
  const { draft } = controller;
  const save = async () => {
    if (await controller.save()) onSaved();
  };
  if (!online) {
    return <main className="settings-page settings-state min-h-0 min-w-0 flex-1 overflow-y-auto bg-[#090c10]"><h1>Settings unavailable</h1><p>Reconnect to the daemon to load its configuration.</p></main>;
  }
  if (controller.loading || !draft) {
    return <main className="settings-page settings-state min-h-0 min-w-0 flex-1 overflow-y-auto bg-[#090c10]"><RefreshCw className="spin" size={22} /><h1>Loading settings</h1><p>Reading the daemon's versioned configuration…</p></main>;
  }
  return (
    <main className="settings-page min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto bg-[#090c10]">
      <div className="settings-layout mx-auto grid w-full max-w-[1160px] gap-8 px-[clamp(22px,4vw,52px)] py-12 lg:grid-cols-[170px_minmax(0,1fr)]">
        <aside className="settings-index" aria-label="Settings sections">
          <span className="eyebrow">CONFIGURATION</span>
          {links.map(([id, label]) => <a href={`#${id}`} key={id}>{label}</a>)}
          <div className="revision-chip">Revision {draft.revision}</div>
        </aside>
        <div className="settings-content">
          <header className="settings-hero">
            <span className="eyebrow">DAEMON CONTROL PLANE</span>
            <h1>Configure how every agent works.</h1>
            <p>Model defaults, trust boundaries and integrations live in one revisioned file. Secrets are always redacted before reaching this browser.</p>
          </header>

          {controller.error && (
            <div className={`settings-alert ${controller.conflict ? "conflict" : ""}`} role="alert">
              <AlertTriangle size={17} />
              <div><strong>{controller.conflict ? "Configuration changed elsewhere" : "Cannot save configuration"}</strong><span>{controller.error}</span></div>
              {controller.conflict && <button className="secondary-button" onClick={() => void controller.reload()}><RefreshCw size={13} /> Reload latest</button>}
            </div>
          )}

          <GeneralSettings value={draft.global} onChange={(value) => controller.setGlobal(() => value)} />
          <SkillsSettings value={draft.global.skills} onChange={(skills) => controller.setGlobal((current) => ({ ...current, skills }))} />
          <MCPSettings value={draft.global.mcp} onChange={(mcp) => controller.setGlobal((current) => ({ ...current, mcp }))} />
          <ExtensionSettings value={draft.global.extensions} onChange={(extensions) => controller.setGlobal((current) => ({ ...current, extensions }))} />
          <SubagentSettings value={draft.global.subagents} onChange={(subagents) => controller.setGlobal((current) => ({ ...current, subagents }))} />
          <WorkspaceLaunchersSettings
            value={effectiveProjectLaunchers(draft.global.launchers)}
            onChange={(launchers) => controller.setGlobal((current) => ({ ...current, launchers }))}
          />
          <DesktopSettings value={draft.global.desktop} onChange={(desktop) => controller.setGlobal((current) => ({ ...current, desktop }))} />

          <footer className="settings-savebar">
            <div><strong>{controller.dirty ? "Unsaved changes" : "Configuration is up to date"}</strong><span>Saving validates the whole document and checks revision {draft.revision}.</span></div>
            <button className="primary-button" onClick={() => void save()} disabled={!controller.dirty || controller.saving}>
              <Save size={14} /> {controller.saving ? "Saving…" : "Save settings"}
            </button>
          </footer>
        </div>
      </div>
    </main>
  );
}
