# WebSocket API

Connect to `ws://127.0.0.1:7331/ws`. Commands and responses are correlated by
`id`; subscribed events arrive independently on the same connection.

```json
{"id":"req-1","type":"list_conversations"}
{"id":"req-1","type":"response","command":"list_conversations","success":true,"data":[]}
```

An unsuccessful response has `success:false` and an `error` string. `data` is
command-specific.

## Command envelope and conversation scope

The complete additive command envelope is:

```text
id, type
conversation_id, chat_id, project_id, thread_id
message, name, category, streaming_behavior
options, chat_options
after, limit
model, level, mode, tools
config, expected_revision
agent_id, parent_id, agent_name, agent_role, task, profile,
agent_ids, wake, timeout_ms
command_name, arguments, attachments
additional_folders
schedule_id, schedule
```

Only fields used by a command need to be sent. A conversation may be addressed
with any compatible identifier. Resolution order is:

```text
conversation_id > chat_id > project_id > thread_id
```

New clients should send `conversation_id`, the kind-specific `chat_id` or
`project_id`, and `thread_id` for compatibility. Standalone chats use IDs such
as `chat_...`; projects use `project_...`; child agents use `agent_...`.

Every live event carries an authoritative scope and generic identifiers:

```json
{
  "type": "message_update",
  "scope": {"kind": "chat", "id": "chat_..."},
  "conversation_id": "chat_...",
  "chat_id": "chat_...",
  "thread_id": "chat_...",
  "sequence": 12,
  "timestamp": "2026-07-11T00:00:00Z",
  "data": {"delta": "hello"}
}
```

Project events contain `project_id` instead of `chat_id`. `thread_id` is a
generic legacy alias, not an indication that the conversation is a project.

## Create and inspect conversations

Create a pathless standalone chat:

```json
{
  "id": "req-chat",
  "type": "create_chat",
  "chat_options": {
    "name": "Research",
    "model": "gpt-5.6",
    "thinking_level": "medium",
    "instructions": "Be concise."
  }
}
```

Create a folder-scoped project:

```json
{
  "id": "req-project",
  "type": "create_project",
  "options": {
    "name": "Demo",
    "category": "Client A",
    "cwd": "/absolute/path/to/project",
    "additional_folders": ["/absolute/path/to/shared-library"],
    "model": "gpt-5.6",
    "thinking_level": "medium",
    "tools": ["read", "grep", "find", "ls"]
  }
}
```

The daemon canonicalizes project `cwd` and every `additional_folders` entry,
rejecting relative, missing, root, and non-directory additions. The `cwd`
remains the main project folder and working directory. Relative tool paths
resolve there; additional folders use their canonical absolute paths. A
standalone chat has an empty folder and no local built-in tools.

Conversation commands:

```text
create_chat              list_chats              get_chat
get_chat_state           get_chat_messages       get_chat_events
delete_chat              subscribe_chat          unsubscribe_chat

create_project           list_projects           get_project
get_project_state        get_project_messages    get_project_events
delete_project           subscribe_project       unsubscribe_project

list_conversations       get_conversation        get_conversation_state
get_conversation_messages get_conversation_events delete_conversation
subscribe_conversation   unsubscribe_conversation
```

Generic compatibility commands are `create_thread`, `new_session`,
`list_threads`, `get_thread`, `get_state`, `get_messages`, `get_events`,
`delete_thread`, `subscribe`, and `unsubscribe`. `list_threads` currently lists
all conversations; use `list_projects` when only folder projects are wanted.

`get_*_messages` accepts `after` and `limit` and returns stored message rows.
`get_*_events` uses the same pagination fields and returns stored event rows
with `sequence`, `type`, `data`, and `created_at`. Replaying after the last seen
sequence closes subscription/reconnect gaps.

Creating a conversation automatically subscribes that WebSocket connection.
`prompt`, `steer`, `follow_up`, and `spawn_agent` also subscribe to their target
when accepted.

## Scheduled prompts

Scheduled prompts are owned by the daemon and remain active when every client
is disconnected. They can target an existing project, an existing standalone
chat, or a fresh one-off standalone chat created for each firing.

```text
list_scheduled_prompts
create_scheduled_prompt
update_scheduled_prompt
delete_scheduled_prompt
run_scheduled_prompt
subscribe_scheduled_prompts
unsubscribe_scheduled_prompts
```

Create a recurring project prompt with a standard five-field crontab
expression:

