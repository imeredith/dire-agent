import { configurationThinkingLevels, firstPartyModels } from "../../lib/display";
import type { GlobalSettings } from "../../lib/protocol";
import { Field, listText, parseList, SettingsSection, Toggle } from "./SettingsFields";

interface GeneralSettingsProps {
  value: GlobalSettings;
  onChange: (value: GlobalSettings) => void;
}

export function GeneralSettings({ value, onChange }: GeneralSettingsProps) {
  const change = <K extends keyof GlobalSettings>(key: K, next: GlobalSettings[K]) =>
    onChange({ ...value, [key]: next });
  return (
    <>
      <SettingsSection
        id="general"
        eyebrow="DEFAULTS"
        title="Model and reasoning"
        description="These defaults apply to new projects unless a conversation overrides them."
      >
        <div className="settings-grid three">
          <Field label="Provider">
            <input value={value.model.provider} onChange={(event) => change("model", { ...value.model, provider: event.target.value })} />
          </Field>
          <Field label="Model">
            <input
              list="model-options"
              value={value.model.id}
              onChange={(event) => change("model", { ...value.model, id: event.target.value })}
            />
            <datalist id="model-options">{firstPartyModels.map((model) => <option value={model} key={model} />)}</datalist>
          </Field>
          <Field label="Context window" hint="Tokens; use 0 when unknown.">
            <input
              type="number"
              min={0}
              value={value.model.context_window ?? 0}
              onChange={(event) => change("model", { ...value.model, context_window: Number(event.target.value) })}
            />
          </Field>
          <Field label="Thinking level">
            <select
              value={value.thinking.level}
              onChange={(event) => change("thinking", { level: event.target.value as GlobalSettings["thinking"]["level"] })}
            >
              {configurationThinkingLevels.map((level) => <option key={level}>{level}</option>)}
            </select>
          </Field>
        </div>
      </SettingsSection>

      <SettingsSection
        id="tools"
        eyebrow="EXECUTION"
        title="Tools, sandbox and queues"
        description="Folder tools run inside the selected project sandbox. Approval remains explicit."
      >
        <div className="settings-grid three">
          <Field label="Enabled tools" hint="One per line or comma separated." wide>
            <textarea
              rows={4}
              value={listText(value.tools.enabled)}
              onChange={(event) => change("tools", { ...value.tools, enabled: parseList(event.target.value) })}
            />
          </Field>
          <Field label="Process sandbox default" hint="Off disables the native process sandbox for projects that inherit this global default.">
            <select
              aria-label="Process sandbox default"
              value={value.tools.sandbox}
              onChange={(event) => change("tools", { ...value.tools, sandbox: event.target.value as GlobalSettings["tools"]["sandbox"] })}
            >
              <option value="strict">Strict</option><option value="workspace">Workspace</option><option value="off">Off</option>
            </select>
          </Field>
          <Field label="Tool approval">
            <ApprovalSelect value={value.tools.approval} onChange={(approval) => change("tools", { ...value.tools, approval })} />
          </Field>
          <Field label="Steering queue">
            <QueueSelect value={value.queues.steering_mode} onChange={(steering_mode) => change("queues", { ...value.queues, steering_mode })} />
          </Field>
          <Field label="Follow-up queue">
            <QueueSelect value={value.queues.follow_up_mode} onChange={(follow_up_mode) => change("queues", { ...value.queues, follow_up_mode })} />
          </Field>
          <Field label="Maximum pending">
            <input type="number" min={1} value={value.queues.max_pending} onChange={(event) => change("queues", { ...value.queues, max_pending: Number(event.target.value) })} />
          </Field>
        </div>
      </SettingsSection>

      <SettingsSection
        id="standalone"
        eyebrow="PATHLESS"
        title="Standalone chat defaults"
        description="Standalone chats persist independently and intentionally have no project folder."
      >
        <div className="settings-grid two">
          <Field label="Model">
            <input value={value.standalone_chat.model} onChange={(event) => change("standalone_chat", { ...value.standalone_chat, model: event.target.value })} />
          </Field>
          <Field label="Thinking">
            <select value={value.standalone_chat.thinking} onChange={(event) => change("standalone_chat", { ...value.standalone_chat, thinking: event.target.value as GlobalSettings["standalone_chat"]["thinking"] })}>
              {configurationThinkingLevels.map((level) => <option key={level}>{level}</option>)}
            </select>
          </Field>
          <Field label="Remote tools" hint="Standalone chats never receive local folder tools." wide>
            <textarea rows={3} value={listText(value.standalone_chat.tools)} onChange={(event) => change("standalone_chat", { ...value.standalone_chat, tools: parseList(event.target.value) })} />
          </Field>
          <Field label="Instructions" wide>
            <textarea rows={4} value={value.standalone_chat.instructions || ""} onChange={(event) => change("standalone_chat", { ...value.standalone_chat, instructions: event.target.value })} />
          </Field>
        </div>
        <Toggle
          label="Persist chat history"
          hint="Keep messages and events in the chat's SQLite file."
          checked={value.standalone_chat.persist_history}
          onChange={(persist_history) => change("standalone_chat", { ...value.standalone_chat, persist_history })}
        />
      </SettingsSection>
    </>
  );
}

function ApprovalSelect({ value, onChange }: { value: GlobalSettings["tools"]["approval"]; onChange: (value: GlobalSettings["tools"]["approval"]) => void }) {
  return <select value={value} onChange={(event) => onChange(event.target.value as typeof value)}><option value="never">Never</option><option value="on-request">On request</option><option value="always">Always</option></select>;
}

function QueueSelect({ value, onChange }: { value: GlobalSettings["queues"]["steering_mode"]; onChange: (value: GlobalSettings["queues"]["steering_mode"]) => void }) {
  return <select value={value} onChange={(event) => onChange(event.target.value as typeof value)}><option value="one-at-a-time">One at a time</option><option value="all">All at once</option></select>;
}
