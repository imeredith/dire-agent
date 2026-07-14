---
name: dire-agent
description: Manage persistent Dire Agent chats and folder-scoped agent projects through the local daemon.
---

# Dire Agent desktop bridge

Use the `dire_agent_*` MCP tools to work with the user's local Dire Agent daemon.

- List conversations before assuming an ID.
- Use `dire_agent_create_chat` for a pathless conversation. A standalone chat has no project folder and receives no local file or shell tools.
- Use `dire_agent_create_project` only when the user supplied or confirmed an absolute folder. Project tools remain constrained to that folder.
- Use `dire_agent_send_message` to continue a conversation. It waits for the agentic loop to settle unless `wait` is explicitly false.
- Use `dire_agent_spawn_agent` for a bounded independent subtask, then `dire_agent_list_agents`, `dire_agent_wait_agents`, and `dire_agent_send_agent_message` to coordinate the team. Child agents cannot gain permissions their parent lacks.
- Interrupt a stuck child with `dire_agent_interrupt_agent`; delete only an idle leaf with `dire_agent_delete_agent`.
- Inspect `dire_agent_get_state` for running status, queues, token/cache usage, context fill, skills, and capabilities.
- Never claim that this transfers the current Codex conversation. It operates a separate persistent Dire Agent conversation through standard MCP.

The Dire Agent daemon uses its configured model provider (Codex subscription or OpenRouter). This plugin does not use Codex app-server.
