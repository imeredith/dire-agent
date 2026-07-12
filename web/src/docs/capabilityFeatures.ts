import type { FeatureDoc } from "./types";

export const capabilityFeatures: FeatureDoc[] = [
  {
    slug: "skills",
    title: "Agent Skills",
    group: "Capabilities",
    summary: "Discover trusted SKILL.md packages progressively and invoke them explicitly from chat.",
    prerequisites: ["Create a valid skill under a configured global root or a project `.agents/skills/<name>/SKILL.md` path."],
    steps: [
      { action: "Open Settings → Skills and confirm the skill root is listed; set trust to Trusted and save.", expected: "The configuration revision increments and no validation alert appears." },
      { action: "Reconnect or reopen the matching project, then open conversation details.", expected: "The skill appears in Capabilities with its metadata; full instructions are not injected yet." },
      { action: "Send `$<skill-name> perform its documented test action`.", expected: "The daemon loads the trusted instructions for that run and the agent follows them." },
      { action: "Set skill trust to Denied, save, and retry.", expected: "The skill is no longer executable and the trust boundary is visible in diagnostics/capabilities." },
    ],
  },
  {
    slug: "mcp",
    title: "MCP tools, resources, and prompts",
    group: "Capabilities",
    summary: "Connect stdio or Streamable HTTP MCP servers and expose bounded tools, resources, and prompts.",
    prerequisites: ["Have a safe test MCP server command or same-origin HTTP endpoint available."],
    steps: [
      { action: "Open Settings → Global MCP registry, add a server ID, choose its transport, and enter the command/URL and arguments.", expected: "A reusable global server definition is created with allowlist and approval policy controls." },
      { action: "Set Enabled by default, use approval Never for the trusted fixture, and save.", expected: "The revision increments; secret environment/header values are redacted when configuration is read back." },
      { action: "Open a conversation’s details and choose Inherit, On, or Off for the server.", expected: "The conversation follows the global default or persists only its local enablement override." },
      { action: "Inspect Capabilities.", expected: "Discovered tools use `mcp__SERVER__TOOL`; resource and prompt helpers use `mcpctx__SERVER__…`." },
      { action: "Ask the agent to call the fixture tool, then list/read its fixture resource and prompt.", expected: "Tool activity and bounded results appear in the transcript; errors remain attributed to the server." },
      { action: "Set the conversation override to Off.", expected: "The server disconnects for that conversation while remaining available to other projects and chats." },
    ],
  },
  {
    slug: "extensions",
    title: "Extensions and commands",
    group: "Capabilities",
    summary: "Run trusted out-of-process adapters with tools, slash commands, prompt fragments, and lifecycle hooks.",
    prerequisites: ["Have an NDJSON extension adapter that implements `extensions/PROTOCOL.md`."],
    steps: [
      { action: "Open Settings → Extensions, add a local source ID, command, and arguments; mark it Enabled and Trusted.", expected: "The extension card shows its sandbox and executable configuration without loading code into the daemon." },
      { action: "Save and reopen conversation details.", expected: "Extension tools appear as `ext__EXTENSION__TOOL`; registered commands appear in the Commands panel." },
      { action: "Run a registered command with fixture arguments from the Commands panel.", expected: "Its output or error is shown, and any returned prompt is queued visibly." },
      { action: "Ask the model to use the fixture extension tool.", expected: "Before/after hooks run around the model/tool cycle and tool output appears in the transcript." },
      { action: "Remove trust and save.", expected: "The adapter stops and its commands/tools are no longer exposed." },
    ],
  },
  {
    slug: "configuration",
    title: "Revisioned configuration",
    group: "Operations",
    summary: "Edit global defaults and integration policy through one validated, secret-redacted configuration document.",
    prerequisites: ["The daemon is online and no other client is intentionally editing the same configuration."],
    steps: [
      { action: "Open Settings and note the revision shown in the left index.", expected: "All sections load from one daemon snapshot and Save changes is disabled initially." },
      { action: "Change the default model to `gpt-5.6-luna`, verify Context window is `372000`, and adjust a queue default.", expected: "Save controls become enabled and the draft remains local until saved." },
      { action: "Click Save changes.", expected: "The entire document validates, the revision increments, and the dirty indicator clears." },
      { action: "Reload Settings.", expected: "The saved values return; configured secrets display `[redacted]` rather than plaintext." },
      { action: "If a second client saves first, save the stale draft.", expected: "A revision-conflict alert offers Reload latest instead of overwriting newer changes." },
    ],
  },
];
