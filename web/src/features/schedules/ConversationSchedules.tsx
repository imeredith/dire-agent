import { CalendarClock, Pencil, Play, Plus } from "lucide-react";
import type { SchedulesController } from "../../hooks/useSchedules";
import { scheduledPromptInput } from "../../hooks/useSchedules";
import { conversationKind, type Conversation, type ScheduledPrompt } from "../../lib/protocol";
import { formatScheduleInstant } from "./SchedulesPage";

interface ConversationSchedulesProps {
  resource: Conversation;
  controller: SchedulesController;
  onAdd: () => void;
  onEdit: (schedule: ScheduledPrompt) => void;
}

export function ConversationSchedules(props: ConversationSchedulesProps) {
  const { controller, resource } = props;
  const kind = conversationKind(resource);
  const schedules = controller.schedules.filter((schedule) =>
    schedule.target_type === kind && schedule.conversation_id === resource.id);
  return (
    <section className="drawer-section conversation-schedules">
      <div className="section-title">
        <span>Scheduled prompts</span>
        {controller.supported && (
          <button className="tiny-button" onClick={props.onAdd}><Plus size={12} /> Add</button>
        )}
      </div>
      {!controller.supported ? (
        <p className="quiet-copy">This daemon does not expose scheduled prompts yet.</p>
      ) : controller.loading && !schedules.length ? (
        <p className="quiet-copy">Loading scheduled prompts…</p>
      ) : schedules.length ? (
        <div className="drawer-schedule-list">
          {schedules.map((schedule) => (
            <article key={schedule.id} className={schedule.enabled ? "" : "disabled"}>
              <CalendarClock size={14} />
              <div>
                <strong>{schedule.name}</strong>
                <small>{schedule.enabled ? `Next ${formatScheduleInstant(schedule.next_run_at, schedule.timezone)}` : "Disabled"}</small>
              </div>
              <label title={schedule.enabled ? "Disable" : "Enable"}>
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
              <button className="icon-button" aria-label={`Run ${schedule.name} now`} disabled={Boolean(controller.busy) || schedule.pending} onClick={() => void controller.runNow(schedule.id)}><Play size={13} /></button>
              <button className="icon-button" aria-label={`Edit ${schedule.name}`} disabled={Boolean(controller.busy) || schedule.pending} onClick={() => props.onEdit(schedule)}><Pencil size={13} /></button>
            </article>
          ))}
        </div>
      ) : (
        <button className="drawer-schedule-empty" onClick={props.onAdd}>
          <CalendarClock size={17} />
          <span><strong>No scheduled prompts</strong><small>Run a prompt here later or on a recurring cron.</small></span>
        </button>
      )}
    </section>
  );
}
