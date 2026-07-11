import {
  Activity,
  BrainCircuit,
  Check,
  CircleStop,
  Cpu,
  ImagePlus,
  MessageCirclePlus,
  PanelRight,
  Send,
  X,
  Zap,
} from "lucide-react";
import { useEffect, useState, type ClipboardEvent, type KeyboardEvent } from "react";
import { CompactSelect, type CompactSelectOption } from "../../components/CompactSelect";
import {
  completeSlashCommand,
  slashCommandSuggestions,
  type ConversationState,
} from "../../lib/conversation";
import { compactPath, mergeModelOptions, thinkingLevels, usageContextWindow } from "../../lib/display";
import {
  maxComposerImages,
  maxComposerImageTotalBytes,
  prepareComposerImage,
  type ComposerImage,
} from "../../lib/image-input";
import { emptyUsage, normalizeUsage, type Conversation, type ImageAttachment, type ModelInfo, type RuntimeState } from "../../lib/protocol";
import { Transcript } from "./Transcript";
import { UsageSummary } from "./UsageSummary";

export type SendMode = "auto" | "steer" | "follow-up";

const sendModeOptions: CompactSelectOption[] = [
  { value: "auto", label: "Auto" },
  { value: "steer", label: "Steer" },
  { value: "follow-up", label: "Follow-up" },
];

interface ConversationViewProps {
  resource: Conversation | null;
  runtime: RuntimeState | null;
  state: ConversationState;
  historyLoading: boolean;
  models: ModelInfo[];
  online: boolean;
  onSubmit: (text: string, mode: SendMode, attachments?: ImageAttachment[]) => Promise<void>;
  onAbort: () => Promise<void>;
  onModelChange: (model: string) => Promise<void>;
  onThinkingChange: (level: string) => Promise<void>;
  onOpenControls: () => void;
  onCreateChat: () => void;
  onCreateProject: () => void;
}

