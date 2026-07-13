import { FolderOpen, MessageSquarePlus, Plus, Settings2, X } from "lucide-react";
import { useEffect, useState } from "react";
import type {
  ConnectionStatus,
  ProjectWorkspaceInspection,
} from "../lib/protocol";
import { parseAdditionalFolders } from "../lib/sandbox-folders";

export interface CreateConversationValues {
  name: string;
  cwd?: string;
  category?: string;
  additionalFolders?: string[];
  worktree?: { baseRef?: string; environmentID?: string; sourceFolder?: string };
}

interface CreateDialogProps {
  kind: "chat" | "project";
  busy?: boolean;
  initialFolder?: string;
  initialCategory?: string;
  onClose: () => void;
  onCreate: (values: CreateConversationValues) => Promise<void>;
  onInspectWorkspace?: (folder: string) => Promise<ProjectWorkspaceInspection>;
}

export function CreateDialog(props: CreateDialogProps) {
  const [name, setName] = useState("");
  const [folder, setFolder] = useState(props.initialFolder || "");
  const [category, setCategory] = useState(props.initialCategory || "");
  const [additionalFolders, setAdditionalFolders] = useState("");
  const [workspaceMode, setWorkspaceMode] = useState<"local" | "worktree">("local");
  const [baseRef, setBaseRef] = useState("HEAD");
  const [environmentID, setEnvironmentID] = useState("");
  const [inspection, setInspection] = useState<ProjectWorkspaceInspection | null>(null);
  const [inspectedFolder, setInspectedFolder] = useState("");
  const [inspecting, setInspecting] = useState(false);
  const [inspectionError, setInspectionError] = useState("");
  useEscape(props.onClose);
  const project = props.kind === "project";
  const title = project ? "New project" : "New chat";
  const folderValue = folder.trim();
  const inspected = inspectedFolder === folderValue ? inspection : null;
  const worktreeReady = workspaceMode === "local" || Boolean(inspected?.git_repository);
  const inspectWorkspace = async () => {
    if (!folderValue || !props.onInspectWorkspace) return;
    setInspecting(true);
    setInspectionError("");
    try {
      const value = await props.onInspectWorkspace(folderValue);
      const next = {
        ...value,
        branches: value.branches ?? [],
        environments: value.environments ?? [],
      };
      setInspection(next);
      setInspectedFolder(folderValue);
      setEnvironmentID((current) => next.environments.some((environment) => environment.id === current) ? current : "");
      if (!next.git_repository) setInspectionError("Worktrees require a folder inside a Git repository.");
    } catch (cause) {
      setInspection(null);
      setInspectedFolder("");
      setInspectionError(cause instanceof Error ? cause.message : "Could not inspect this folder");
    } finally {
      setInspecting(false);
    }
  };
  return (
    <div className="modal-layer" role="dialog" aria-modal="true" aria-label={`Create ${props.kind}`}>
      <button className="modal-scrim" onClick={props.onClose} aria-label={`Close create ${props.kind}`} />
      <form
        className="modal-card"
        onSubmit={(event) => {
          event.preventDefault();
          void props.onCreate({
            name: name.trim(),
            cwd: project ? folder.trim() : undefined,
            category: project ? category.trim() : undefined,
            additionalFolders: project ? parseAdditionalFolders(additionalFolders) : undefined,
            worktree: project && workspaceMode === "worktree" ? {
              baseRef: baseRef.trim() || "HEAD",
              environmentID: environmentID || undefined,
              sourceFolder: inspected?.folder,
            } : undefined,
          });
        }}
      >
        <div className="modal-heading">
          <div className="modal-icon">{project ? <FolderOpen size={18} /> : <MessageSquarePlus size={18} />}</div>
          <div><strong>{title}</strong><span>{project ? "Give the agent a sandboxed folder" : "Start a pathless conversation"}</span></div>
          <button type="button" className="icon-button" onClick={props.onClose} aria-label="Close"><X size={17} /></button>
        </div>
        <label>
          <span>{project ? "Project name" : "Chat name"}</span>
          <input
            autoFocus
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder={project ? "My project" : "New idea"}
          />
        </label>
        {project && (
          <>
            <label>
              <span>Project category</span>
              <input
                value={category}
                onChange={(event) => setCategory(event.target.value)}
                placeholder="Client or workspace"
                aria-label="Project category"
                maxLength={80}
              />
            </label>
            <label>
              <span>Workspace</span>
              <select
                aria-label="Project workspace"
                value={workspaceMode}
                onChange={(event) => {
                  setWorkspaceMode(event.target.value as "local" | "worktree");
                  setInspectionError("");
                }}
              >
                <option value="local">Local checkout</option>
                <option value="worktree">New worktree</option>
              </select>
            </label>
            <label>
              <span>{workspaceMode === "worktree" ? "Source project folder" : "Project folder"}</span>
              <input
                value={folder}
                onChange={(event) => {
                  setFolder(event.target.value);
                  setInspection(null);
                  setInspectedFolder("");
                  setInspectionError("");
                }}
                placeholder="/absolute/path/to/project"
                spellCheck={false}
              />
            </label>
            {workspaceMode === "worktree" && (
              <>
                <div className="project-inspection-row">
                  <button
                    type="button"
                    className="secondary-button"
                    disabled={!folderValue || inspecting || props.busy}
                    onClick={() => void inspectWorkspace()}
                  >
                    {inspecting ? "Inspecting…" : "Inspect source folder"}
                  </button>
                  {inspected?.git_repository && (
                    <span>{inspected.repository_root || inspected.folder}{inspected.current_branch ? ` · ${inspected.current_branch}` : ""}</span>
                  )}
                </div>
                {inspectionError && <p className="form-error">{inspectionError}</p>}
                <label>
                  <span>Starting ref</span>
                  <input
                    list="worktree-ref-options"
                    aria-label="Starting ref"
                    value={baseRef}
                    onChange={(event) => setBaseRef(event.target.value)}
                    placeholder="HEAD"
                    spellCheck={false}
                  />
                  <datalist id="worktree-ref-options">
                    {inspected?.branches.map((branch) => <option value={branch} key={branch} />)}
                  </datalist>
                </label>
                <label>
                  <span>Local environment</span>
                  <select
                    aria-label="Local environment"
                    value={environmentID}
                    onChange={(event) => setEnvironmentID(event.target.value)}
                    disabled={!inspected?.git_repository}
                  >
                    <option value="">No environment</option>
                    {inspected?.environments.map((environment) => (
                      <option value={environment.id} key={environment.id}>{environment.name} · {environment.id}</option>
                    ))}
                  </select>
                </label>
              </>
            )}
            <label>
              <span>Additional sandbox folders</span>
              <textarea
                value={additionalFolders}
                onChange={(event) => setAdditionalFolders(event.target.value)}
                placeholder={"/absolute/path/to/shared\n/absolute/path/to/docs"}
                aria-label="Additional sandbox folders"
                rows={3}
                spellCheck={false}
              />
            </label>
          </>
        )}
        <p>{project
          ? workspaceMode === "worktree"
            ? "Dire Agent creates an isolated checkout, then runs the selected environment setup script before opening the project."
            : "The project folder remains the main working directory. Add one optional absolute folder per line."
          : "Chats retain their own SQLite history but cannot read or modify local files."}</p>
        {project && workspaceMode === "worktree" && props.busy && (
          <p className="project-creation-status" role="status">Creating the worktree and running its setup script…</p>
        )}
        <div className="modal-actions">
          <button type="button" className="secondary-button" onClick={props.onClose}>Cancel</button>
          <button
            type="submit"
            className="primary-button"
            disabled={!name.trim() || (project && !folder.trim()) || (project && !worktreeReady) || props.busy}
          >
            <Plus size={14} /> {props.busy
              ? workspaceMode === "worktree" ? "Creating worktree…" : "Creating…"
              : `Create ${props.kind}`}
          </button>
        </div>
      </form>
    </div>
  );
}

