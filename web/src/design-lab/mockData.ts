export const projects = [
  { name: "Dire Agent WebUI", meta: "3 active tasks", status: "running" },
  { name: "Checkout reliability", meta: "Needs review", status: "review" },
  { name: "Docs refresh", meta: "Idle · 2h", status: "idle" },
  { name: "Telemetry cleanup", meta: "Done yesterday", status: "done" },
] as const;

export const agents = [
  { id: "root", name: "orchestrator", role: "Coordinating the rollout", status: "Running", progress: 68 },
  { id: "ux", name: "ux-reviewer", role: "Comparing agent workspaces", status: "Done", progress: 100 },
  { id: "ui", name: "ui-builder", role: "Building responsive shell", status: "Running", progress: 54 },
  { id: "qa", name: "qa", role: "Waiting on preview build", status: "Blocked", progress: 30 },
] as const;

export const steps = [
  { label: "Map the current information architecture", state: "done" },
  { label: "Prototype the parallel-agent workspace", state: "done" },
  { label: "Wire the Changes and Preview surfaces", state: "active" },
  { label: "Validate keyboard and mobile flows", state: "queued" },
] as const;

export const changes = [
  { file: "src/design-lab/DesignLabApp.tsx", delta: "+186", state: "modified" },
  { file: "src/design-lab/design-lab.css", delta: "+412", state: "modified" },
  { file: "src/main.tsx", delta: "+6", state: "modified" },
] as const;

export const activity = [
  { time: "10:42", agent: "ui-builder", text: "Created the responsive comparison shell", tone: "running" },
  { time: "10:39", agent: "ux-reviewer", text: "Recommended a task-first hierarchy", tone: "done" },
  { time: "10:34", agent: "orchestrator", text: "Checkpoint saved before dependency changes", tone: "checkpoint" },
] as const;

export const conversation = [
  {
    role: "you",
    text: "Explore the design space of the WebUI. Keep the power-user workflows, but make parallel work much easier to scan.",
  },
  {
    role: "agent",
    text: "I’m treating the project, task, and agent run as separate layers. The first pass adds a visible plan, a review surface, and a clearer safety mode.",
  },
] as const;

