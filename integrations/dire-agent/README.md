# Dire Agent Codex desktop plugin

This plugin launches `dire-agent mcp`, which connects to the local daemon at
`ws://127.0.0.1:7331/ws`. It does not use Codex app-server.

Install `dire-agent` on the desktop app's PATH, run `dire-agent start`, then add
this directory as a local Codex plugin or marketplace source. The Web UI's
Desktop settings page reports the same paths and health.

The bridge also exposes persistent agent-team operations for spawning,
listing, messaging, waiting, interrupting, and deleting child agents.
