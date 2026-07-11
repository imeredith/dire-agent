import { AppWindow, Command, ExternalLink } from "lucide-react";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AppSidebar, type AppView } from "./components/AppSidebar";
import { ConnectionDialog, CreateDialog, type CreateConversationValues } from "./components/Dialogs";
import { TopBar } from "./components/TopBar";
import { ConversationDrawer } from "./features/conversation/ConversationDrawer";
import { ConversationView, type SendMode } from "./features/conversation/ConversationView";
import { ProjectEnvironmentDialog } from "./features/conversation/ProjectEnvironmentDialog";
import { SettingsPage } from "./features/settings/SettingsPage";
import { useConversationSession } from "./hooks/useConversationSession";
import { useCapabilityCommands } from "./hooks/useCapabilityCommands";
import { useDaemonConnection } from "./hooks/useDaemonConnection";
import { useSettings } from "./hooks/useSettings";
import { useSubagents } from "./hooks/useSubagents";
import { parseComposerInput } from "./lib/conversation";
import { formatContext, formatTokens, usageContextWindow } from "./lib/display";
import {
  conversationKind,
  defaultWebSocketURL,
  normalizeUsage,
  type Conversation,
  type ImageAttachment,
  type ProjectLauncher,
  type WireEvent,
} from "./lib/protocol";
import {
  defaultProjectLaunchers,
  matchesLauncherShortcut,
} from "./lib/terminal";
import { readAppStorage, writeAppStorage } from "./lib/storage";

const TerminalPanel = lazy(() => import("./components/TerminalPanel").then((module) => ({ default: module.TerminalPanel })));

const helpText = `**Chat commands**

- \`/steer TEXT\` — inject guidance into the active run
- \`/follow-up TEXT\` — queue the next turn
- \`/abort\` — cancel the active run
- \`/model [MODEL]\` — show or change the model
- \`/thinking [LEVEL]\` — show or change reasoning
- \`/name [NAME]\` — show or rename this conversation
- \`/folders\` — show the main and additional sandbox folders
- \`/folder-add PATH\` — include an absolute sandbox folder
- \`/folder-remove PATH\` — remove an included sandbox folder
- \`/status\` — show model, queues and token usage
- \`/clear\` — clear this local transcript view
- \`/help\` — show this reference
- \`/quit\` — disconnect this browser client`;

