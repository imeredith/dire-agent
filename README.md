# Dire Agent

Dire Agent is a Go library and local daemon for persistent AI-agent
conversations. It supports two conversation scopes:

- **Projects** are bound to a canonical folder and may use explicitly enabled
  local tools inside that folder.
- **Standalone chats** have no folder and no local file or shell tools.

The Codex provider calls the ChatGPT/Codex subscription endpoint directly with
credentials created by `codex login`. It does **not** invoke Codex CLI or use
`codex app-server`. Provider-neutral interfaces keep model transports separate
from storage, tools, the agent loop, and clients.

## Included

- A provider-neutral multi-turn API, serializable provider state, and an
  agentic tool loop.
- Direct Codex HTTP/SSE transport with credential refresh and GPT-5.6 model
  selection.
- Cumulative input, output, cache-read, provider-reported cache-write,
  total-token, and current-context accounting.
- One WAL-enabled SQLite file per project, standalone chat, and child agent,
  containing metadata, messages, events, usage, and provider state.
- Prompt, steering, follow-up queues, abort, model/thinking controls, and
  streamed lifecycle events. Reasoning summaries and tool input/output are
  streamed, persisted, and restored in both interactive clients.
- Persistent child agents with profiles, bounded depth/concurrency, durable
  inter-agent messages, wait, interrupt, and completion reporting.
- Agent Skills-compatible discovery and progressive loading.
- MCP tools, resources, and prompts over stdio and Streamable HTTP.
- Trusted, out-of-process extension adapters with tools, slash commands, prompt
  fragments, and before/after model/tool hooks. Local Codex plugin and Pi
  package manifests are discoverable.
- A WebSocket daemon, reusable Go client, Bubble Tea chat, and
  React/Tailwind/Vite app. An optional MCP compatibility bridge is included but
  is not part of the direct Codex provider path.
- Persisted project categories with a category-only privacy view, keyboard
  slash-command completion, project-scoped image paste, and persistent,
  configurable workspace tabs for terminals, TUIs, and desktop launchers.
- Built-in `read`, `grep`, `find`, `ls`, `write`, `edit`, and `bash` tools.

## Security boundary

Projects are bound to one canonical main directory and may include additional
canonical sandbox folders. The main project folder remains the working
directory: relative tool paths and shell commands start there, while an
additional folder is addressed by its absolute path. New projects receive only
`read,grep,find,ls` by default. File tools enforce lexical and symlink-aware
containment across the configured roots; `write` and `edit` cannot escape the
project sandbox.

`bash` always runs through macOS `/usr/bin/sandbox-exec`, with the main project
folder, included folders, and temporary directories writable and network
access denied. It fails closed when the sandbox is unavailable. Configured
stdio MCP servers and trusted extension processes receive the same included
folders and are also wrapped by `sandbox-exec` unless their sandbox mode is
explicitly `off`:

- `strict`: project sandbox/temp writes only; network denied.
- `workspace`: the same file boundary with network allowed.
- `off`: no process sandbox; the child has the daemon user's permissions.

Standalone chats never receive the local built-ins. They may still use trusted
skills, enabled MCP tools, extensions, and agent-team tools. Remote HTTP MCP
actions are not constrained by a local filesystem sandbox.

Pasted images are accepted only by top-level projects. The daemon validates
their MIME type and size, generates the filename, rejects symlink escapes, and
stores them under `<project>/.dire-agent/attachments`. A prompt may contain up to
four images, 5 MiB each and 10 MiB total. Pathless chats reject images.

Workspace launchers are explicit user-operated applications, not model tools.
The browser sends only a configured launcher ID; the daemon resolves its direct
executable and argument list without invoking a shell. Terminal/TUI processes
start in the selected project folder and desktop applications open on the
machine running the daemon. Both have the daemon user's normal permissions and
are outside the agent's `sandbox-exec` boundary. Agent `bash`, stdio MCP, and
extension execution retain the sandbox policy described above.

The browser terminal loads Cascadia Code from Google Fonts, uses the pinned Nerd
Fonts 3.4 symbols-only webfont for TUI icons, enables programming ligatures,
and uses xterm's WebGL custom-glyph renderer so box-drawing borders remain
continuous. The PTY advertises a dark true-color environment and removes
inherited color opt-outs such as `NO_COLOR`, so lazygit and LazyVim can render
their intended palettes.

The daemon currently has no transport authentication. It listens on loopback
by default and refuses a non-loopback address unless `-allow-remote` is passed.
Do not expose it directly to an untrusted network.

