import { CalendarClock, Save, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import {
  conversationKind,
  type Conversation,
  type ScheduledPrompt,
  type ScheduledPromptInput,
  type ScheduledPromptSchedule,
  type ScheduledPromptTarget,
} from "../../lib/protocol";

interface ScheduleDialogProps {
  schedule?: ScheduledPrompt | null;
  initialTarget?: Conversation | null;
  projects: Conversation[];
  chats: Conversation[];
  busy?: boolean;
  onClose: () => void;
  onSave: (input: ScheduledPromptInput) => Promise<boolean>;
}

const cronPresets = [
  ["", "Custom expression"],
  ["0 * * * *", "Every hour"],
  ["0 9 * * *", "Every day at 9:00"],
  ["0 9 * * 1-5", "Weekdays at 9:00"],
  ["0 9 * * 1", "Every Monday at 9:00"],
] as const;

const cronAliases = new Set([
  "@hourly",
  "@daily",
  "@midnight",
  "@weekly",
  "@monthly",
  "@yearly",
  "@annually",
]);

export function browserTimezone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

function defaultRunAt(): string {
  const next = new Date(Date.now() + 60 * 60 * 1_000);
  next.setSeconds(0, 0);
  const local = new Date(next.getTime() - next.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function zonedParts(date: Date, timezone: string): Record<string, number> {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  }).formatToParts(date);
  return Object.fromEntries(parts
    .filter((part) => part.type !== "literal")
    .map((part) => [part.type, Number(part.value)]));
}

function timezoneOffset(date: Date, timezone: string): number {
  const parts = zonedParts(date, timezone);
  const represented = Date.UTC(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute, parts.second);
  return Math.round((represented - date.getTime()) / 60_000) * 60_000;
}

export function zonedDateTimeToISO(value: string, timezone: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value);
  if (!match) throw new Error("Choose a valid date and time.");
  const localUTC = Date.UTC(
    Number(match[1]),
    Number(match[2]) - 1,
    Number(match[3]),
    Number(match[4]),
    Number(match[5]),
  );
  let instant = localUTC - timezoneOffset(new Date(localUTC), timezone);
  instant = localUTC - timezoneOffset(new Date(instant), timezone);
  return new Date(instant).toISOString();
}

export function isoToDateTimeLocal(value: string | undefined, timezone: string): string {
  if (!value) return defaultRunAt();
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return defaultRunAt();
  try {
    const parts = zonedParts(date, timezone);
    const pad = (number: number) => String(number).padStart(2, "0");
    return `${parts.year}-${pad(parts.month)}-${pad(parts.day)}T${pad(parts.hour)}:${pad(parts.minute)}`;
  } catch {
    return defaultRunAt();
  }
}

function targetValue(schedule: ScheduledPrompt | null | undefined, initial: Conversation | null | undefined): string {
  if (schedule?.target_type === "one_off") return "one_off";
  if (schedule?.conversation_id) return `${schedule.target_type}:${schedule.conversation_id}`;
  if (initial) return `${conversationKind(initial)}:${initial.id}`;
  return "one_off";
}

function scheduleTarget(value: string): { target_type: ScheduledPromptTarget; conversation_id?: string } {
  if (value === "one_off") return { target_type: "one_off" };
  const separator = value.indexOf(":");
  return {
    target_type: value.slice(0, separator) as ScheduledPromptTarget,
    conversation_id: value.slice(separator + 1),
  };
}

