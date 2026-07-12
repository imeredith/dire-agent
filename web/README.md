# Dire Agent web

React 19, Tailwind CSS 4, and Vite 8 client for the Dire Agent daemon.

## Run locally

Start the daemon from the repository root:

```sh
codex login
go run ./cmd/dire-agent daemon
```

In another terminal:

```sh
cd web
npm install
npm run dev
```

Open `http://127.0.0.1:5173`. Vite proxies `/ws`, `/terminal`, `/attachments`,
`/healthz`, and `/project/server` to `127.0.0.1:7331`, so the browser and daemon
remain same-origin without disabling WebSocket origin verification.

Open `http://127.0.0.1:5173/docs` for one test page per supported feature,
including prerequisites, exact Web UI actions, and expected results.

## Production hosting

From the repository root, build an optimized single binary with the Web UI
embedded:

```sh
npm --prefix web install
make production
./dist/dire-agent start
```

The application is then available at `http://127.0.0.1:7331`; the Go server
owns the HTML/assets, WebSocket APIs, terminals, attachments, and health check
on one origin. The build stages Vite output under `internal/webui/dist`, embeds
it behind the `webui` Go build tag, and writes the final binary to
`dist/dire-agent`. Both generated directories are ignored by Git.

To serve an ordinary `web/dist` build without embedding it:

```sh
npm run build
go run ../cmd/dire-agent daemon -web-dir ./dist
```

Deep links use an `index.html` SPA fallback. Files below `/assets/` are treated
as content-hashed and cached immutably; HTML is served with `no-cache`. Use
`-no-web-ui` with an embedded production binary to run the daemon API only.

## Project development servers

A development server listening on the daemon machine at `127.0.0.1:5172` can
be opened through either the development or production Web UI at
`/project/server/5172/`. HTTP redirects, assets, cookies, WebSockets, Vite HMR,
and Next.js HMR stay on that mounted path.

The injected compatibility bootstrap preserves the mount when an application
calls `history.pushState` or `history.replaceState`, and deep-link reloads are
sent to the upstream server without the mount. For React Router or another
router that explicitly matches the browser pathname, use
`window.__DIRE_AGENT_PROJECT_PROXY__.prefix` as its basename. The object also
provides a mount-free `pathname` and `rewriteURL` helper.

The legacy `window.__GOAGENT_PROJECT_PROXY__` name remains an alias during the
rename transition.

Next.js App Router projects should set `basePath` to the complete mount, such as
`/project/server/5172`. The daemon detects a Next server configured for that
base path and preserves the mounted path upstream, while ordinary Vite-style
servers continue to receive the prefix-stripped route.

This feature is a local development bridge, not an authentication boundary.
It targets only `127.0.0.1` on the daemon machine. Use `-project-proxy=false` to
turn it off; exposing it from a non-loopback daemon requires the explicit
`-allow-remote-project-proxy` flag in addition to `-allow-remote`.

## Features

- Create, search, open, rename, and delete standalone chats and projects.
- Group projects into persisted categories and select a category-only privacy
  view that hides other projects and survives reloads.
- Keep standalone chats pathless while persisting their own SQLite history.
- Scope every project to a main folder plus optional included folders. Relative
  paths still start in the main folder, and daemon tools remain confined to the
  resulting sandbox rather than receiving unrestricted filesystem access.
- Restore SQLite-backed messages, project settings, and cumulative usage after
  reload.
- Stream assistant output, reasoning summaries, and tool execution over
  WebSockets; persist complete reasoning and tool input/output cards.
- Prompt, steer, queue follow-ups, and abort active runs.
- See cumulative input, output, cache-read, and provider-reported cache-write
  tokens plus current context utilization and percentage. Luna uses the
  installed Codex catalog's 372,000-token context window.
- Choose a model from a dropdown containing `gpt-5.6`, `gpt-5.6-sol`,
  `gpt-5.6-terra`, `gpt-5.6-luna`, and models discovered from the daemon.
- Configure thinking level, tool access, and queue behavior per conversation.
  GPT-5.6 reasoning levels include `none`, `minimal`, `low`, `medium`, `high`,
  `xhigh`, and `max`; `off` remains available for older daemon/model setups.
- Use `/steer`, `/follow-up`, `/abort`, `/model`, `/thinking`, `/name`,
  `/folders`, `/folder-add`, `/folder-remove`, `/status`, `/clear`, `/help`,
  and `/quit` from the composer, with Tab/Enter slash completion and arrow-key
  selection. The conversation drawer provides the same included-folder editor.
- Paste PNG, JPEG, WebP, or GIF clipboard images into a project composer. The
  daemon owns the generated file inside the project sandbox and restores the
  image after reload; pathless chats reject image input.
- Switch among Chat and persistent project application tabs without destroying
  inactive PTYs. The defaults are Terminal, lazygit, and nvim; their shortcuts
  toggle with Cmd/Ctrl+backtick, Cmd/Ctrl+Shift+G, and Cmd/Ctrl+Shift+E.
  Settings → Workspace tabs can reorder them or add direct terminal/TUI and
  desktop launchers. Desktop apps open on the daemon host.
- Render terminal tabs with xterm.js, Cascadia Code from Google Fonts, the
  pinned Nerd Fonts 3.4 symbols-only webfont, Cascadia programming ligatures,
  and WebGL custom box glyphs for continuous nvim/lazygit borders. PTYs
  advertise true color and use a Tokyo Night–compatible palette.
- Reconnect automatically, resubscribe, and catch up persisted event sequence
  gaps without retrying potentially duplicated mutations.
- Manage daemon-wide models, sandbox policy, queues, skills, the global MCP registry,
  extensions, subagent profiles, workspace launchers, standalone defaults, and
  desktop sync from the full-page Settings view. Configuration saves are
  validated and revisioned; MCP secrets remain redacted in the browser.
- Override each registered MCP server per project or chat with Inherit, On, or
  Off from the conversation drawer. Child threads inherit the root project
  choice and can be explicitly narrowed or re-enabled through the daemon API
  when their persisted spawn grant permits it.
- Discover capabilities and skills for the selected conversation.
- Discover and execute extension-provided commands with free-form arguments;
  command output, errors, and daemon-queued prompts remain visible in the
  conversation drawer.
- Spawn and navigate child-agent trees, read their transcripts, send guidance,
  and interrupt active children when the daemon exposes the subagent API.
- Responsive navigation and conversation-detail drawers for smaller screens.

The client understands generic `conversation_id`, standalone `chat_id`,
folder-scoped `project_id`, and legacy `thread_id` event envelopes. Mutating
commands include all applicable identifiers so clients can migrate without
losing an active transcript.

The WebSocket URL is stored only in browser local storage. Set
`VITE_DAEMON_URL` at build time or use Connection settings when a reverse proxy
provides another trusted `ws://` or `wss://` endpoint.

## Verify

```sh
npm run typecheck
npm test
npm run build
```

With the daemon and Vite server running, this creates and deletes one temporary,
folder-scoped project through the same-origin proxy:

```sh
npm run smoke:daemon
```

## Deployment boundary

The daemon's WebSocket and content APIs currently have no transport
authentication. A public HTTPS site also cannot safely connect to a local
insecure `ws://` endpoint. Keep this client local, or put the Go-hosted UI and
daemon behind the same authenticated TLS reverse proxy. Do not expose the Dire
Agent daemon directly to an untrusted network.