## Start the daemon

```sh
codex login
go run ./cmd/dire-agentd
```

Defaults:

- WebSocket/HTTP: `127.0.0.1:7331`
- conversation databases: `~/.dire-agent/projects`
- configuration: `~/.dire-agent/config.json`
- model: `gpt-5.6`
- credentials: `$CODEX_HOME/auth.json` or `~/.codex/auth.json`

If the projects directory is absent but `~/.dire-agent/threads` exists, the daemon
uses the legacy directory so existing histories remain visible. The convenient
`gpt-5.6` selector maps to `gpt-5.6-sol` on the ChatGPT-backed wire endpoint;
explicit `sol`, `terra`, and `luna` variants pass through unchanged.

After an upgrade from GoAgent, existing `~/.goagent/projects`,
`~/.goagent/threads`, and `~/.goagent/config.json` remain readable when the
corresponding Dire Agent paths do not yet exist. New installations and new
project attachments use the `.dire-agent` namespace; previously stored
`.goagent/attachments` remain readable.

Override the main paths and project defaults with flags:

```sh
go run ./cmd/dire-agentd \
  -data-dir ./agent-data \
  -config ./dire-agent.json \
  -cwd /path/to/project \
  -tools read,grep,find,ls,write,edit,bash
```

Configuration is validated, revisioned, atomically replaced, and written with
mode `0600`. It has global settings plus per-project overrides for models,
queues, sandbox policy, skills, MCP servers, extensions, child-agent profiles,
desktop metadata, and standalone-chat defaults. WebSocket configuration reads
redact secret environment variables and headers; sending `[redacted]` back in
an update preserves the stored value.

## Terminal chat

Create or open a folder project:

```sh
go run ./cmd/dire-agentctl
go run ./cmd/dire-agentctl -project PROJECT_ID
go run ./cmd/dire-agentctl -folder /path/to/project
```

Create or open a pathless standalone chat:

```sh
go run ./cmd/dire-agentctl -standalone
go run ./cmd/dire-agentctl -chat CHAT_ID
```

`-message` supplies an initial prompt. Enter sends, `Ctrl+J` or `Shift+Enter`
adds a newline, `PgUp`/`PgDn` scrolls, and `Ctrl+C` leaves without stopping a
daemon run. An ordinary message sent during a run becomes a follow-up.

Bubble Tea commands:

```text
/steer TEXT       inject guidance into the active run
/follow-up TEXT   queue the next turn (alias: /followup)
/abort            cancel the active run
/agents           list the conversation's agent tree
/spawn NAME TASK  spawn a general child; NAME PROFILE -- TASK selects a profile
/message ID TEXT  send and wake an agent (alias: /msg)
/wait [ID ...]    wait up to 30 seconds for child agents
/interrupt ID     cancel a running child agent
/delete-agent ID  delete an idle child agent
/commands         list extension-provided commands
/ext:ID:CMD ARGS  execute an extension command
/model [MODEL]    show or change the model while idle
/thinking [LEVEL] show or change reasoning level
/name [NAME]      show or rename the conversation
/folders          show the main and additional sandbox folders
/folder-add PATH  include an absolute folder in the project sandbox
/folder-remove PATH remove an included folder by canonical path
/status           show scope, state, settings, token usage, and context
/clear            clear the local transcript view
/help             show command help
/quit             leave the client
```

Trusted skills can be requested with `/skill:NAME arguments` or `$NAME`. These
remain normal prompts in the UI; the daemon expands the selected `SKILL.md`
instructions for that run.

Non-interactive `dire-agentctl -action` values include `prompt`, `steer`,
`follow-up`, `abort`, `list`, `list-chats`, `state`, `create`, and
`create-chat`.

## Web app

The `web` directory contains the React 19, Tailwind CSS 4, and Vite client. It
separates Chats and Projects, keeps the transcript independently scrollable,
streams model/tool activity, displays token/cache/context usage, and includes
conversation controls, child-agent management, and full-page configuration for
skills, MCP, extensions, subagents, sandboxing, and desktop metadata.

Projects can be grouped by category. Selecting one category hides every other
project and switches away from a previously selected hidden project; the
privacy filter survives reloads. Paste PNG, JPEG, WebP, or GIF data directly
into the composer. The composer footer contains model, thinking, and queue
selectors, while `/` commands complete with Tab, Enter, or arrow keys. A
project's details drawer can include additional sandbox folders without
changing its main folder or relative working directory.