interface ConnectionDialogProps {
  endpoint: string;
  status: ConnectionStatus;
  error: string;
  onClose: () => void;
  onSave: (endpoint: string) => void;
}

export function ConnectionDialog(props: ConnectionDialogProps) {
  const [draft, setDraft] = useState(props.endpoint);
  const [validation, setValidation] = useState("");
  useEscape(props.onClose);
  const save = () => {
    const value = draft.trim();
    if (!/^wss?:\/\//.test(value)) {
      setValidation("Use a ws:// or wss:// WebSocket URL.");
      return;
    }
    props.onSave(value);
  };
  return (
    <div className="modal-layer" role="dialog" aria-modal="true" aria-label="Connection settings">
      <button className="modal-scrim" onClick={props.onClose} aria-label="Close connection settings" />
      <div className="modal-card">
        <div className="modal-heading">
          <div className="modal-icon"><Settings2 size={18} /></div>
          <div><strong>Daemon connection</strong><span>WebSocket endpoint for this browser</span></div>
          <button className="icon-button" onClick={props.onClose} aria-label="Close"><X size={17} /></button>
        </div>
        <label>
          <span>WebSocket URL</span>
          <input value={draft} onChange={(event) => setDraft(event.target.value)} spellCheck={false} />
        </label>
        <div className="connection-detail">
          <span className={`connection-dot ${props.status}`} />
          <span>{props.status === "online" ? "Connected" : props.error || "Not connected"}</span>
        </div>
        {validation && <p className="form-error">{validation}</p>}
        <p>Keep the same-origin <code>/ws</code> URL when Vite proxies your local daemon.</p>
        <div className="modal-actions">
          <button className="secondary-button" onClick={props.onClose}>Cancel</button>
          <button className="primary-button" onClick={save}>Reconnect</button>
        </div>
      </div>
    </div>
  );
}

function useEscape(onClose: () => void) {
  useEffect(() => {
    const listener = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", listener);
    return () => window.removeEventListener("keydown", listener);
  }, [onClose]);
}
