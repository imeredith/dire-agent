# Dire Agent extension process protocol

Dire Agent does not load Pi JavaScript/TypeScript or Go plugins into the daemon.
An extension that exposes tools runs as an explicitly configured child process.
The extension must be enabled and have `trusted` trust state before it starts.
The command and each argument are passed directly to the operating system; no
shell is inserted and no package installation or network lookup is performed.
The child receives only explicitly configured environment variables unless
`inherit_env` is enabled for that process.

The process boundary protects daemon memory and lifecycle. When an adapter is
launched through the daemon capability registry, Dire Agent also applies the
configured macOS `sandbox-exec` policy: strict mode denies network access and
confines writes to the project and temporary directories; workspace mode keeps
the write boundary while allowing network. Sandbox-off is an explicit setting.

## Framing

stdin and stdout carry JSON-RPC 2.0, one compact JSON object per line (NDJSON).
stdout is protocol-only. Human-readable logs belong on stderr. Request IDs must
be returned unchanged. Responses may arrive out of order. Messages and captured
stderr are bounded by the client configuration.

Protocol version: `1.0`.

## Methods

### `initialize`

First request:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocol_version":"1.0","client":{"name":"dire-agent","version":"1"},"extension_id":"example"}}
```

The result must confirm the version and may register Pi-style contributions:

```json
{"jsonrpc":"2.0","id":1,"result":{"protocol_version":"1.0","server":{"name":"example-adapter","version":"1.0.0"},"registration":{"commands":[{"name":"deploy","description":"Prepare a deployment."}],"prompt_fragments":[{"id":"policy","text":"Follow the release checklist.","priority":10}],"hooks":[{"id":"guard","event":"before_tool","priority":10}]}}}
```

Registration supports `commands`, ordered `prompt_fragments`, lifecycle
`hooks`, a JSON-object `settings_schema`, declarative `ui` contributions, and
`tool_renderers`. UI contributions are metadata only; extension JavaScript or
React is never injected into a client. All collections and payloads are
validated and bounded by host limits.

### `list_tools`

The params are `{}`. The result contains `tools`. Each input schema must be a
JSON Schema object with top-level `"type":"object"`.

```json
{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"lookup","description":"Look up a record.","input_schema":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}}]}}
```

Tools are exposed to the model as `ext__<extension-id>__<tool-name>`. Unsafe
name characters are normalized and long names receive a deterministic suffix.

### `call_tool`

```json
{"jsonrpc":"2.0","id":3,"method":"call_tool","params":{"name":"lookup","arguments":{"id":"42"}}}
```

```json
{"jsonrpc":"2.0","id":3,"result":{"output":"record contents","is_error":false}}
```

`is_error` marks a tool-level failure while preserving bounded output for the
model. JSON-RPC `error` is reserved for protocol, validation, or adapter faults.

### `execute_command`

Registered slash commands are invoked with a name and an unparsed argument
string:

```json
{"jsonrpc":"2.0","id":4,"method":"execute_command","params":{"name":"deploy","arguments":"staging"}}
```

The result can contain display `output`, a `prompt` for the conversation, and
`is_error`. Dire Agent exposes names as `/ext:<extension-id>:<command>` and queues
a non-empty returned prompt through the normal conversation run queue.

### `invoke_hook`

Hooks run in ascending `priority` order. Supported events are
`before_prompt`, `after_model`, `before_tool`, and `after_tool`.

```json
{"jsonrpc":"2.0","id":5,"method":"invoke_hook","params":{"hook_id":"guard","event":"before_tool","payload":{"tool_name":"write","arguments":{"path":"release.txt"}}}}
```

A hook result may transform the event-specific `prompt`, `model_text`,
`arguments`, `output`, or `is_error` fields. `{"veto":true,"message":"..."}`
stops the operation with a bounded error. Adapter failures are isolated to the
current run or tool call; they do not crash the daemon.

### `get_status`

The params are `{}`. Return `{"level":"ready|degraded|error","message":"..."}`
for settings and diagnostics surfaces.

### `shutdown`

The params are `{}`. Return an empty object result, stop accepting work, and
exit. Dire Agent closes stdin and terminates the process after the bounded shutdown
period even when the adapter does not respond.

## Cancellation

Every client request has a context and deadline. A timed-out request is forgotten
and a late response is ignored. Protocol 1.0 has no cancellation notification;
adapters should enforce their own operation limits as well.
