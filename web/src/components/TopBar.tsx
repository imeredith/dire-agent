import {
  AppWindow,
  Bug,
  FileCode2,
  FlaskConical,
  GitBranch,
  Menu,
  MessageSquareText,
  PanelRight,
  Play,
  Save,
  Settings2,
  SquareTerminal,
  Wrench,
  X,
} from "lucide-react";
import type { AppView } from "./AppSidebar";
import type { ConnectionStatus, ProjectLauncher } from "../lib/protocol";
import { formatLauncherShortcut } from "../lib/terminal";

interface TopBarProps {
  view: AppView;
  connection: ConnectionStatus;
  settingsDirty?: boolean;
  settingsSaving?: boolean;
  hasConversation: boolean;
  terminalAvailable?: boolean;
  launchers?: ProjectLauncher[];
  openLauncherIDs?: string[];
  activeLauncherID?: string;
  onMenu: () => void;
  onDetails: () => void;
  onShowConversation?: () => void;
  onToggleLauncher?: (launcher: ProjectLauncher) => void;
  onCloseLauncher?: (launcherID: string) => void;
  onSaveSettings?: () => void;
}

function LauncherIcon({ launcher }: { launcher: ProjectLauncher }) {
  if (launcher.icon === "tool") return <Wrench size={15} />;
  if (launcher.icon === "run") return <Play size={15} />;
  if (launcher.icon === "debug") return <Bug size={15} />;
  if (launcher.icon === "test") return <FlaskConical size={15} />;
  const searchable = `${launcher.id} ${launcher.label} ${launcher.command || ""}`.toLowerCase();
  if (launcher.kind === "desktop") return <AppWindow size={15} />;
  if (searchable.includes("git")) return <GitBranch size={15} />;
  if (searchable.includes("nvim") || searchable.includes("vim") || searchable.includes("editor")) return <FileCode2 size={15} />;
  return <SquareTerminal size={15} />;
}

export function TopBar(props: TopBarProps) {
  const open = new Set(props.openLauncherIDs ?? []);
  return (
    <header className="topbar relative z-30 flex h-[60px] shrink-0 items-center justify-between gap-4 border-b border-white/10 bg-canvas/90 px-4 backdrop-blur-xl">
      <div className="topbar-left flex min-w-0 items-center gap-2">
        <button className="icon-button menu-button" onClick={props.onMenu} aria-label="Open navigation"><Menu size={18} /></button>
        {props.view === "settings" ? (
          <div className="topbar-title"><Settings2 size={16} /><span><strong>Settings</strong><small>Daemon-wide configuration</small></span></div>
        ) : props.terminalAvailable ? (
          <div className="workspace-tabs" role="tablist" aria-label="Project workspace tabs">
            <button
              className={`workspace-tab-button ${!props.activeLauncherID ? "active" : ""}`}
              role="tab"
              aria-selected={!props.activeLauncherID}
              onClick={props.onShowConversation}
              title="Conversation"
            >
              <MessageSquareText size={15} /><span>Chat</span>
            </button>
            {(props.launchers ?? []).map((launcher) => {
              const active = props.activeLauncherID === launcher.id;
              const shortcut = formatLauncherShortcut(launcher.shortcut);
              return (
                <span className={`workspace-tab ${active ? "active" : ""} ${open.has(launcher.id) ? "open" : ""}`} key={launcher.id}>
                  <button
                    className="workspace-tab-button"
                    role="tab"
                    aria-selected={active}
                    aria-label={`${active ? "Hide" : "Open"} ${launcher.label}`}
                    title={`${launcher.label}${shortcut ? ` · ${shortcut}` : ""}`}
                    onClick={() => props.onToggleLauncher?.(launcher)}
                  >
                    <LauncherIcon launcher={launcher} /><span>{launcher.label}</span>
                  </button>
                  {open.has(launcher.id) && (
                    <button
                      className="workspace-tab-close"
                      onClick={() => props.onCloseLauncher?.(launcher.id)}
                      aria-label={`Close ${launcher.label}`}
                      title={`Close ${launcher.label}`}
                    ><X size={12} /></button>
                  )}
                </span>
              );
            })}
          </div>
        ) : (
          <div className="topbar-title"><span><strong>Workspace</strong><small>Persistent AI conversations</small></span></div>
        )}
      </div>
      <div className="topbar-actions flex shrink-0 items-center gap-2">
        {props.view === "settings" && props.onSaveSettings && (
          <button
            className="primary-button compact-button"
            onClick={props.onSaveSettings}
            disabled={!props.settingsDirty || props.settingsSaving || props.connection !== "online"}
          >
            <Save size={14} /> {props.settingsSaving ? "Saving…" : "Save changes"}
          </button>
        )}
        {props.view === "conversation" && props.hasConversation && (
          <button className="icon-button" onClick={props.onDetails} aria-label="Open conversation details"><PanelRight size={17} /></button>
        )}
      </div>
    </header>
  );
}
