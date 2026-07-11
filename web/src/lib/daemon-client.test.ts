import { describe, expect, it, vi } from "vitest";
import { DaemonClient } from "./daemon-client";

class FakeWebSocket extends EventTarget {
	readyState: number = WebSocket.CONNECTING;
  sent: string[] = [];

  send(data: string) {
    this.sent.push(data);
  }

  open() {
    this.readyState = WebSocket.OPEN;
    this.dispatchEvent(new Event("open"));
  }

  message(value: unknown) {
    this.dispatchEvent(new MessageEvent("message", { data: JSON.stringify(value) }));
  }

  close() {
    this.readyState = WebSocket.CLOSED;
    this.dispatchEvent(new CloseEvent("close", { code: 1000 }));
  }
}

function harness() {
  let socket: FakeWebSocket | undefined;
  const client = new DaemonClient("ws://example.test/ws", () => {
    socket = new FakeWebSocket();
    return socket as unknown as WebSocket;
  });
  return { client, socket: () => socket! };
}

describe("DaemonClient", () => {
  it("gives worktree creation headroom beyond the setup timeout", async () => {
    const client = new DaemonClient("ws://example.test/ws");
    const project = {
      id: "project_1", kind: "project" as const, model: "gpt-test", thinking_level: "medium" as const,
      steering_mode: "one-at-a-time" as const, follow_up_mode: "one-at-a-time" as const,
      tools: [], status: "idle", created_at: "now", updated_at: "now",
    };
    const request = vi.spyOn(client, "request").mockResolvedValue(project);

    await client.createProject({
      name: "Isolated task",
      cwd: "/repo",
      worktree: { base_ref: "main", environment_id: "environment.toml" },
    });

    expect(request).toHaveBeenCalledWith({
      type: "create_project",
      options: {
        name: "Isolated task",
        cwd: "/repo",
        worktree: { base_ref: "main", environment_id: "environment.toml" },
      },
    }, 900_000);
  });

  it("correlates out-of-order responses and routes unsolicited events", async () => {
    const { client, socket } = harness();
    const events: string[] = [];
    client.onEvent((event) => events.push(event.type));
    const connecting = client.connect();
    socket().open();
    await connecting;

    const first = client.request<{ value: number }>({ type: "first" });
    const second = client.request<{ value: number }>({ type: "second" });
    const firstRequest = JSON.parse(socket().sent[0]);
    const secondRequest = JSON.parse(socket().sent[1]);

    socket().message({
      type: "message_update",
      thread_id: "thread_test",
      timestamp: "now",
      data: { delta: "hi" },
    });
    socket().message({
      id: secondRequest.id,
      type: "response",
      command: "second",
      success: true,
      data: { value: 2 },
    });
    socket().message({
      id: firstRequest.id,
      type: "response",
      command: "first",
      success: true,
      data: { value: 1 },
    });

    await expect(first).resolves.toEqual({ value: 1 });
    await expect(second).resolves.toEqual({ value: 2 });
    expect(events).toEqual(["message_update"]);
  });

  it("rejects server errors and all pending requests on disconnect", async () => {
    const { client, socket } = harness();
    const connecting = client.connect();
    socket().open();
    await connecting;

    const failed = client.request<void>({ type: "abort" });
    const request = JSON.parse(socket().sent[0]);
    socket().message({
      id: request.id,
      type: "response",
      command: "abort",
      success: false,
      error: "thread is idle",
    });
    await expect(failed).rejects.toThrow("thread is idle");

    const pending = client.request<void>({ type: "get_state" });
    socket().close();
    await expect(pending).rejects.toThrow("connection lost");
  });

  it("sends the exact command envelope", async () => {
    const { client, socket } = harness();
    const connecting = client.connect();
    socket().open();
    await connecting;

    const result = client.request<void>({
      type: "prompt",
      thread_id: "thread_1",
      message: "hello",
      streaming_behavior: "followUp",
    });
    const sent = JSON.parse(socket().sent[0]);
    expect(sent).toMatchObject({
      type: "prompt",
      thread_id: "thread_1",
      message: "hello",
      streaming_behavior: "followUp",
    });
    expect(sent.id).toEqual(expect.any(String));
    socket().message({
      id: sent.id,
      type: "response",
      command: "prompt",
      success: true,
    });
    await expect(result).resolves.toBeUndefined();
  });

  it("scopes subagent commands to chats and correlates the created agent", async () => {
    const { client, socket } = harness();
    const connecting = client.connect();
    socket().open();
    await connecting;
    const chat = {
      id: "chat_1", kind: "chat" as const, model: "gpt-test", thinking_level: "medium" as const,
      steering_mode: "one-at-a-time" as const, follow_up_mode: "one-at-a-time" as const,
      tools: [], status: "idle", created_at: "now", updated_at: "now",
    };
    const result = client.spawnAgent(chat, {
      agent_name: "reviewer",
      agent_role: "review",
      task: "Review changes",
    });
    const sent = JSON.parse(socket().sent[0]);
    expect(sent).toMatchObject({
      type: "spawn_agent",
      conversation_id: "chat_1",
      chat_id: "chat_1",
      thread_id: "chat_1",
      parent_id: "chat_1",
      agent_name: "reviewer",
      agent_role: "review",
      task: "Review changes",
    });
    socket().message({
      id: sent.id,
      type: "response",
      command: "spawn_agent",
      success: true,
      data: { id: "agent_1", agent_name: "reviewer", depth: 1, status: "running" },
    });
    await expect(result).resolves.toMatchObject({ id: "agent_1", status: "running" });

    const messageResult = client.sendAgentMessage(chat, "agent_1", "Focus on auth");
    const messageRequest = JSON.parse(socket().sent[1]);
    expect(messageRequest).toMatchObject({
      type: "send_agent_message",
      conversation_id: "chat_1",
      parent_id: "chat_1",
      agent_id: "agent_1",
      message: "Focus on auth",
      wake: true,
    });
    socket().message({
      id: messageRequest.id,
      type: "response",
      command: "send_agent_message",
      success: true,
      data: { id: "agentmsg_1", from_id: "chat_1", to_id: "agent_1", content: "Focus on auth" },
    });
    await expect(messageResult).resolves.toMatchObject({ id: "agentmsg_1", to_id: "agent_1" });
  });

  it("lists and executes conversation-scoped capability commands", async () => {
    const { client, socket } = harness();
    const connecting = client.connect();
    socket().open();
    await connecting;
    const project = {
      id: "project_1", kind: "project" as const, model: "gpt-test", thinking_level: "medium" as const,
      steering_mode: "one-at-a-time" as const, follow_up_mode: "one-at-a-time" as const,
      tools: [], status: "idle", created_at: "now", updated_at: "now",
    };

    const listed = client.listCapabilityCommands(project);
    const listRequest = JSON.parse(socket().sent[0]);
    expect(listRequest).toMatchObject({
      type: "list_capability_commands",
      conversation_id: "project_1",
      project_id: "project_1",
      thread_id: "project_1",
    });
    socket().message({
      id: listRequest.id,
      type: "response",
      command: "list_capability_commands",
      success: true,
      data: [{ name: "release", description: "Prepare release", source: "extension:test" }],
    });
    await expect(listed).resolves.toEqual([
      { name: "release", description: "Prepare release", source: "extension:test" },
    ]);

    const executed = client.executeCapabilityCommand(project, "release", "v2.0");
    const executeRequest = JSON.parse(socket().sent[1]);
    expect(executeRequest).toMatchObject({
      type: "execute_capability_command",
      conversation_id: "project_1",
      command_name: "release",
      arguments: "v2.0",
    });
    socket().message({
      id: executeRequest.id,
      type: "response",
      command: "execute_capability_command",
      success: true,
      data: { output: "done", prompt: "continue" },
    });
    await expect(executed).resolves.toEqual({ output: "done", prompt: "continue" });
  });

  it("lists configured launchers and launches desktop apps by ID only", async () => {
    const { client, socket } = harness();
    const connecting = client.connect();
    socket().open();
    await connecting;
    const project = {
      id: "project_1", kind: "project" as const, model: "gpt-test", thinking_level: "medium" as const,
      steering_mode: "one-at-a-time" as const, follow_up_mode: "one-at-a-time" as const,
      tools: [], status: "idle", created_at: "now", updated_at: "now",
    };

    const listed = client.getProjectLaunchers(project);
    const listRequest = JSON.parse(socket().sent[0]);
    expect(listRequest).toMatchObject({
      type: "get_project_launchers",
      conversation_id: "project_1",
      project_id: "project_1",
      thread_id: "project_1",
    });
    socket().message({
      id: listRequest.id,
      type: "response",
      command: "get_project_launchers",
      success: true,
      data: [{ id: "code", label: "Code", kind: "desktop", command: "/usr/bin/open", args: ["."] }],
    });
    await expect(listed).resolves.toEqual([
      { id: "code", label: "Code", kind: "desktop", command: "/usr/bin/open", args: ["."] },
    ]);

    const launched = client.launchProjectApp(project, "code");
    const launchRequest = JSON.parse(socket().sent[1]);
    expect(launchRequest).toMatchObject({
      type: "launch_project_app",
      conversation_id: "project_1",
      project_id: "project_1",
      thread_id: "project_1",
      launcher_id: "code",
    });
    expect(launchRequest).not.toHaveProperty("command");
    expect(launchRequest).not.toHaveProperty("args");
    socket().message({
      id: launchRequest.id,
      type: "response",
      command: "launch_project_app",
      success: true,
      data: { launched: true, id: "code", label: "Code" },
    });
    await expect(launched).resolves.toEqual({ launched: true, id: "code", label: "Code" });
  });

  it("inspects repositories and edits project-local environments", async () => {
    const { client, socket } = harness();
    const connecting = client.connect();
    socket().open();
    await connecting;
    const project = {
      id: "project_1", kind: "project" as const, model: "gpt-test", thinking_level: "medium" as const,
      steering_mode: "one-at-a-time" as const, follow_up_mode: "one-at-a-time" as const,
      tools: [], status: "idle", created_at: "now", updated_at: "now",
    };
    const environment = {
      id: "environment.toml",
      version: 1,
      name: "Development",
      setup: { script: "npm install" },
      actions: [{ name: "Test", icon: "test" as const, command: "npm test" }],
    };

    const inspected = client.inspectProjectWorkspace("/repo");
    const inspectRequest = JSON.parse(socket().sent[0]);
    expect(inspectRequest).toMatchObject({ type: "inspect_project_workspace", folder: "/repo" });
    socket().message({
      id: inspectRequest.id, type: "response", command: "inspect_project_workspace", success: true,
      data: { folder: "/repo", git_repository: true, repository_root: "/repo", branches: null, environments: null },
    });
    await expect(inspected).resolves.toMatchObject({ branches: [], environments: [] });

    const listed = client.getProjectEnvironments(project);
    const listRequest = JSON.parse(socket().sent[1]);
    expect(listRequest).toMatchObject({
      type: "get_project_environments",
      conversation_id: "project_1",
      project_id: "project_1",
    });
    socket().message({
      id: listRequest.id, type: "response", command: "get_project_environments", success: true,
      data: [{ ...environment, actions: null }],
    });
    await expect(listed).resolves.toEqual([{ ...environment, actions: [] }]);

    const saved = client.putProjectEnvironment(project, environment, "old-hash");
    const putRequest = JSON.parse(socket().sent[2]);
    expect(putRequest).toMatchObject({
      type: "put_project_environment",
      project_id: "project_1",
      environment_id: "environment.toml",
      environment,
      expected_hash: "old-hash",
    });
    socket().message({
      id: putRequest.id, type: "response", command: "put_project_environment", success: true,
      data: { ...environment, hash: "new-hash" },
    });
    await expect(saved).resolves.toMatchObject({ id: "environment.toml", hash: "new-hash" });

    const deleted = client.deleteProjectEnvironment(project, "environment.toml", "new-hash");
    const deleteRequest = JSON.parse(socket().sent[3]);
    expect(deleteRequest).toMatchObject({
      type: "delete_project_environment",
      project_id: "project_1",
      environment_id: "environment.toml",
      expected_hash: "new-hash",
    });
    socket().message({
      id: deleteRequest.id, type: "response", command: "delete_project_environment", success: true,
      data: { deleted: true, id: "environment.toml" },
    });
    await expect(deleted).resolves.toEqual({ deleted: true, id: "environment.toml" });
  });
});