export function ScheduleDialog(props: ScheduleDialogProps) {
  const existing = props.schedule;
  const initialTimezone = existing?.timezone || browserTimezone();
  const [name, setName] = useState(existing?.name || "");
  const [prompt, setPrompt] = useState(existing?.prompt || "");
  const [target, setTarget] = useState(() => targetValue(existing, props.initialTarget));
  const [scheduleType, setScheduleType] = useState<ScheduledPromptSchedule>(existing?.schedule_type || "cron");
  const [cron, setCron] = useState(existing?.cron || "0 9 * * 1-5");
  const [timezone, setTimezone] = useState(initialTimezone);
  const [runAt, setRunAt] = useState(() => isoToDateTimeLocal(existing?.run_at, initialTimezone));
  const [enabled, setEnabled] = useState(existing?.enabled ?? true);
  const [validation, setValidation] = useState("");
  const missingTarget = useMemo(() => {
    if (!existing?.conversation_id) return null;
    const resources = [...props.projects, ...props.chats];
    return resources.some((item) => item.id === existing.conversation_id)
      ? null
      : `${existing.target_type}:${existing.conversation_id}`;
  }, [existing, props.chats, props.projects]);

  useEffect(() => {
    const close = (event: KeyboardEvent) => event.key === "Escape" && props.onClose();
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [props.onClose]);

  const save = async () => {
    const normalizedName = name.trim();
    const normalizedPrompt = prompt.trim();
    const normalizedTimezone = timezone.trim();
    const targetFields = scheduleTarget(target);
    if (!normalizedName) return setValidation("Give this scheduled prompt a name.");
    if (!normalizedPrompt) return setValidation("Enter the prompt to run.");
    if (targetFields.target_type !== "one_off" && !targetFields.conversation_id) {
      return setValidation("Choose a project or standalone chat.");
    }
    if (!normalizedTimezone) return setValidation("Enter an IANA timezone such as Pacific/Auckland.");
    try {
      // This also validates the timezone before the request reaches the daemon.
      new Intl.DateTimeFormat("en", { timeZone: normalizedTimezone }).format();
    } catch {
      return setValidation("Enter a valid IANA timezone such as Pacific/Auckland.");
    }
    const normalizedCron = cron.trim().toLowerCase();
    if (scheduleType === "cron" && !cronAliases.has(normalizedCron) && normalizedCron.split(/\s+/).length !== 5) {
      return setValidation("Use a five-field cron expression or a supported @alias.");
    }
    let normalizedRunAt: string | undefined;
    if (scheduleType === "once") {
      try {
        normalizedRunAt = zonedDateTimeToISO(runAt, normalizedTimezone);
      } catch (cause) {
        return setValidation(cause instanceof Error ? cause.message : "Choose a valid date and time.");
      }
    }
    setValidation("");
    await props.onSave({
      name: normalizedName,
      prompt: normalizedPrompt,
      ...targetFields,
      schedule_type: scheduleType,
      ...(scheduleType === "cron" ? { cron: normalizedCron } : {}),
      timezone: normalizedTimezone,
      ...(normalizedRunAt ? { run_at: normalizedRunAt } : {}),
      enabled,
    });
  };

  return (
    <div className="modal-layer" role="dialog" aria-modal="true" aria-label={existing ? "Edit scheduled prompt" : "Create scheduled prompt"}>
      <button className="modal-scrim" onClick={props.onClose} aria-label="Close scheduled prompt" />
      <form className="modal-card schedule-dialog" onSubmit={(event) => { event.preventDefault(); void save(); }}>
        <div className="modal-heading">
          <div className="modal-icon"><CalendarClock size={18} /></div>
          <div>
            <strong>{existing ? "Edit scheduled prompt" : "New scheduled prompt"}</strong>
            <span>Run a prompt later or on a recurring schedule</span>
          </div>
          <button type="button" className="icon-button" onClick={props.onClose} aria-label="Close"><X size={17} /></button>
        </div>

        <div className="schedule-dialog-grid">
          <label className="schedule-field-wide">
            <span>Name</span>
            <input autoFocus maxLength={120} value={name} onChange={(event) => setName(event.target.value)} placeholder="Weekday status review" />
          </label>
          <label className="schedule-field-wide">
            <span>Prompt</span>
            <textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} rows={5} placeholder="Review the project and summarize anything that needs attention." />
          </label>
          <label className="schedule-field-wide">
            <span>Target</span>
            <select aria-label="Target" value={target} onChange={(event) => setTarget(event.target.value)}>
              <option value="one_off">Fresh one-off standalone chat</option>
              {props.projects.length > 0 && (
                <optgroup label="Projects">
                  {props.projects.map((project) => <option key={project.id} value={`project:${project.id}`}>{project.name || project.id}</option>)}
                </optgroup>
              )}
              {props.chats.length > 0 && (
                <optgroup label="Standalone chats">
                  {props.chats.map((chat) => <option key={chat.id} value={`chat:${chat.id}`}>{chat.name || chat.id}</option>)}
                </optgroup>
              )}
              {missingTarget && <option value={missingTarget}>Unavailable target · {existing?.conversation_id}</option>}
            </select>
            <small>{target === "one_off" ? "Each run creates a new pathless chat without project file tools." : "Runs in the selected conversation and keeps its context."}</small>
          </label>
          <label>
            <span>Schedule type</span>
            <select aria-label="Schedule type" value={scheduleType} onChange={(event) => setScheduleType(event.target.value as ScheduledPromptSchedule)}>
              <option value="cron">Recurring cron</option>
              <option value="once">Run once</option>
            </select>
          </label>
          <label>
            <span>Timezone</span>
            <input aria-label="Timezone" list="schedule-timezones" value={timezone} onChange={(event) => setTimezone(event.target.value)} spellCheck={false} />
            <datalist id="schedule-timezones">
              <option value="UTC" />
              <option value="Pacific/Auckland" />
              <option value="America/Los_Angeles" />
              <option value="America/New_York" />
              <option value="Europe/London" />
            </datalist>
          </label>
          {scheduleType === "cron" ? (
            <>
              <label>
                <span>Common schedule</span>
                <select aria-label="Common schedule" value={cronPresets.some(([value]) => value === cron) ? cron : ""} onChange={(event) => event.target.value && setCron(event.target.value)}>
                  {cronPresets.map(([value, label]) => <option value={value} key={label}>{label}</option>)}
                </select>
              </label>
              <label>
                <span>Cron expression</span>
                <input aria-label="Cron expression" value={cron} onChange={(event) => setCron(event.target.value)} placeholder="0 9 * * 1-5" spellCheck={false} />
                <small>minute · hour · day · month · weekday; aliases include @hourly, @daily/@midnight, @weekly, @monthly, @yearly/@annually</small>
              </label>
            </>
          ) : (
            <label className="schedule-field-wide">
              <span>Run at</span>
              <input aria-label="Run at" type="datetime-local" value={runAt} onChange={(event) => setRunAt(event.target.value)} />
              <small>This wall-clock time uses {timezone || "the selected timezone"}.</small>
            </label>
          )}
        </div>

        <label className="schedule-enabled-toggle">
          <span><strong>Enabled</strong><small>Disabled prompts keep their configuration but do not run.</small></span>
          <input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
        </label>
        {validation && <p className="form-error" role="alert">{validation}</p>}
        <div className="modal-actions">
          <button type="button" className="secondary-button" onClick={props.onClose}>Cancel</button>
          <button type="submit" className="primary-button" disabled={props.busy}>
            <Save size={14} /> {props.busy ? "Saving…" : existing ? "Save scheduled prompt" : "Create scheduled prompt"}
          </button>
        </div>
      </form>
    </div>
  );
}