```json
{
  "id": "req-schedule",
  "type": "create_scheduled_prompt",
  "schedule": {
    "name": "Weekday review",
    "prompt": "Review open work and propose today's priorities.",
    "target_type": "project",
    "conversation_id": "project_...",
    "schedule_type": "cron",
    "cron": "0 9 * * 1-5",
    "timezone": "Pacific/Auckland",
    "enabled": true
  }
}
```

Cron fields are `minute hour day-of-month month day-of-week`. Lists, ranges,
steps, English month/weekday abbreviations, and the `@hourly`, `@daily`,
`@weekly`, `@monthly`, and `@yearly` aliases are accepted. When both day fields
are restricted, traditional crontab OR semantics apply. Timezones are IANA
names; `Local` uses the daemon host timezone.
Nonexistent wall times during a spring-forward transition are skipped; both
instances of a repeated wall time during a fall-back transition run.

Create a single isolated run with `schedule_type:"once"`, an RFC 3339
`run_at`, and `target_type:"one_off"` without a conversation ID:

```json
{
  "id": "req-once",
  "type": "create_scheduled_prompt",
  "schedule": {
    "name": "Research follow-up",
    "prompt": "Research the release notes and summarize material changes.",
    "target_type": "one_off",
    "schedule_type": "once",
    "run_at": "2026-07-15T20:30:00Z",
    "timezone": "Pacific/Auckland",
    "enabled": true
  }
}
```

`list_scheduled_prompts` lists every schedule. Include a conversation scope to
filter the result. Updates send `schedule_id` and a `schedule` patch; deletes
and manual runs send only `schedule_id`. Running manually does not change the
next automatic firing.

Existing targets use follow-up semantics: an idle conversation starts
immediately and a busy one queues the prompt. Further recurrences coalesce
while the same scheduled prompt is pending. A `one_off` target creates a new
pathless chat and records it as `last_conversation_id`.

Records report `next_run_at`, `last_run_at`, `last_status`, `last_error`,
`last_conversation_id`, and `pending`. One-time schedules disable themselves
when claimed. Automatically claimed work left pending by an unclean daemon stop
is retried after restart; an interrupted manual Run now does not enable a
disabled schedule.
Subscribers receive daemon-global events scoped as
`{"kind":"schedule","id":"schedule_..."}`:

```text
scheduled_prompt_created, scheduled_prompt_updated, scheduled_prompt_deleted
scheduled_prompt_triggered, scheduled_prompt_coalesced
scheduled_prompt_completed, scheduled_prompt_failed
```

## Run controls and events

Start a run:

```json
{"id":"req-2","type":"prompt","conversation_id":"project_...","message":"Read README.md and summarize it"}
```

Attach sandbox-owned image input to a new project prompt:

```json
{
  "id": "req-image",
  "type": "prompt",
  "project_id": "project_...",
  "message": "Inspect this screenshot",
  "attachments": [{
    "name": "clipboard.png",
    "mime_type": "image/png",
    "data": "BASE64_BYTES"
  }]
}
```

Images are supported only when starting an idle top-level project. The daemon
accepts PNG, JPEG, WebP, and GIF; limits each image to 5 MiB, accepts four at
most, caps the combined decoded size at 10 MiB, and writes generated filenames
under `<project>/.dire-agent/attachments`. Stored user-message data contains only
`name`, `mime_type`, `file`, and `size`, never the inbound base64 field.

While a conversation is running:

- `steer` injects text before the next model step.
- `follow_up` queues another complete turn.
- `prompt` accepts `streaming_behavior:"steer"` or `"followUp"` as a compact
  equivalent.
- `abort` cancels the active run.

Steering and follow-up queues support `all` and `one-at-a-time`. Typical events:

```text
agent_start, turn_start
message_start, message_update, message_end
reasoning_start, reasoning_update, reasoning_end
tool_execution_start, tool_execution_end
turn_end, agent_end, agent_settled
queue_update, abort_requested, agent_error, persistence_error
usage_update, capabilities_updated
```

`message_end` contains provider-neutral usage for that model step. The following
`usage_update` contains canonical cumulative counters while retaining the latest
context snapshot:

```json
{
  "input_tokens": 1200,
  "output_tokens": 240,
  "cache_read_tokens": 800,
  "cache_write_tokens": 64,
  "total_tokens": 1440,
  "context_tokens": 1440,
  "context_window": 372000
}
```

