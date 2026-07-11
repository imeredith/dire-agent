# Dire Agent Codex desktop plugin

This plugin launches the `dire-agent-mcp` companion, which connects to the local
daemon at `ws://127.0.0.1:7331/ws`. It does not use Codex app-server.

Build or install the companion so `dire-agent-mcp` is on the desktop app's PATH,
start `dire-agentd`, then add this directory as a local Codex plugin or marketplace
source. The Web UI's Desktop settings page reports the same paths and health.

The bridge also exposes persistent agent-team operations for spawning,
listing, messaging, waiting, interrupting, and deleting child agents.
