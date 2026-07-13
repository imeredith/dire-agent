import {
  AlertTriangle,
  CalendarClock,
  Clock3,
  History,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Trash2,
} from "lucide-react";
import type { SchedulesController } from "../../hooks/useSchedules";
import { scheduledPromptInput } from "../../hooks/useSchedules";
import type { Conversation, ScheduledPrompt } from "../../lib/protocol";

interface SchedulesPageProps {
  controller: SchedulesController;
  projects: Conversation[];
  chats: Conversation[];
  online: boolean;
  onCreate: () => void;
  onEdit: (schedule: ScheduledPrompt) => void;
  onOpenConversation: (conversation: Conversation) => void;
}

export function formatScheduleInstant(value: string | undefined, timezone?: string): string {
  if (!value) return "Not yet";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  try {
    return new Intl.DateTimeFormat(undefined, {
      timeZone: timezone || undefined,
      dateStyle: "medium",
      timeStyle: "short",
    }).format(date);
  } catch {
    return date.toLocaleString();
  }
}

function targetLabel(schedule: ScheduledPrompt, projects: Conversation[], chats: Conversation[]): string {
  if (schedule.target_type === "one_off") return "Fresh one-off standalone chat";
  const resources = schedule.target_type === "project" ? projects : chats;
  const target = resources.find((item) => item.id === schedule.conversation_id);
  return target?.name || schedule.conversation_id || "Unavailable target";
}

function scheduleLabel(schedule: ScheduledPrompt): string {
  if (schedule.schedule_type === "cron") return `${schedule.cron || "Invalid cron"} · ${schedule.timezone}`;
  return `Once · ${formatScheduleInstant(schedule.run_at, schedule.timezone)} · ${schedule.timezone}`;
}

function statusLabel(schedule: ScheduledPrompt): string {
  if (schedule.last_error) return schedule.last_error;
  if (schedule.last_status) return schedule.last_status.replaceAll("_", " ");
  return "Never run";
}

function ResultConversation(props: {
  schedule: ScheduledPrompt;
  projects: Conversation[];
  chats: Conversation[];
  onOpen: (conversation: Conversation) => void;
}) {
  if (!props.schedule.last_conversation_id) return null;
  const conversation = [...props.chats, ...props.projects]
    .find((item) => item.id === props.schedule.last_conversation_id);
  return (
    <div>
      <span>Result conversation</span>
      {conversation ? (
        <button className="schedule-result-button" onClick={() => props.onOpen(conversation)}>Open result</button>
      ) : (
        <strong title={props.schedule.last_conversation_id}>{props.schedule.last_conversation_id}</strong>
      )}
    </div>
  );
}

export function SchedulesPage(props: SchedulesPageProps) {
  const { controller } = props;
  if (!props.online) {
    return (
      <main className="schedules-page schedules-state">
        <CalendarClock size={23} />
        <h1>Scheduled prompts unavailable</h1>
        <p>Reconnect to the daemon to manage background tasks.</p>
      </main>
    );
  }
  if (!controller.supported) {
    return (
      <main className="schedules-page schedules-state">
        <AlertTriangle size={23} />
        <h1>Scheduled prompts unsupported</h1>
        <p>Upgrade the daemon to use scheduled prompts.</p>
      </main>
    );
  }
  return (
    <main className="schedules-page">
      <div className="schedules-content">
        <header className="schedules-hero">
          <div>
            <span className="eyebrow">BACKGROUND TASKS</span>
            <h1>Scheduled prompts</h1>
            <p>Run recurring project work, continue a standalone chat, or create a fresh one-off chat at the scheduled time.</p>
          </div>
          <button className="primary-button" onClick={props.onCreate}><Plus size={14} /> New scheduled prompt</button>
        </header>

        {controller.error && (
          <div className="schedules-alert" role="alert">
            <AlertTriangle size={16} />
            <span>{controller.error}</span>
            <button className="secondary-button" onClick={() => void controller.reload()}><RefreshCw size={13} /> Retry</button>
          </div>
        )}

        {controller.loading && !controller.schedules.length ? (
          <div className="schedules-loading" role="status"><RefreshCw className="spin" size={19} /> Loading scheduled prompts…</div>
        ) : controller.schedules.length ? (
          <div className="schedule-list" aria-label="Scheduled prompts">
            {controller.schedules.map((schedule) => (
              <article className={`schedule-card ${schedule.enabled ? "" : "disabled"}`} key={schedule.id} aria-label={schedule.name}>
                <header>
                  <div className="schedule-card-icon"><CalendarClock size={18} /></div>
                  <div className="schedule-card-title">
                    <strong>{schedule.name}</strong>
                    <span>{targetLabel(schedule, props.projects, props.chats)}</span>
                  </div>
                  <label className="schedule-card-toggle">
                    <span>{schedule.enabled ? "Enabled" : "Disabled"}</span>
                    <input
                      type="checkbox"
                      aria-label={`Enable ${schedule.name}`}
                      checked={schedule.enabled}
                      disabled={Boolean(controller.busy)}
                      onChange={(event) => void controller.update(schedule.id, {
                        ...scheduledPromptInput(schedule),
                        enabled: event.target.checked,
                      })}
                    />
                  </label>
                </header>
                <p className="schedule-prompt-preview">{schedule.prompt}</p>
                <div className="schedule-expression"><Clock3 size={13} /><code>{scheduleLabel(schedule)}</code></div>
                <div className="schedule-timing-grid">
                  <div><span>Next run</span><strong>{schedule.enabled ? formatScheduleInstant(schedule.next_run_at, schedule.timezone) : "Disabled"}</strong></div>
                  <div><span>Last run</span><strong>{formatScheduleInstant(schedule.last_run_at, schedule.timezone)}</strong></div>
                  <div className={schedule.last_error ? "schedule-last-error" : ""}>
                    <span>Last result</span><strong>{statusLabel(schedule)}</strong>
                  </div>
                  <ResultConversation
                    schedule={schedule}
                    projects={props.projects}
                    chats={props.chats}
                    onOpen={props.onOpenConversation}
                  />
                </div>
                <footer>
                  <button
                    className="secondary-button"
                    aria-label={`Run ${schedule.name} now`}
                    disabled={Boolean(controller.busy) || schedule.pending}
                    onClick={() => void controller.runNow(schedule.id)}
                  ><Play size={13} /> {controller.busy === `run:${schedule.id}` ? "Starting…" : schedule.pending ? "In progress" : "Run now"}</button>
                  <button className="secondary-button" aria-label={`Edit ${schedule.name}`} disabled={Boolean(controller.busy) || schedule.pending} onClick={() => props.onEdit(schedule)}><Pencil size={13} /> Edit</button>
                  <button
                    className="danger-button schedule-delete-button"
                    aria-label={`Delete ${schedule.name}`}
                    disabled={Boolean(controller.busy) || schedule.pending}
                    onClick={() => window.confirm(`Delete scheduled prompt “${schedule.name}”?`) && void controller.remove(schedule.id)}
                  ><Trash2 size={13} /> Delete</button>
                </footer>
              </article>
            ))}
          </div>
        ) : (
          <div className="schedules-empty">
            <History size={25} />
            <h2>No scheduled prompts</h2>
            <p>Create one to run project work or start a fresh standalone chat automatically.</p>
            <button className="primary-button" onClick={props.onCreate}><Plus size={14} /> New scheduled prompt</button>
          </div>
        )}
      </div>
    </main>
  );
}