The same cumulative usage appears at the top level of `get_state` and in
conversation metadata. Cache values are zero when the provider does not report
them.

## Conversation settings and capabilities

```text
set_chat_name, set_conversation_name, set_project_name, set_project_category
set_thread_name, set_session_name
set_model
set_thinking_level
set_steering_mode
set_follow_up_mode
set_tools
set_project_sandbox_folders
get_available_tools
get_available_models
get_capabilities
get_commands
```

The setting value is sent in `name`, `category`, `model`, `level`, `mode`,
`tools`, or `additional_folders`. Settings cannot be changed during an active
run. Chats reject local tool enablement, categories, and sandbox folders.
Categories are trimmed, limited to 80 characters, and available only to
top-level projects.

Replace a project's complete included-folder set with:

```json
{
  "id": "req-folders",
  "type": "set_project_sandbox_folders",
  "project_id": "project_...",
  "additional_folders": ["/absolute/path/to/shared-library"]
}
```

The daemon deduplicates canonical paths, omits folders already contained by the
main project folder, and permits at most 16 additions. Updating this setting
reopens the provider session so its system instructions immediately identify
the main folder and every included folder.

## Project attachments and terminal PTY

Persisted images are served read-only from:

```text
GET /attachments/PROJECT_ID/GENERATED_FILENAME
```

The handler resolves the project first, accepts generated basenames only, and
rejects symlinks and non-image files. Responses use private immutable caching.

Interactive project applications use a separate WebSocket because their
frames are terminal data rather than daemon command envelopes:

```text
ws://127.0.0.1:7331/terminal?project_id=project_...&launcher_id=shell&cols=120&rows=36
```

`launcher_id` names a server-configured terminal/TUI entry; the browser never
sends an executable or arguments. `mode=shell|lazygit|nvim` remains a legacy
alias. Chats, desktop-kind launchers, and child agents are rejected. The
process starts in the project's canonical folder with
`TERM=xterm-256color`, `COLORTERM=truecolor`, and a dark-background hint.
Color-suppression variables inherited by the daemon, including `NO_COLOR`, are
removed for this explicitly interactive PTY. Client frames are JSON:

```json
{"type":"input","data":"pwd\n"}
{"type":"resize","cols":120,"rows":36}
```

Server frames are `ready`, `output`, `error`, or `exit`. `output.data` is
base64-encoded PTY bytes. The PTY is killed when its WebSocket closes. This is
a user-operated terminal with the daemon user's normal permissions, not a
model-callable sandboxed tool.

The command API exposes the effective ordered definitions with
`get_project_launchers`. A configured desktop entry is started with:

```json
{"type":"launch_project_app","project_id":"project_...","launcher_id":"finder"}
```

The daemon executes the stored command and argument vector directly in the
project folder and returns `{launched,id,label}`. Desktop apps run on the daemon
machine, outlive the requesting WebSocket, and share the same user-operated,
non-sandboxed permission boundary as terminal tabs.

