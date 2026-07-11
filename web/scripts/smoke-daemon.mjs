const url = process.env.DIRE_AGENT_SMOKE_URL || "ws://127.0.0.1:5173/ws";
const socket = new WebSocket(url);
const pending = new Map();

const opened = new Promise((resolve, reject) => {
  socket.addEventListener("open", resolve, { once: true });
  socket.addEventListener("error", () => reject(new Error(`could not connect to ${url}`)), {
    once: true,
  });
});

socket.addEventListener("message", (event) => {
  const message = JSON.parse(String(event.data));
  if (message.type !== "response") return;
  const request = pending.get(message.id);
  if (!request) return;
  pending.delete(message.id);
  if (message.success) request.resolve(message.data);
  else request.reject(new Error(message.error || `${message.command} failed`));
});

function command(type, payload = {}) {
  const id = crypto.randomUUID();
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    socket.send(JSON.stringify({ id, type, ...payload }));
  });
}

async function compatibleCommand(type, legacyType, payload = {}) {
  try {
    return await command(type, payload);
  } catch (error) {
    if (!/unknown command|unsupported|not supported/i.test(String(error))) throw error;
    return command(legacyType, payload);
  }
}

await opened;
let project;
try {
  project = await compatibleCommand("create_project", "create_thread", {
    options: { name: "Web smoke test", cwd: process.cwd() },
  });
  const scope = { project_id: project.id, thread_id: project.id };
  const fetched = await compatibleCommand("get_project", "get_thread", scope);
  const state = await command("get_state", scope);
  if (fetched.id !== project.id || (state.project || state.thread).id !== project.id || state.running) {
    throw new Error("daemon returned an invalid initial project state");
  }
  const updated = await command("set_thinking_level", {
    ...scope,
    level: "high",
  });
  if (updated.thinking_level !== "high") {
    throw new Error("thinking setting was not persisted");
  }
  const projects = (await compatibleCommand("list_projects", "list_threads")) || [];
  if (!projects.some((item) => item.id === project.id)) {
    throw new Error("created project was not listed");
  }
  process.stdout.write(`web daemon smoke passed (${project.id})\n`);
} finally {
  if (project?.id) {
    const scope = { project_id: project.id, thread_id: project.id };
    await compatibleCommand("delete_project", "delete_thread", scope).catch(() => undefined);
  }
  socket.close(1000, "smoke complete");
}
