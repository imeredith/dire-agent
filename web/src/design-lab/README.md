# Dire Agent WebUI design lab

Open `/designs` in the WebUI development server. The route uses fixture data and does not connect to the daemon, so every concept is reviewable in the same state.

## Comparison method

Every mockup contains the same selected project, four-step plan, four agents, three changed files, lifecycle states, context usage, and steering affordance. The gallery can preview every direction at desktop, tablet, and mobile widths.

| # | Direction | Implementation | Product hypothesis |
|---|---|---|---|
| 01 | Command Center | shadcn/ui source components | The strongest all-round hierarchy keeps conversation central while exposing plan, agents, changes, preview, and sandbox state. |
| 02 | Operations Cockpit | Material UI | Progress, health, alerts, and checkpoints should dominate when many long-running jobs are active. |
| 03 | Three-Pane Workbench | Mantine | A project → task → work-surface hierarchy maps well to a developer's existing mental model. |
| 04 | Fleet Console | Ant Design | Teams managing many projects benefit from lifecycle filters, tables, ownership, and explicit review queues. |
| 05 | Accessible Focus Mode | React Aria Components | One dominant task, large readable type, visible focus, and complete keyboard behavior make an excellent accessibility benchmark. |
| 06 | Agent Topology | React Flow | A read-only delegation graph can make child-agent relationships legible without turning the product into a workflow editor. |
| 07 | Attention Inbox | Custom React | Asynchronous work should be grouped by `Running`, `Needs you`, and `Ready to review`, rather than only by recency. |
| 08 | Terminal-First IDE | React, Lucide, existing xterm language | Dire Agent's terminal/TUI integration is differentiating enough to drive the whole shell. |
| 09 | Notebook Workspace | Custom React | Plans, tool calls, changes, and checkpoints can form a readable work journal instead of a stream of cards. |
| 10 | Mobile Companion | Custom React | Mobile should monitor, approve, and steer; it should not imitate a desktop terminal workstation. |

## Findings from comparable products

- [OpenHands](https://docs.openhands.dev/openhands/usage/key-features) makes Chat, Changes, VS Code, Terminal, App Preview, and Browser first-class work surfaces. That directly informed the workbench and command-center tabs.
- [AiderDesk](https://github.com/hotovo/aider-desk) distinguishes projects from tasks and gives review, worktrees, terminals, and agent profiles dedicated surfaces. That informed the project → task hierarchy.
- [Cursor background agents](https://docs.cursor.com/background-agent) separates asynchronous agent monitoring from foreground chat, while its [review flow](https://docs.cursor.com/en/agent/review) emphasizes file-by-file changes. That informed the attention inbox and review states.
- [GitHub Copilot agent sessions](https://docs.github.com/en/enterprise-cloud%40latest/copilot/how-tos/copilot-on-github/use-copilot-agents/manage-and-track-agents) frame the lifecycle as delegate → monitor → review. That informed the fleet console.
- [Cline Plan and Act](https://docs.cline.bot/core-workflows/plan-and-act), [Roo modes and checkpoints](https://roocodeinc.github.io/Roo-Code/features/checkpoints/), and [Windsurf Cascade](https://docs.windsurf.com/windsurf/cascade/cascade) make intent, permission, plans, queues, and recovery visible. Those patterns informed the safety presets, visible plan, steer composer, and checkpoints.
- [goose session management](https://goose-docs.ai/docs/guides/sessions/session-management/) keeps starting and retrieving sessions low-friction. That informed the instant `New task` actions and searchable navigation.

## Recommended synthesis

Use **Command Center** as the base, then borrow three focused ideas:

1. The **Attention Inbox** lifecycle home for returning users.
2. The **Terminal-First IDE** persistent work-surface tabs for active coding sessions.
3. The **Accessible Focus Mode** type scale, focus visibility, and interaction contracts as acceptance criteria.

Keep **Agent Topology** as an optional inspection surface for runs with several child agents. It is useful for observability, but too spatial to replace the primary project/task navigation.