Thinking levels are `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, and
`max`; `off` remains accepted for compatibility. Built-in tool names are
`read`, `grep`, `find`, `ls`, `write`, `edit`, and `bash`.

`get_available_models` returns `{provider,id,context_window?}` records. The
default Codex registry contains `gpt-5.6`, `gpt-5.6-sol`, `gpt-5.6-terra`,
`gpt-5.6-luna`, and `gpt-5.4`. The direct subscription provider maps the
`gpt-5.6` alias to `gpt-5.6-sol` on the wire.

`get_capabilities` returns the effective descriptor catalog, discovered skills,
and skill diagnostics. Sources include built-ins, trusted skills, MCP tools,
extensions, extension commands, and child-agent orchestration tools. Changes to
configuration apply immediately to loaded idle conversations and to the next
new run after an active conversation settles.

Trusted extensions may register slash commands:

```text
list_capability_commands
execute_capability_command
```

The list result contains `{name,description,source}` objects. Execution uses:

```json
{
  "id": "req-ext",
  "type": "execute_capability_command",
  "conversation_id": "project_...",
  "command_name": "ext:plugin:command",
  "arguments": "optional text"
}
```

The result is `{output?,prompt?,is_error?}`. If an extension returns `prompt`,
the daemon automatically queues it as a follow-up.

## Configuration

```text
config_get
config_effective
config_validate
config_update
```

`config_get` returns the versioned configuration document:

```json
{
  "version": 1,
  "revision": 3,
  "global": {
    "model": {}, "thinking": {}, "tools": {}, "queues": {},
    "skills": {}, "mcp": {}, "extensions": {}, "subagents": {},
    "desktop": {}, "standalone_chat": {}
  },
  "projects": {}
}
```

Secret MCP/extension environment values and MCP headers are returned as
`"[redacted]"`. `config_update` replaces the complete document and requires
`expected_revision` (or the candidate's `revision`). Redacted placeholders
preserve the previous secrets. A stale revision fails rather than overwriting a
concurrent edit. Successful updates increment the revision and refresh idle
capability snapshots.

`config_validate` validates a complete candidate without persisting it.
`config_effective` accepts a conversation ID and returns
`{settings,project_override}` after applying a matching project override.

Configuration supports:

- global and per-project model/tool/queue defaults;
- Agent Skills roots, disabled paths, and trust;
- stdio or Streamable HTTP MCP servers, tool allowlists, approval modes,
  environment/headers, and secret markers;
- local, Git, or registry extension source metadata and trust;
- child-agent limits and profiles;
- desktop paths/sync preferences and standalone-chat defaults.

Only trusted local extension adapters are executed. Git/registry installation
and automatic desktop sync are not currently performed.

## Child agents

The model-facing tools and matching WebSocket operations are:

```text
spawn_agent
list_agents
get_agent                 (WebSocket only)
send_agent_message
wait_agents
interrupt_agent
delete_agent              (WebSocket only)
```

Spawn example:

```json
{
  "id": "req-spawn",
  "type": "spawn_agent",
  "conversation_id": "project_...",
  "parent_id": "project_...",
  "agent_name": "reviewer",
  "profile": "review",
  "agent_role": "security reviewer",
  "task": "Review authentication changes",
  "model": "gpt-5.6",
  "level": "high",
  "tools": ["read", "grep"]
}
```

The child receives a separate `agent_...` conversation and SQLite file, starts
its task immediately, and cannot gain tools the parent/profile did not grant.
Standalone-chat children receive no inherited project or capability tools;
their agent-team orchestration tools remain available. Depth, children, and
concurrency limits come from configuration; child profiles can independently
allow or deny spawning.

`list_agents` uses `parent_id` or the envelope conversation ID. `get_agent`,
`interrupt_agent`, and `delete_agent` use `agent_id`. `send_agent_message` also
uses `message`; `wake` defaults to true. Messages are durable and cross-team
routing is rejected. `wait_agents` accepts optional `agent_ids` and
`timeout_ms`; the default is 30 seconds and the maximum is 60 seconds.

Agent-team events include:

```text
agent_created, agent_spawned, agent_completed
agent_message, agent_message_sent
agent_message_wake_error, agent_report_error
```

`agent_completed` is emitted on the child and parent scopes. With auto-report
enabled, a bounded completion summary is also sent to the parent as a durable
agent message.

## Persistence and process boundary

Each project, standalone chat, and child agent owns `<id>.db`. Provider state is
restored lazily after restart; a conversation recorded as running after an
unclean shutdown recovers as idle. Messages, events, usage, and tool records
remain queryable when no client was connected. Existing `thread_*.db` files
remain valid.

Project file tools enforce canonical containment across the main project folder
and its included folders. Relative paths resolve from the main folder;
additional roots require absolute paths. `bash` uses macOS `sandbox-exec` with
network and outside-sandbox writes denied. Stdio MCP and trusted extension
processes inherit the included writable roots and use the configured `strict`,
`workspace`, or `off` sandbox policy; sandboxed launches fail closed when
`sandbox-exec` is unavailable. Child agents inherit their parent project's
folder set. Remote MCP calls are outside that filesystem boundary.

## Current protocol limitations

- The daemon has no WebSocket authentication or TLS; keep it loopback-only or
  place it behind an authenticated reverse proxy.
- Interactive approval prompts are not implemented. MCP tools with
  `on-request` or `always` approval remain disabled/approval-required.
- MCP roots, sampling, elicitation, and first-class prompt/resource UI are not
  advertised. Servers may expose prompts and resources through bounded
  `mcpctx__SERVER__*` model tools.
- Extension UI/settings/tool-renderer registrations and bundled manifest
  theme/app/MCP metadata are not yet surfaced or imported automatically.
- Desktop sync fields are configuration metadata; install the standard MCP
  plugin from `integrations/dire-agent` manually.
- Compaction and ordinary conversation fork/tree navigation are not available.
  Persistent child-agent trees are supported separately.