export function ConversationView(props: ConversationViewProps) {
  const { resource, runtime, state, historyLoading, models, online } = props;
  const [composer, setComposer] = useState("");
  const [sendMode, setSendMode] = useState<SendMode>("auto");
  const [completionIndex, setCompletionIndex] = useState(0);
  const [completionDismissed, setCompletionDismissed] = useState(false);
  const [images, setImages] = useState<ComposerImage[]>([]);
  const [imageError, setImageError] = useState("");
  const running = Boolean(runtime?.running || state.running);
  const usage = normalizeUsage(runtime?.usage || emptyUsage);
  const contextWindow = resource ? usageContextWindow(usage, models, resource.model) : 0;
  const completions = completionDismissed ? [] : slashCommandSuggestions(composer);
  const activeCompletionIndex = Math.min(completionIndex, Math.max(0, completions.length - 1));
  const selectedCompletion = completions[activeCompletionIndex];
  const modelOptions = mergeModelOptions(models, resource?.model);

  useEffect(() => {
    setCompletionIndex(0);
    setCompletionDismissed(false);
  }, [composer]);

  const submit = async () => {
    const value = composer.trim();
    if ((!value && images.length === 0) || !online || !resource || historyLoading) return;
    if (images.length > 0 && (running || sendMode !== "auto" || value.startsWith("/"))) {
      setImageError("Pasted images can only be sent with a new prompt while the project is idle.");
      return;
    }
    setComposer("");
    setImages([]);
    setImageError("");
    await props.onSubmit(value, sendMode, images.map(({ name, mime_type, data, size }) => ({ name, mime_type, data, size })));
  };

  const onPaste = async (event: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = Array.from(event.clipboardData.files).filter((file) => file.type.startsWith("image/"));
    if (files.length === 0) return;
    event.preventDefault();
    if (isChatResource(resource)) {
      setImageError("Image paste requires a folder-scoped project sandbox.");
      return;
    }
    if (running) {
      setImageError("Wait for the current run to finish before attaching an image.");
      return;
    }
    try {
      if (images.length + files.length > maxComposerImages) throw new Error(`You can attach up to ${maxComposerImages} images.`);
      const prepared = await Promise.all(files.map(prepareComposerImage));
      const total = [...images, ...prepared].reduce((sum, image) => sum + (image.size || 0), 0);
      if (total > maxComposerImageTotalBytes) throw new Error("Pasted images must total 10 MiB or less.");
      setImages((current) => [...current, ...prepared]);
      setImageError("");
    } catch (error) {
      setImageError(error instanceof Error ? error.message : "Could not paste this image.");
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (completions.length > 0) {
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        const direction = event.key === "ArrowDown" ? 1 : -1;
        setCompletionIndex((current) => (current + direction + completions.length) % completions.length);
        return;
      }
      if (event.key === "Tab" || (event.key === "Enter" && !event.shiftKey)) {
        event.preventDefault();
        if (selectedCompletion) setComposer(completeSlashCommand(selectedCompletion));
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        setCompletionDismissed(true);
        return;
      }
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void submit();
    }
  };

  if (!resource) {
    return (
      <main className="conversation-panel empty-conversation flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-[#090c10]">
        <div className="empty-hero">
          <div className="hero-orbit"><MessageCirclePlus size={26} /></div>
          <span className="eyebrow">YOUR AGENT WORKSPACE</span>
          <h1>{online ? "Start a conversation" : "Waiting for the daemon"}</h1>
          <p>
            {online
              ? "Use a standalone chat for ideas, or create a folder-scoped project when the agent needs tools."
              : "Run dire-agent start and this page will reconnect automatically."}
          </p>
          {online && (
            <div className="empty-actions">
              <button className="primary-button" onClick={props.onCreateChat}>New chat</button>
              <button className="secondary-button" onClick={props.onCreateProject}>New project</button>
            </div>
          )}
        </div>
      </main>
    );
  }

  const isChat = runtime?.kind === "chat" || resource.kind === "chat" || resource.id.startsWith("chat_");
  return (
    <main className="conversation-panel flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-[#090c10]">
      <header className="conversation-header flex shrink-0 flex-col gap-3 border-b border-white/[0.08] px-[clamp(18px,3vw,34px)] py-3">
        <div className="conversation-heading-row">
          <div>
            <div className="eyebrow">
              {isChat ? "STANDALONE CHAT" : `PROJECT · ${compactPath(resource.cwd)}`}
            </div>
            <h1>{resource.name || (isChat ? "Untitled chat" : "Unnamed project")}</h1>
          </div>
          <div className="run-summary">
            <span className={running ? "run-live" : "run-idle"}>
              {running ? <Activity className="pulse" size={14} /> : <Check size={14} />}
              {running ? "Running" : "Ready"}
            </span>
            {(state.followUpsQueued > 0 || state.steeringQueued > 0) && (
              <span className="queue-count">{state.followUpsQueued + state.steeringQueued} queued</span>
            )}
            <button className="icon-button" onClick={props.onOpenControls} aria-label="Open conversation details">
              <PanelRight size={17} />
            </button>
          </div>
        </div>
        <UsageSummary usage={usage} contextWindow={contextWindow} />
      </header>

      <Transcript
        conversationID={resource.id}
        state={state}
        loading={historyLoading}
        onPrompt={setComposer}
      />

      <footer className="composer-wrap shrink-0 border-t border-white/[0.08] bg-[#090c10]/95 px-[clamp(18px,3vw,34px)] pt-3 pb-2">
        <div className="composer mx-auto w-full max-w-[760px] rounded-xl border border-white/15 bg-[#10141a] shadow-xl focus-within:border-[#ff7657]/35">
          {completions.length > 0 && (
            <div id="slash-command-list" className="slash-completions" role="listbox" aria-label="Slash command suggestions">
              {completions.map((command, index) => (
                <button
                  key={command.name}
                  id={`slash-command-${command.name}`}
                  type="button"
                  role="option"
                  aria-selected={index === activeCompletionIndex}
                  className={index === activeCompletionIndex ? "selected" : ""}
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => setComposer(completeSlashCommand(command))}
                >
                  <code>/{command.name}</code><span>{command.description}</span>
                </button>
              ))}
            </div>
          )}
          <textarea
            value={composer}
            onChange={(event) => setComposer(event.target.value)}
            onKeyDown={onKeyDown}
            onPaste={(event) => void onPaste(event)}
            placeholder={historyLoading ? "Loading conversation history…" : running ? "Queue a follow-up, or use /steer…" : "Message the agent…"}
            aria-label="Message the agent"
            aria-expanded={completions.length > 0}
            aria-controls={completions.length > 0 ? "slash-command-list" : undefined}
            aria-activedescendant={selectedCompletion ? `slash-command-${selectedCompletion.name}` : undefined}
            rows={3}
          />
          {images.length > 0 && (
            <div className="composer-images" aria-label="Attached images">
              {images.map((image) => (
                <div className="composer-image" key={image.id}>
                  <img src={image.previewURL} alt={image.name} />
                  <span>{image.name}</span>
                  <button
                    type="button"
                    onClick={() => setImages((current) => current.filter((item) => item.id !== image.id))}
                    aria-label={`Remove ${image.name}`}
                  ><X size={12} /></button>
                </div>
              ))}
            </div>
          )}
          {imageError && <p className="composer-image-error" role="alert">{imageError}</p>}
          <div className="composer-actions">
            <span className="paste-hint" title={isChat ? "Select a project to paste images" : "Paste an image from the clipboard"}>
              <ImagePlus size={13} /> Paste image
            </span>
            <CompactSelect
              ariaLabel="Composer model"
              className="composer-model-select"
              disabled={running || !online}
              icon={<Cpu size={13} />}
              onChange={(model) => void props.onModelChange(model)}
              options={modelOptions.map((model) => ({ value: model.id, label: model.id }))}
              title="Model"
              value={resource.model}
            />
            <CompactSelect
              ariaLabel="Composer thinking level"
              disabled={running || !online}
              icon={<BrainCircuit size={13} />}
              onChange={(level) => void props.onThinkingChange(level)}
              options={thinkingLevels.map((level) => ({ value: level, label: level }))}
              title="Thinking level"
              value={resource.thinking_level}
            />
            <CompactSelect
              ariaLabel="Message behavior"
              icon={<Zap size={13} />}
              onChange={(mode) => setSendMode(mode as SendMode)}
              options={sendModeOptions}
              value={sendMode}
            />
            <span className="composer-hint">Shift + Enter for newline</span>
            {running && (
              <button className="abort-button" onClick={() => void props.onAbort()}>
                <CircleStop size={14} /> Abort
              </button>
            )}
            <button
              className="send-button"
              onClick={() => void submit()}
              disabled={(!composer.trim() && images.length === 0) || !online || historyLoading}
              aria-label="Send message"
            >
              <Send size={15} />
            </button>
          </div>
        </div>
        <p className="composer-note">
          {isChat ? "Standalone chats have no folder tools." : "Tool execution is confined to this project's main and included folders."}
        </p>
      </footer>
    </main>
  );
}

function isChatResource(resource: Conversation | null): boolean {
  return Boolean(resource && (resource.kind === "chat" || resource.id.startsWith("chat_")));
}