The default project application shortcuts are:

- `Cmd/Ctrl + backtick`: shell terminal
- `Cmd/Ctrl + Shift + G`: lazygit
- `Cmd/Ctrl + Shift + E`: nvim

They open as persistent top-bar tabs: switching back to Chat keeps each PTY
alive, invoking the active shortcut toggles back to Chat, and the tab close
button terminates that session. Settings → Workspace tabs can add, remove,
reorder, and change terminal/TUI or desktop launchers and their shortcuts.

```sh
cd web
npm install
npm run dev
```

Open `http://127.0.0.1:5173`. Vite proxies `/ws`, `/terminal`, `/attachments`,
and `/healthz` to the local daemon. The feature-by-feature Web UI test guide is at
`http://127.0.0.1:5173/docs`. See [web/README.md](web/README.md) and
[docs/websocket.md](docs/websocket.md).

### Production Web UI

Build a single daemon binary containing the optimized Vite application:

```sh
npm --prefix web install
make production
./dist/dire-agentd
```

Open `http://127.0.0.1:7331`. The embedded UI, `/ws`, `/terminal`,
`/attachments`, and `/healthz` are all served by the same Go HTTP server, so no
Vite process or cross-origin configuration is required. Client-side routes such
as `/docs` fall back to `index.html`; hashed Vite assets receive immutable cache
headers while HTML is revalidated.

A normal development binary can instead host an existing build directory:

```sh
npm --prefix web run build
go run ./cmd/dire-agentd -web-dir ./web/dist
```

The production-tagged binary serves its embedded UI automatically. Pass
`-no-web-ui` to keep it API-only, or `-web-dir PATH` to serve an external build
instead when using a normal binary. These options do not add authentication;
the loopback-only security boundary still applies.

### Project server proxy

The daemon can mount a development server running on its own loopback interface
below the Web UI origin. For example, a server listening at
`http://127.0.0.1:5172` is available at:

```text
http://127.0.0.1:7331/project/server/5172/
```

The proxy forwards HTTP and WebSocket traffic, normally removes the mount prefix
before calling the upstream server, and rewrites redirects, cookies, URL-bearing
headers, HTML resource attributes, module imports, and CSS asset URLs beneath the
mount without changing unrelated framework string constants.
It also injects a small bootstrap that rewrites browser fetch/XHR, workers,
EventSource, WebSocket connections, and DOM URL attributes. This keeps Vite HMR
and Next.js HMR WebSockets on the proxied origin.

History-based routers are supported: `pushState` and `replaceState` retain the
mount, and a reload of `/project/server/5172/some/route` is forwarded upstream
as `/some/route`. Applications whose router requires an explicit basename can
read the injected prefix:

```tsx
const proxy = window.__DIRE_AGENT_PROJECT_PROXY__;
<BrowserRouter basename={proxy?.prefix}>{/* routes */}</BrowserRouter>;
```

During the rename transition, `window.__GOAGENT_PROJECT_PROXY__` points to the
same object so existing proxied applications keep their basename contract.

The same object exposes `pathname` with the mount removed and a `rewriteURL`
helper. Next.js App Router projects must set Next's build-time `basePath` to the
mount, for example `basePath: '/project/server/5172'`. The daemon detects that
configured Next endpoint and preserves the prefix upstream instead of stripping
it, allowing Next routing and HMR to use their supported sub-path contract.
Vite and other ordinary development servers continue to receive stripped paths.

The proxy is enabled by default on loopback listeners and targets only the
daemon host's literal `127.0.0.1`; it does not proxy to the browser machine or
arbitrary hosts. Disable it with `-project-proxy=false`. Because it exposes
other loopback HTTP services through the daemon, a non-loopback listener also
requires both `-allow-remote` and `-allow-remote-project-proxy`.

## Skills, MCP, and extensions

Skills are discovered from configured global roots (by default
`~/.agents/skills`, `~/.codex/skills`, and `~/.pi/agent/skills`), plugin roots,
and ancestor `.agents/skills`, `.codex/skills`, and `.pi/skills` directories for
projects. Metadata is disclosed first; complete instructions are loaded only on
demand. Skill trust must be `trusted` before the daemon exposes the skill tool
or expands explicit invocations.