function App() {
  const [endpoint, setEndpoint] = useState(() => readAppStorage("endpoint") || defaultWebSocketURL());
  const [reconnectKey, setReconnectKey] = useState(0);
  const [selectedID, setSelectedID] = useState(() =>
    readAppStorage("conversation") ||
    readAppStorage("project") ||
    readAppStorage("thread") || "");
  const [view, setView] = useState<AppView>("conversation");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [dialog, setDialog] = useState<"chat" | "project" | "connection" | "">("");
  const [environmentProject, setEnvironmentProject] = useState<Conversation | null>(null);
  const [environmentRevision, setEnvironmentRevision] = useState(0);
  const [projectLaunchers, setProjectLaunchers] = useState<ProjectLauncher[]>(defaultProjectLaunchers);
  const [openLauncherIDs, setOpenLauncherIDs] = useState<string[]>([]);
  const [activeLauncherID, setActiveLauncherID] = useState("");
  const [busy, setBusy] = useState("");
  const [notice, setNotice] = useState("");
  const eventSink = useRef<(event: WireEvent) => void>(() => undefined);
  const notify = useCallback((message: string) => {
    setNotice(message);
    window.setTimeout(() => setNotice((current) => current === message ? "" : current), 3_500);
  }, []);

  const daemon = useDaemonConnection(endpoint, eventSink, reconnectKey);
  const conversations = useMemo(
    () => [...daemon.chats, ...daemon.projects],
    [daemon.chats, daemon.projects],
  );
  const selected = conversations.find((item) => item.id === selectedID) ?? null;
  const session = useConversationSession({
    client: daemon.client,
    connection: daemon.status,
    connectionVersion: daemon.version,
    selected,
    onUpsert: daemon.upsertConversation,
    onActivity: daemon.promoteConversation,
    onNotice: notify,
  });
  const subagents = useSubagents({
    client: daemon.client,
    resource: selected,
    connectionVersion: daemon.version,
    onNotice: notify,
  });
  const capabilityCommands = useCapabilityCommands({
    client: daemon.client,
    resource: selected,
    connectionVersion: daemon.version,
  });
  eventSink.current = (event) => {
    session.handleEvent(event);
    subagents.handleEvent(event);
    capabilityCommands.handleEvent(event);
  };
  const settings = useSettings({
    client: daemon.client,
    active: view === "settings",
    connectionVersion: daemon.version,
  });
  const selectedProjectID = selected && conversationKind(selected) === "project" ? selected.id : "";

  useEffect(() => {
    // The transport reports online before the initial list requests finish.
    // Keep the persisted selection until the fully initialized client arrives.
    if (daemon.status !== "online" || !daemon.client) return;
    if (selectedID && conversations.some((item) => item.id === selectedID)) return;
    setSelectedID(conversations[0]?.id || "");
  }, [conversations, daemon.client, daemon.status, selectedID]);

  useEffect(() => {
    if (selectedID) writeAppStorage("conversation", selectedID);
  }, [selectedID]);

  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "n") {
        event.preventDefault();
        setDialog("chat");
      }
    };
    window.addEventListener("keydown", listener);
    return () => window.removeEventListener("keydown", listener);
  }, []);

  useEffect(() => {
    setOpenLauncherIDs([]);
    setActiveLauncherID("");
  }, [selectedProjectID]);

  useEffect(() => {
    if (daemon.status === "online") return;
    setOpenLauncherIDs([]);
    setActiveLauncherID("");
  }, [daemon.status]);

  useEffect(() => {
    if (!selected || !selectedProjectID || daemon.status !== "online" || !daemon.client?.isOpen) {
      if (!selectedProjectID) setProjectLaunchers(defaultProjectLaunchers);
      return;
    }
    let cancelled = false;
    void daemon.client.getProjectLaunchers(selected).then((launchers) => {
      if (!cancelled) setProjectLaunchers(launchers);
    }).catch(() => {
      // A pre-launcher daemon still supports the three legacy terminal modes.
      if (!cancelled) setProjectLaunchers(defaultProjectLaunchers);
    });
    return () => { cancelled = true; };
  }, [daemon.client, daemon.status, daemon.version, environmentRevision, selectedProjectID, settings.config?.revision]);

  useEffect(() => {
    const available = new Set(projectLaunchers.map((launcher) => launcher.id));
    setOpenLauncherIDs((current) => {
      const filtered = current.filter((id) => available.has(id));
      return filtered.length === current.length ? current : filtered;
    });
    setActiveLauncherID((current) => current && !available.has(current) ? "" : current);
  }, [projectLaunchers]);

  const closeLauncher = useCallback((launcherID: string) => {
    setOpenLauncherIDs((current) => current.filter((id) => id !== launcherID));
    setActiveLauncherID((current) => current === launcherID ? "" : current);
  }, []);

  const launchDesktopApp = useCallback(async (launcher: ProjectLauncher) => {
    if (!selected || conversationKind(selected) !== "project" || !daemon.client?.isOpen) return false;
    try {
      await daemon.client.launchProjectApp(selected, launcher.id);
      notify(`${launcher.label} opened on the daemon host`);
      return true;
    } catch (error) {
      notify(error instanceof Error ? error.message : `Could not open ${launcher.label}`);
      return false;
    }
  }, [daemon.client, notify, selected]);

  const toggleLauncher = useCallback((launcher: ProjectLauncher) => {
    if (daemon.status !== "online" || !selectedProjectID) return;
    if (activeLauncherID === launcher.id) {
      setActiveLauncherID("");
      return;
    }
    const firstOpen = !openLauncherIDs.includes(launcher.id);
    setOpenLauncherIDs((current) => current.includes(launcher.id) ? current : [...current, launcher.id]);
    setActiveLauncherID(launcher.id);
    if (launcher.kind === "desktop" && firstOpen) {
      void launchDesktopApp(launcher).then((launched) => {
        if (!launched) closeLauncher(launcher.id);
      });
    }
  }, [activeLauncherID, closeLauncher, daemon.status, launchDesktopApp, openLauncherIDs, selectedProjectID]);

  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      if (event.repeat || !selectedProjectID || view !== "conversation" || daemon.status !== "online") return;
      const launcher = projectLaunchers.find((candidate) => matchesLauncherShortcut(event, candidate.shortcut));
      if (!launcher) return;
      event.preventDefault();
      event.stopPropagation();
      toggleLauncher(launcher);
    };
    window.addEventListener("keydown", listener, true);
    return () => window.removeEventListener("keydown", listener, true);
  }, [daemon.status, projectLaunchers, selectedProjectID, toggleLauncher, view]);

  const selectConversation = (conversation: Conversation) => {
    setOpenLauncherIDs([]);
    setActiveLauncherID("");
    setSelectedID(conversation.id);
    setView("conversation");
    setSidebarOpen(false);
    setDrawerOpen(false);
  };

  const createConversation = async (values: CreateConversationValues) => {
    if (!daemon.client?.isOpen) return;
    setBusy("create");
    try {
      const created = dialog === "project"
        ? await daemon.client.createProject({
          name: values.name,
          cwd: values.cwd,
          category: values.category,
          additional_folders: values.additionalFolders,
          worktree: values.worktree ? {
            base_ref: values.worktree.baseRef,
            environment_id: values.worktree.environmentID,
            source_project_id: selected &&
              conversationKind(selected) === "project" &&
              (selected.worktree?.source_cwd || selected.cwd) ===
                (values.worktree.sourceFolder || values.cwd)
              ? selected.id
              : undefined,
          } : undefined,
        })
        : await daemon.client.createChat({ name: values.name });
      const canonical: Conversation = {
        ...created,
        kind: dialog === "project" ? "project" : "chat",
      };
      daemon.upsertConversation(canonical);
      selectConversation(canonical);
      if (values.cwd) writeAppStorage("project.folder", values.cwd);
      if (values.category) writeAppStorage("project.category", values.category);
      setDialog("");
      notify(dialog === "project" ? "Project created" : "Chat created");
    } catch (error) {
      notify(error instanceof Error ? error.message : "Could not create conversation");
    } finally {
      setBusy("");
    }
  };

  const deleteConversation = useCallback(async (resource: Conversation) => {
    const preserved = resource.worktree
      ? ` The worktree checkout at ${resource.worktree.path || resource.cwd} will be preserved, and cleanup scripts will not run.`
      : "";
    if (!window.confirm(`Delete “${resource.name || resource.id}” and its history?${preserved}`)) return;
    if (!daemon.client?.isOpen) return;
    setBusy("delete");
    try {
      await daemon.client.deleteConversation(resource);
      daemon.removeConversation(resource.id);
      if (selectedID === resource.id) {
        setSelectedID(conversations.find((item) => item.id !== resource.id)?.id || "");
      }
      setDrawerOpen(false);
      notify(`${conversationKind(resource) === "chat" ? "Chat" : "Project"} deleted`);
    } catch (error) {
      notify(error instanceof Error ? error.message : "Could not delete conversation");
    } finally {
      setBusy("");
    }
  }, [conversations, daemon.client, daemon.removeConversation, notify, selectedID]);

  const submitComposer = async (text: string, mode: SendMode, attachments: ImageAttachment[] = []) => {
    if (!session.runtime || !selected) return;
    let action;
    try {
      action = parseComposerInput(text);
    } catch (error) {
      notify(error instanceof Error ? error.message : "Invalid command");
      return;
    }
    if (action.kind === "help") return session.appendLocal({ role: "system", content: helpText });
    if (action.kind === "clear") {
      session.clearTranscript();
      notify("Local transcript cleared. Reopen the conversation to restore history.");
      return;
    }
    if (action.kind === "quit") {
      daemon.client?.close();
      notify("Disconnected from daemon");
      return;
    }
    if (action.kind === "abort") return session.abort();
    if (action.kind === "model") {
      if (!action.value) notify(`Model: ${selected.model}`);
      else await session.update({ type: "set_model", model: action.value }, "Model updated");
      return;
    }
    if (action.kind === "thinking") {
      if (!action.value) notify(`Thinking: ${selected.thinking_level}`);
      else await session.update({ type: "set_thinking_level", level: action.value }, "Thinking updated");
      return;
    }
    if (action.kind === "name") {
      if (!action.value) notify(`Name: ${selected.name || "Untitled conversation"}`);
      else await session.update({ type: "set_conversation_name", name: action.value }, "Conversation renamed");
      return;
    }
    if (action.kind === "folders") {
      if (conversationKind(selected) === "chat") {
        notify("Standalone chats have no project sandbox");
        return;
      }
      const extras = selected.additional_folders?.length
        ? selected.additional_folders.map((folder) => `- ${folder}`).join("\n")
        : "- None";
      session.appendLocal({
        role: "system",
        content: `**Main project folder**\n\n${selected.cwd}\n\n**Additional sandbox folders**\n\n${extras}`,
      });
      return;
    }
    if (action.kind === "folder-add") {
      if (conversationKind(selected) === "chat") {
        notify("Standalone chats have no project sandbox");
        return;
      }
      await session.update({
        type: "set_project_sandbox_folders",
        additional_folders: [...(selected.additional_folders ?? []), action.value],
      }, "Sandbox folder added");
      return;
    }
    if (action.kind === "folder-remove") {
      const current = selected.additional_folders ?? [];
      const next = current.filter((folder) => folder !== action.value);
      if (next.length === current.length) {
        notify("Folder not found; use /folders for canonical paths");
        return;
      }
      await session.update({
        type: "set_project_sandbox_folders",
        additional_folders: next,
      }, "Sandbox folder removed");
      return;
    }
    if (action.kind === "status") {
      const usage = normalizeUsage(session.runtime.usage);
      const window = usageContextWindow(usage, daemon.models, selected.model);
      session.appendLocal({
        role: "system",
        content: `**${selected.name || "Untitled conversation"}**\n\n${session.runtime.running ? "Running" : "Idle"} · ${selected.model} · thinking ${selected.thinking_level}\n\nInput ${formatTokens(usage.input_tokens)} · output ${formatTokens(usage.output_tokens)} · cache read ${formatTokens(usage.cache_read_tokens)} · cache write ${formatTokens(usage.cache_write_tokens)}\n\nContext ${formatContext(usage.context_tokens, window)}`,
      });
      return;
    }
    let kind = action.kind;
    if (kind === "prompt" && mode !== "auto") kind = mode;
    if (kind === "prompt" && session.runtime.running) kind = "follow-up";
    await session.send(kind, action.value, attachments);
  };

  const saveSettings = async () => {
    if (await settings.save()) notify("Configuration saved");
  };
  const openedLaunchers = openLauncherIDs
    .map((id) => projectLaunchers.find((launcher) => launcher.id === id))
    .filter((launcher): launcher is ProjectLauncher => Boolean(launcher));

  return (
    <div className="app-shell grid h-dvh min-h-0 w-full overflow-hidden bg-canvas lg:grid-cols-[270px_minmax(0,1fr)]">
      <AppSidebar
        open={sidebarOpen}
        endpoint={endpoint}
        connection={daemon.status}
        chats={daemon.chats}
        projects={daemon.projects}
        selectedID={selectedID}
        view={view}
        onClose={() => setSidebarOpen(false)}
        onSelect={selectConversation}
        onSettings={() => { setView("settings"); setSidebarOpen(false); setDrawerOpen(false); }}
        onCreateChat={() => setDialog("chat")}
        onCreateProject={() => setDialog("project")}
        onDelete={(resource) => void deleteConversation(resource)}
        onConnection={() => setDialog("connection")}
      />
      <section className="app-main flex min-h-0 min-w-0 flex-col overflow-hidden">
        <TopBar
          view={view}
          connection={daemon.status}
          settingsDirty={settings.dirty}
          settingsSaving={settings.saving}
          hasConversation={Boolean(selected)}
          terminalAvailable={Boolean(selected && conversationKind(selected) === "project" && daemon.status === "online")}
          launchers={projectLaunchers}
          openLauncherIDs={openLauncherIDs}
          activeLauncherID={activeLauncherID}
          onMenu={() => setSidebarOpen(true)}
          onDetails={() => setDrawerOpen(true)}
          onShowConversation={() => setActiveLauncherID("")}
          onToggleLauncher={toggleLauncher}
          onCloseLauncher={closeLauncher}
          onSaveSettings={() => void saveSettings()}
        />
        {view === "settings" ? (
          <SettingsPage controller={settings} online={daemon.status === "online"} onSaved={() => notify("Configuration saved")} />
        ) : (
          <div className="terminal-workspace-panel">
            <div
              className={`workspace-content-panel${activeLauncherID ? "" : " workspace-content-panel-active"}`}
              role="tabpanel"
              aria-hidden={Boolean(activeLauncherID)}
              aria-label="Project conversation"
            >
              <ConversationView
                resource={selected}
                runtime={session.runtime}
                state={session.conversation}
                historyLoading={session.historyLoading}
                models={daemon.models}
                online={daemon.status === "online"}
                onSubmit={submitComposer}
                onAbort={session.abort}
                onModelChange={async (model) => {
                  await session.update({ type: "set_model", model }, "Model updated");
                }}
                onThinkingChange={async (level) => {
                  await session.update({ type: "set_thinking_level", level }, "Thinking updated");
                }}
                onOpenControls={() => setDrawerOpen(true)}
                onCreateChat={() => setDialog("chat")}
                onCreateProject={() => setDialog("project")}
              />
            </div>
            {selected && conversationKind(selected) === "project" && openedLaunchers.map((launcher) => (
              launcher.kind === "terminal" ? (
                <Suspense
                  key={launcher.id}
                  fallback={activeLauncherID === launcher.id ? <div className="workspace-panel-loading" role="status">Opening {launcher.label}…</div> : null}
                >
                  <TerminalPanel
                    endpoint={endpoint}
                    project={selected}
                    launcher={launcher}
                    active={activeLauncherID === launcher.id}
                  />
                </Suspense>
              ) : (
                <section
                  key={launcher.id}
                  className={`desktop-launcher-panel${activeLauncherID === launcher.id ? " workspace-content-panel-active" : ""}`}
                  role="tabpanel"
                  aria-hidden={activeLauncherID !== launcher.id}
                  aria-label={`${launcher.label} desktop application`}
                >
                  <div className="desktop-launcher-card">
                    <div className="desktop-launcher-icon"><AppWindow size={24} /></div>
                    <span className="eyebrow">DAEMON HOST</span>
                    <h2>{launcher.label}</h2>
                    <p>This desktop application was launched in <code>{selected.cwd}</code> on the machine running Dire Agent.</p>
                    <button className="primary-button" onClick={() => void launchDesktopApp(launcher)}>
                      <ExternalLink size={14} /> Launch again
                    </button>
                  </div>
                </section>
              )
            ))}
          </div>
        )}
      </section>

      <ConversationDrawer
        open={drawerOpen && view === "conversation"}
        resource={selected}
        runtime={session.runtime}
        capabilities={session.capabilities}
        models={daemon.models}
        tools={daemon.tools}
        subagents={subagents}
        capabilityCommands={capabilityCommands}
        onClose={() => setDrawerOpen(false)}
        onUpdate={session.update}
        onDelete={deleteConversation}
        onManageEnvironments={(project) => setEnvironmentProject(project)}
      />

      {(dialog === "chat" || dialog === "project") && (
        <CreateDialog
          kind={dialog}
          busy={busy === "create"}
          initialFolder={readAppStorage("project.folder") || ""}
          initialCategory={readAppStorage("project.category") || ""}
          onClose={() => setDialog("")}
          onCreate={createConversation}
          onInspectWorkspace={(folder) => {
            if (!daemon.client?.isOpen) return Promise.reject(new Error("Daemon is not connected"));
            return daemon.client.inspectProjectWorkspace(folder);
          }}
        />
      )}
      {environmentProject && daemon.client?.isOpen && (
        <ProjectEnvironmentDialog
          client={daemon.client}
          project={environmentProject}
          onClose={() => setEnvironmentProject(null)}
          onNotice={notify}
          onChanged={() => setEnvironmentRevision((current) => current + 1)}
        />
      )}
      {dialog === "connection" && (
        <ConnectionDialog
          endpoint={endpoint}
          status={daemon.status}
          error={daemon.error}
          onClose={() => setDialog("")}
          onSave={(value) => {
            writeAppStorage("endpoint", value);
            setEndpoint(value);
            setReconnectKey((current) => current + 1);
            setDialog("");
          }}
        />
      )}
      {notice && <div className="toast fixed right-5 bottom-5 z-[100] flex max-w-[min(420px,calc(100vw-32px))] items-center gap-2 rounded-lg border border-white/15 bg-slate-800 px-3 py-2.5 text-xs shadow-2xl" role="status"><Command size={15} /> {notice}</div>}
    </div>
  );
}

export default App;
