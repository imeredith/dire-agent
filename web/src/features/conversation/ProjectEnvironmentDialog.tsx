import { Braces, Plus, Save, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { DaemonClient } from "../../lib/daemon-client";
import type {
  Conversation,
  ProjectEnvironment,
  ProjectEnvironmentAction,
  ProjectEnvironmentLifecycle,
  ProjectEnvironmentPlatform,
} from "../../lib/protocol";

interface ProjectEnvironmentDialogProps {
  client: DaemonClient;
  project: Conversation;
  onClose: () => void;
  onNotice: (message: string) => void;
  onChanged: () => void;
}

const platforms: Array<{ id: "default" | ProjectEnvironmentPlatform; label: string }> = [
  { id: "default", label: "Default" },
  { id: "darwin", label: "macOS" },
  { id: "linux", label: "Linux" },
  { id: "win32", label: "Windows" },
];

export function ProjectEnvironmentDialog(props: ProjectEnvironmentDialogProps) {
  const [environments, setEnvironments] = useState<ProjectEnvironment[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [draft, setDraft] = useState<ProjectEnvironment | null>(null);
  const [creating, setCreating] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const next = await props.client.getProjectEnvironments(props.project);
      setEnvironments(next ?? []);
      if (!draft && next?.[0]) {
        setSelectedID(next[0].id);
        setDraft(cloneEnvironment(next[0]));
      }
    } catch (cause) {
      setError(errorMessage(cause, "Could not load local environments"));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // The dialog is remounted for each project; preserve in-progress edits during refreshes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.client, props.project.id]);

  useEffect(() => {
    const close = (event: KeyboardEvent) => event.key === "Escape" && props.onClose();
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [props.onClose]);

  const selected = environments.find((environment) => environment.id === selectedID);
  const incompleteAction = draft?.actions.some((action) => !action.name.trim() || !action.command.trim()) ?? false;
  const dirty = useMemo(() => {
    if (!draft) return false;
    if (creating) return true;
    return JSON.stringify(cleanEnvironment(draft)) !== JSON.stringify(selected ? cleanEnvironment(selected) : null);
  }, [creating, draft, selected]);

  const select = (environment: ProjectEnvironment) => {
    if (dirty && !window.confirm("Discard unsaved environment changes?")) return;
    setCreating(false);
    setSelectedID(environment.id);
    setDraft(cloneEnvironment(environment));
    setError("");
  };

  const add = () => {
    if (dirty && !window.confirm("Discard unsaved environment changes?")) return;
    const id = availableEnvironmentID(environments);
    setCreating(true);
    setSelectedID(id);
    setDraft(emptyEnvironment(id));
    setError("");
  };

  const save = async () => {
    if (!draft || !draft.id.trim() || !draft.name.trim() || incompleteAction) return;
    setSaving(true);
    setError("");
    try {
      const candidate = cleanEnvironment({
        ...draft,
        id: draft.id.trim(),
        name: draft.name.trim(),
      });
      const saved = await props.client.putProjectEnvironment(
        props.project,
        candidate,
        creating ? undefined : selected?.hash,
      );
      setEnvironments((current) => {
        const exists = current.some((environment) => environment.id === saved.id);
        return exists
          ? current.map((environment) => environment.id === saved.id ? saved : environment)
          : [...current, saved];
      });
      setSelectedID(saved.id);
      setDraft(cloneEnvironment(saved));
      setCreating(false);
      props.onNotice("Local environment saved");
      props.onChanged();
    } catch (cause) {
      setError(errorMessage(cause, "Could not save local environment"));
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    if (!selected || !window.confirm(`Delete ${selected.name}?`)) return;
    setSaving(true);
    setError("");
    try {
      await props.client.deleteProjectEnvironment(props.project, selected.id, selected.hash);
      const next = environments.filter((environment) => environment.id !== selected.id);
      setEnvironments(next);
      setSelectedID(next[0]?.id ?? "");
      setDraft(next[0] ? cloneEnvironment(next[0]) : null);
      setCreating(false);
      props.onNotice("Local environment deleted");
      props.onChanged();
    } catch (cause) {
      setError(errorMessage(cause, "Could not delete local environment"));
    } finally {
      setSaving(false);
    }
  };

  const updateLifecycle = (
    key: "setup" | "cleanup",
    platform: "default" | ProjectEnvironmentPlatform,
    script: string,
  ) => {
    if (!draft) return;
    const lifecycle = { ...(draft[key] ?? { script: "" }) };
    if (platform === "default") lifecycle.script = script;
    else if (script) lifecycle[platform] = { script };
    else delete lifecycle[platform];
    setDraft({ ...draft, [key]: lifecycle });
  };

  const updateAction = (index: number, next: ProjectEnvironmentAction) => {
    if (!draft) return;
    setDraft({
      ...draft,
      actions: draft.actions.map((action, current) => current === index ? next : action),
    });
  };

  return (
    <div className="modal-layer" role="dialog" aria-modal="true" aria-label="Local environments">
      <button className="modal-scrim" onClick={props.onClose} aria-label="Close local environments" />
      <div className="modal-card environment-modal">
        <div className="environment-modal-header">
          <div className="modal-heading">
            <div className="modal-icon"><Braces size={18} /></div>
            <div>
              <strong>Local environments</strong>
              <span>Worktree setup and project actions · {environmentSource(props.project)}</span>
            </div>
            <button type="button" className="icon-button" onClick={props.onClose} aria-label="Close"><X size={17} /></button>
          </div>
          {error && <p className="form-error" role="alert">{error}</p>}
        </div>

        <div className="environment-layout">
          <aside className="environment-list" aria-label="Configured local environments">
            <button type="button" className="secondary-button full-width" onClick={add} disabled={saving}>
              <Plus size={14} /> Add environment
            </button>
            {loading && <p className="quiet-copy">Loading environments…</p>}
            {!loading && environments.map((environment) => (
              <button
                type="button"
                key={environment.id}
                className={`environment-list-item${!creating && selectedID === environment.id ? " selected" : ""}`}
                onClick={() => select(environment)}
              >
                <strong>{environment.name}</strong>
                <small>{environment.id}</small>
              </button>
            ))}
            {!loading && !environments.length && !creating && (
              <p className="quiet-copy">No environment files yet. Add one to set up new worktrees.</p>
            )}
          </aside>

          <section className="environment-editor" aria-label="Local environment editor">
            {draft ? (
              <>
                <div className="settings-grid two">
                  <label className="settings-field">
                    <span>Environment name</span>
                    <input
                      aria-label="Environment name"
                      value={draft.name}
                      onChange={(event) => setDraft({ ...draft, name: event.target.value })}
                      placeholder="Development"
                    />
                  </label>
                  <label className="settings-field">
                    <span>File name</span>
                    <input
                      aria-label="Environment file name"
                      value={draft.id}
                      onChange={(event) => setDraft({ ...draft, id: event.target.value })}
                      disabled={!creating}
                      placeholder="environment.toml"
                      spellCheck={false}
                    />
                    <small>Stored under .codex/environments in the source project folder.</small>
                  </label>
                </div>

                <LifecycleEditor
                  title="Setup scripts"
                  hint="Runs after Dire Agent creates a new worktree. Platform scripts override the default."
                  value={draft.setup}
                  onChange={(platform, script) => updateLifecycle("setup", platform, script)}
                />
                <LifecycleEditor
                  title="Cleanup scripts"
                  hint="Stored for compatible or manual cleanup flows. Deleting a project does not run cleanup or remove its checkout."
                  value={draft.cleanup}
                  onChange={(platform, script) => updateLifecycle("cleanup", platform, script)}
                />

                <div className="environment-editor-heading">
                  <div><strong>Project actions</strong><small>Quick terminal commands exposed with this environment.</small></div>
                  <button
                    type="button"
                    className="secondary-button"
                    onClick={() => setDraft({ ...draft, actions: [...draft.actions, emptyAction()] })}
                  >
                    <Plus size={13} /> Add action
                  </button>
                </div>
                <div className="integration-stack environment-actions">
                  {draft.actions.map((action, index) => (
                    <ActionEditor
                      key={action.id || index}
                      action={action}
                      index={index}
                      onChange={(next) => updateAction(index, next)}
                      onRemove={() => setDraft({ ...draft, actions: draft.actions.filter((_, current) => current !== index) })}
                    />
                  ))}
                  {!draft.actions.length && <p className="quiet-copy">No project actions configured.</p>}
                </div>
                {incompleteAction && <p className="form-error">Every action needs both a name and command.</p>}
              </>
            ) : (
              <div className="integration-empty">
                <Braces size={21} />
                <strong>Select or add an environment</strong>
                  <span>Environment files can be committed and shared with the source project.</span>
              </div>
            )}
          </section>
        </div>

        <div className="modal-actions environment-modal-actions">
          {!creating && selected && (
            <button type="button" className="danger-button" onClick={() => void remove()} disabled={saving}>
              <Trash2 size={14} /> Delete
            </button>
          )}
          <span />
          <button type="button" className="secondary-button" onClick={props.onClose}>Close</button>
          <button
            type="button"
            className="primary-button"
            onClick={() => void save()}
            disabled={!draft || !dirty || !draft.id.trim() || !draft.name.trim() || incompleteAction || saving}
          >
            <Save size={14} /> {saving ? "Saving…" : "Save environment"}
          </button>
        </div>
      </div>
    </div>
  );
}

function LifecycleEditor(props: {
  title: string;
  hint: string;
  value?: ProjectEnvironmentLifecycle;
  onChange: (platform: "default" | ProjectEnvironmentPlatform, script: string) => void;
}) {
  return (
    <section className="environment-script-section">
      <div className="environment-editor-heading"><div><strong>{props.title}</strong><small>{props.hint}</small></div></div>
      <div className="settings-grid two">
        {platforms.map((platform) => (
          <label className="settings-field" key={platform.id}>
            <span>{platform.label}</span>
            <textarea
              aria-label={`${props.title} ${platform.label}`}
              rows={platform.id === "default" ? 4 : 3}
              value={platform.id === "default" ? props.value?.script ?? "" : props.value?.[platform.id]?.script ?? ""}
              onChange={(event) => props.onChange(platform.id, event.target.value)}
              placeholder={platform.id === "default" ? "npm install\nnpm run build" : "Optional override"}
              spellCheck={false}
            />
          </label>
        ))}
      </div>
    </section>
  );
}

function ActionEditor(props: {
  action: ProjectEnvironmentAction;
  index: number;
  onChange: (action: ProjectEnvironmentAction) => void;
  onRemove: () => void;
}) {
  const { action } = props;
  const set = <K extends keyof ProjectEnvironmentAction>(key: K, value: ProjectEnvironmentAction[K]) => {
    props.onChange({ ...action, [key]: value });
  };
  return (
    <article className="integration-card compact" aria-label={`Environment action ${props.index + 1}`}>
      <header>
        <div className="integration-icon"><Braces size={15} /></div>
        <div><strong>{action.name || `Action ${props.index + 1}`}</strong><span>integrated terminal action</span></div>
        <button type="button" className="icon-button danger-icon" onClick={props.onRemove} aria-label={`Remove action ${props.index + 1}`}>
          <Trash2 size={14} />
        </button>
      </header>
      <div className="settings-grid three">
        <label className="settings-field">
          <span>Name</span>
          <input aria-label={`Action ${props.index + 1} name`} value={action.name} onChange={(event) => set("name", event.target.value)} />
        </label>
        <label className="settings-field">
          <span>Icon</span>
          <select
            aria-label={`Action ${props.index + 1} icon`}
            value={action.icon ?? "tool"}
            onChange={(event) => set("icon", event.target.value as ProjectEnvironmentAction["icon"])}
          >
            <option value="tool">Tool</option><option value="run">Run</option><option value="debug">Debug</option><option value="test">Test</option>
          </select>
        </label>
        <label className="settings-field">
          <span>Platform</span>
          <select
            aria-label={`Action ${props.index + 1} platform`}
            value={action.platform ?? ""}
            onChange={(event) => set("platform", (event.target.value || undefined) as ProjectEnvironmentAction["platform"])}
          >
            <option value="">All platforms</option><option value="darwin">macOS</option><option value="linux">Linux</option><option value="win32">Windows</option>
          </select>
        </label>
        <label className="settings-field wide">
          <span>Command</span>
          <textarea
            aria-label={`Action ${props.index + 1} command`}
            rows={3}
            value={action.command}
            onChange={(event) => set("command", event.target.value)}
            placeholder="npm test"
            spellCheck={false}
          />
        </label>
      </div>
    </article>
  );
}

function emptyEnvironment(id: string): ProjectEnvironment {
  return { id, version: 1, name: "Development", setup: { script: "" }, actions: [] };
}

function emptyAction(): ProjectEnvironmentAction {
  const id = globalThis.crypto?.randomUUID?.() ?? `action-${Date.now()}`;
  return { id, name: "", icon: "tool", command: "" };
}

function cloneEnvironment(environment: ProjectEnvironment): ProjectEnvironment {
  return JSON.parse(JSON.stringify(environment)) as ProjectEnvironment;
}

function cleanEnvironment(environment: ProjectEnvironment): ProjectEnvironment {
  const cleanup = cleanLifecycle(environment.cleanup);
  return {
    ...environment,
    setup: cleanLifecycle(environment.setup) ?? { script: "" },
    ...(cleanup ? { cleanup } : { cleanup: undefined }),
    actions: environment.actions.map((action) => ({
      ...action,
      name: action.name.trim(),
      command: action.command.trim(),
      ...(action.platform ? {} : { platform: undefined }),
    })),
  };
}

function cleanLifecycle(value?: ProjectEnvironmentLifecycle): ProjectEnvironmentLifecycle | undefined {
  if (!value) return undefined;
  const next: ProjectEnvironmentLifecycle = { script: value.script ?? "" };
  for (const platform of ["darwin", "linux", "win32"] as const) {
    const script = value[platform]?.script ?? "";
    if (script) next[platform] = { script };
  }
  if (!next.script && !next.darwin && !next.linux && !next.win32) return undefined;
  return next;
}

function availableEnvironmentID(environments: ProjectEnvironment[]): string {
  const used = new Set(environments.map((environment) => environment.id));
  if (!used.has("environment.toml")) return "environment.toml";
  for (let suffix = 2; ; suffix += 1) {
    const candidate = `environment-${suffix}.toml`;
    if (!used.has(candidate)) return candidate;
  }
}

function environmentSource(project: Conversation): string {
  return project.worktree?.source_cwd || project.cwd || "project folder";
}

function errorMessage(cause: unknown, fallback: string): string {
  return cause instanceof Error ? cause.message : fallback;
}