MCP supports stdio and Streamable HTTP. Remote tools use model-visible names
`mcp__SERVER__TOOL`. Servers that advertise resources or prompts also receive
bounded `mcpctx__SERVER__list_resources`, `read_resource`, `list_prompts`, and
`get_prompt` tools. Discovery follows pagination and tool-list change
notifications. Server/tool allowlists, timeouts, bounded results, secret
redaction, same-origin HTTP redirects, and connection diagnostics are enforced.
The outward `dire-agent-mcp` bridge is rejected as an inward server to prevent
recursion.

Extensions use an NDJSON JSON-RPC subprocess protocol rather than loading
JavaScript, TypeScript, or Go plugins into daemon memory. A local source must be
enabled, explicitly `trusted`, and configured with a command/argv adapter before
it runs. Tools are named `ext__EXTENSION__TOOL`; registered commands use
`/ext:EXTENSION:COMMAND`. See [extensions/PROTOCOL.md](extensions/PROTOCOL.md).
Git/registry sources are catalog metadata only in this iteration and are not
automatically downloaded or executed.

## Codex/ChatGPT desktop bridge

`dire-agent-mcp` exposes the daemon through standard MCP over stdio; it does not
use app-server:

```sh
go build -o /usr/local/bin/dire-agent-mcp ./cmd/dire-agent-mcp
go run ./cmd/dire-agentd
```

The local Codex plugin is in [integrations/dire-agent](integrations/dire-agent). It
provides MCP tools to list/create chats and projects, inspect state/history,
send a message (optionally waiting for settlement), abort a run, and coordinate
persistent child-agent teams. Install
that directory as a local plugin after ensuring `dire-agent-mcp` is on the desktop
app's PATH. It manages separate Dire Agent conversations; it does not transfer the
current Codex task.

Desktop sync paths and modes are configurable, but automatic plugin install,
marketplace editing, config import/export, and file watching are not yet
performed by the daemon.

## Packages

- `agent`, `agentloop`: provider/session contracts and the model/tool loop.
- `provider/codex`: direct Codex credentials, HTTP/SSE, state, and tool calls.
- `threadstore`: per-conversation SQLite persistence; the historical name is
  retained for compatibility.
- `tools`: confined built-ins and reusable macOS process sandboxing.
- `skills`, `mcpclient`, `extensions`, `capability`: discovery, transports,
  trust/policy, hooks, and per-run capability composition.
- `agentteam`: persistent child-agent model tools and shared types.
- `daemon`, `client`, `chatui`: manager, WebSocket API, Go client, and Bubble
  Tea UI.
- `mcpserver`, `cmd/dire-agent-mcp`, `integrations/dire-agent`: desktop MCP bridge.
- `web`: React/Tailwind/Vite browser client.

## Current limitations

- The ChatGPT subscription endpoint is not a public, supported OpenAI API and
  can change without compatibility guarantees. A supported Platform provider
  can be added behind the existing interfaces.
- Process sandboxing currently depends on macOS `sandbox-exec`. Sandboxed local
  tools/MCP/extensions fail closed on unsupported hosts.
- There is no interactive approval broker yet. MCP tools configured as
  `on-request` or `always` are reported as approval-required and are not exposed
  to the model; explicitly trusted tools require `approval: "never"`.
- MCP does not advertise roots, sampling, or elicitation. Prompts and resources
  are exposed through bounded model tools, not as first-class composer/UI
  objects.
- Arbitrary Pi TypeScript extensions are not loaded directly. Use the
  out-of-process adapter protocol. Adapter UI/settings/tool-renderer
  registrations and manifest theme/app/MCP metadata are catalogued but are not
  yet activated as client UI or imported automatically.
- Session compaction, forking, and tree navigation for ordinary conversations
  are not implemented. Child-agent trees are a separate orchestration feature.
- The daemon/WebSocket has no built-in authentication or TLS.

## Tests

```sh
go test ./...
go test -race ./...

cd web
npm run typecheck
npm test
npm run build
```

The opt-in live test uses current Codex CLI credentials, makes a real direct
request, and consumes subscription allowance:

```sh
go test -tags=live ./provider/codex -run TestLiveSubscriptionCredentials -v
go test -tags=live ./provider/codex -run TestLiveLunaReasoningAndImage -v
```

It uses `gpt-5.6-luna` by default; set `CODEX_LIVE_MODEL` to override it. The
longer `TestLiveLunaPromptCaching` test validates a real cache read. The direct
subscription stream currently omits cache-write telemetry, so the library and
UIs report `0` rather than inventing a write count; a later nonzero cache read
is the evidence that the prefix was stored. Model output is never prompt-cached.
