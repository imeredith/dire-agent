import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { chatFixture, mockState, projectFixture, resetMockDaemon } from "./test/mock-daemon";

vi.mock("./lib/daemon-client", async () => {
  const mock = await import("./test/mock-daemon");
  return {
    DaemonClient: mock.MockDaemonClient,
    unsupported: (error: unknown) => /unknown command|unsupported/i.test(error instanceof Error ? error.message : String(error)),
  };
});

import App from "./App";

describe("App conversations", () => {
  afterEach(() => cleanup());
  beforeEach(() => {
    localStorage.clear();
    resetMockDaemon();
  });

  it("creates, chats in, and deletes a pathless conversation with generic IDs", async () => {
    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<App />);

    await screen.findByText("Start a conversation");
    await user.click(screen.getAllByRole("button", { name: "New chat" })[0]);
    const dialog = screen.getByRole("dialog", { name: "Create chat" });
    await user.type(within(dialog).getByLabelText("Chat name"), "Ideas");
    await user.click(within(dialog).getByRole("button", { name: "Create chat" }));

    const composer = await screen.findByLabelText("Message the agent");
    await user.type(composer, "hello from web");
    await user.click(screen.getByRole("button", { name: "Send message" }));
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "prompt",
      conversation_id: chatFixture.id,
      chat_id: chatFixture.id,
      thread_id: chatFixture.id,
      message: "hello from web",
    })));
    expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "create_chat",
      chat_options: { name: "Ideas" },
    }));
    expect(screen.getByText("STANDALONE CHAT")).toBeInTheDocument();
    expect(screen.getByText("Standalone chats have no folder tools.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Delete Ideas" }));
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "delete_chat",
      conversation_id: chatFixture.id,
    })));
  });

  it("creates a folder-scoped project, preserves Shift+Enter, and changes models", async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByText("Start a conversation");
    await user.click(screen.getAllByRole("button", { name: "New project" })[0]);
    const dialog = screen.getByRole("dialog", { name: "Create project" });
    await user.type(within(dialog).getByLabelText("Project name"), "Web project");
    await user.type(within(dialog).getByLabelText("Project folder"), "/workspace");
    await user.click(within(dialog).getByRole("button", { name: "Create project" }));

    const composer = await screen.findByLabelText("Message the agent");
    fireEvent.change(composer, { target: { value: "line one" } });
    fireEvent.keyDown(composer, { key: "Enter", shiftKey: true });
    expect(mockState.requests.some((request) => request.type === "prompt")).toBe(false);

    await user.click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const drawer = screen.getByRole("complementary", { name: "Conversation details" });
    const model = within(drawer).getByLabelText("Model");
    expect(within(model).getByRole("option", { name: /gpt-5\.6-luna/ })).toBeInTheDocument();
    expect(within(model).getByRole("option", { name: /server-special/ })).toBeInTheDocument();
    await user.selectOptions(model, "gpt-5.6-luna");
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "set_model",
      project_id: projectFixture.id,
      conversation_id: projectFixture.id,
      model: "gpt-5.6-luna",
    })));
  });

  it("keeps transcript scrolling contained and handles generic usage events", async () => {
    mockState.chats = [chatFixture];
    render(<App />);
    const scroll = await screen.findByTestId("message-scroll");
    expect(scroll).toHaveClass("message-scroll");
    expect(scroll.parentElement).toHaveClass("conversation-panel");
    expect(screen.getByLabelText("Token usage")).toBeInTheDocument();

    await act(async () => {
      mockState.eventListeners.forEach((listener) => listener({
        type: "message_end",
        conversation_id: chatFixture.id,
        chat_id: chatFixture.id,
        thread_id: chatFixture.id,
        sequence: 1,
        timestamp: "2026-07-10T00:01:00Z",
        data: {
          message_id: "message-1",
          text: "Done",
          usage: {
            input_tokens: 90,
            output_tokens: 5,
            cache_read_tokens: 5,
            cache_write_tokens: 2,
            total_tokens: 95,
            context_tokens: 500,
            context_window: 1_000,
          },
        },
      }));
    });
    await waitFor(() => {
      const usage = screen.getByLabelText("Token usage");
      expect(within(usage).getByText("100")).toBeInTheDocument();
      expect(within(usage).getByText("25")).toBeInTheDocument();
      expect(within(usage).getByText("35")).toBeInTheDocument();
      expect(within(usage).getByText("42")).toBeInTheDocument();
      expect(within(usage).getByText("500 / 1,000 (50%)")).toBeInTheDocument();
    });
  });

  it("preserves the saved conversation while initial lists are loading", async () => {
    mockState.chats = [chatFixture];
    mockState.projects = [projectFixture];
    localStorage.setItem("dire-agent.conversation", projectFixture.id);

    render(<App />);

    expect(await screen.findByRole("heading", { name: projectFixture.name })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: chatFixture.name })).not.toBeInTheDocument();
  });

  it("keeps the current transcript visible after a manual disconnect", async () => {
    const user = userEvent.setup();
    mockState.projects = [projectFixture];
    mockState.messages[projectFixture.id] = [{
      sequence: 1,
      kind: "message",
      role: "assistant",
      content: "PERSISTED_OFFLINE_MESSAGE",
      created_at: "2026-07-10T00:00:01Z",
    }];

    render(<App />);
    expect(await screen.findByText("PERSISTED_OFFLINE_MESSAGE")).toBeInTheDocument();
    const composer = screen.getByLabelText("Message the agent");
    await user.type(composer, "/quit{Enter}");

    expect(await screen.findByRole("button", { name: "offline" })).toBeInTheDocument();
    expect(screen.getByText("PERSISTED_OFFLINE_MESSAGE")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "offline" }));
    const dialog = screen.getByRole("dialog", { name: "Connection settings" });
    await user.click(within(dialog).getByRole("button", { name: "Reconnect" }));

    expect(await screen.findByRole("button", { name: "Daemon online" })).toBeInTheDocument();
    expect(screen.getByText("PERSISTED_OFFLINE_MESSAGE")).toBeInTheDocument();
  });

  it("spawns, messages, navigates and interrupts a child agent", async () => {
    const user = userEvent.setup();
    mockState.projects = [projectFixture];
    render(<App />);
    await screen.findByLabelText("Message the agent");
    await user.click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const drawer = screen.getByRole("complementary", { name: "Conversation details" });
    await user.click(within(drawer).getByRole("button", { name: "Spawn" }));
    await user.type(within(drawer).getByLabelText("Name"), "reviewer");
    await user.type(within(drawer).getByLabelText("Task"), "Review the authentication flow");
    await user.click(within(drawer).getByRole("button", { name: /Spawn child agent/ }));

    expect((await within(drawer).findAllByText("reviewer")).length).toBeGreaterThan(0);
    const message = within(drawer).getByLabelText("Message reviewer");
    await user.type(message, "Focus on token validation");
    await user.click(within(drawer).getByRole("button", { name: "Send agent message" }));
    await user.click(within(drawer).getByRole("button", { name: "Interrupt reviewer" }));
    expect(mockState.requests).toContainEqual(expect.objectContaining({ type: "spawn_agent", agent_name: "reviewer" }));
    expect(mockState.requests).toContainEqual(expect.objectContaining({ type: "send_agent_message", agent_id: "agent_review" }));
    expect(mockState.requests).toContainEqual(expect.objectContaining({ type: "interrupt_agent", agent_id: "agent_review" }));

    await act(async () => {
      mockState.eventListeners.forEach((listener) => listener({
        type: "agent_message_sent",
        conversation_id: projectFixture.id,
        project_id: projectFixture.id,
        timestamp: "2026-07-10T00:01:58Z",
        data: { id: "agentmsg_out", from_id: projectFixture.id, to_id: "agent_review", content: "Focus on token validation" },
      }));
      mockState.eventListeners.forEach((listener) => listener({
        type: "agent_message",
        conversation_id: projectFixture.id,
        project_id: projectFixture.id,
        timestamp: "2026-07-10T00:01:59Z",
        data: { from_id: "agent_review", to_id: projectFixture.id, content: "Review complete" },
      }));
      mockState.eventListeners.forEach((listener) => listener({
        type: "agent_completed",
        conversation_id: projectFixture.id,
        project_id: projectFixture.id,
        timestamp: "2026-07-10T00:02:00Z",
        data: {
          agent: { id: "agent_review", name: "reviewer", role: "general", depth: 1, status: "idle" },
          status: "idle",
          result: "All checks pass",
        },
      }));
    });
    await waitFor(() => expect(within(drawer).getAllByText("Focus on token validation")).toHaveLength(1));
    await waitFor(() => expect(within(drawer).getByText("Review complete")).toBeInTheDocument());
    await waitFor(() => expect(within(drawer).getByText("All checks pass")).toBeInTheDocument());
    await waitFor(() => expect(within(drawer).getByText("general · idle")).toBeInTheDocument());
  });
});
