import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
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

describe("per-conversation MCP registry overrides", () => {
  afterEach(() => cleanup());
  beforeEach(() => {
    localStorage.clear();
    resetMockDaemon();
    mockState.config.global.mcp.servers = {
      docs: {
        transport: "stdio",
        command: "docs-mcp",
        args: [],
        approval: "on-request",
        enabled: true,
      },
    };
    mockState.capabilities = {
      capabilities: [
        { name: "read", source: "builtin", enabled: true, status: "ready" },
        { name: "mcp:docs", source: "mcp", description: "stdio MCP server", enabled: true, status: "ready" },
      ],
      skills: [],
      skill_diagnostics: [],
    };
  });

  it("turns a global MCP server off for a project and resets it to inherit", async () => {
    const user = userEvent.setup();
    mockState.projects = [projectFixture];
    render(<App />);

    await screen.findByLabelText("Message the agent");
    await user.click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const drawer = screen.getByRole("complementary", { name: "Conversation details" });
    const registry = within(drawer).getByText("MCP registry").closest("section")!;
    const docs = within(registry).getByRole("group", { name: "docs MCP server" });
    const override = within(docs).getByLabelText("MCP override for docs");

    expect(override).toHaveValue("inherit");
    expect(within(docs).getByText("Enabled · ready")).toBeInTheDocument();

    await user.selectOptions(override, "off");
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "set_mcp_server_enabled",
      conversation_id: projectFixture.id,
      project_id: projectFixture.id,
      thread_id: projectFixture.id,
      mcp_server: "docs",
      enabled: false,
    })));
    await waitFor(() => expect(override).toHaveValue("off"));
    await waitFor(() => expect(within(docs).getByText("Disabled · disabled")).toBeInTheDocument());
    expect(mockState.projects[0].mcp_server_overrides).toEqual({ docs: false });

    await user.selectOptions(override, "inherit");
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "set_mcp_server_enabled",
      conversation_id: projectFixture.id,
      project_id: projectFixture.id,
      mcp_server: "docs",
      enabled: null,
    })));
    await waitFor(() => expect(override).toHaveValue("inherit"));
    await waitFor(() => expect(within(docs).getByText("Enabled · ready")).toBeInTheDocument());
    expect(mockState.projects[0].mcp_server_overrides).toBeUndefined();
  });

  it("applies the same registry override controls to a pathless chat", async () => {
    const user = userEvent.setup();
    mockState.chats = [chatFixture];
    render(<App />);

    await screen.findByLabelText("Message the agent");
    await user.click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const drawer = screen.getByRole("complementary", { name: "Conversation details" });
    const override = within(drawer).getByLabelText("MCP override for docs");
    await user.selectOptions(override, "off");

    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "set_mcp_server_enabled",
      conversation_id: chatFixture.id,
      chat_id: chatFixture.id,
      thread_id: chatFixture.id,
      mcp_server: "docs",
      enabled: false,
    })));
    expect(mockState.chats[0].mcp_server_overrides).toEqual({ docs: false });
  });

  it("distinguishes configured enablement from a failed server state", async () => {
    mockState.projects = [projectFixture];
    mockState.capabilities.capabilities[1] = {
      ...mockState.capabilities.capabilities[1],
      enabled: true,
      status: "error",
    };
    render(<App />);

    await screen.findByLabelText("Message the agent");
    await userEvent.setup().click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const status = screen.getByText("Enabled · error");
    expect(status.querySelector(".status-muted")).toBeInTheDocument();
    expect(status.querySelector(".status-ready")).not.toBeInTheDocument();
  });
});
