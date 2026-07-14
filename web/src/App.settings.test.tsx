import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mockState, resetMockDaemon } from "./test/mock-daemon";

vi.mock("./lib/daemon-client", async () => {
  const mock = await import("./test/mock-daemon");
  return {
    DaemonClient: mock.MockDaemonClient,
    unsupported: (error: unknown) => /unknown command|unsupported/i.test(error instanceof Error ? error.message : String(error)),
  };
});

import App from "./App";

describe("App settings", () => {
  afterEach(() => cleanup());
  beforeEach(() => {
    localStorage.clear();
    resetMockDaemon();
  });

  async function openSettings(user: ReturnType<typeof userEvent.setup>) {
    render(<App />);
    await screen.findByText("Start a conversation");
    await user.click(screen.getByRole("button", { name: "Settings" }));
    await screen.findByRole("heading", { name: "Configure how every agent works." });
  }

  it("validates and saves settings with an optimistic revision", async () => {
    const user = userEvent.setup();
    await openSettings(user);
    const section = screen.getByRole("heading", { name: "Model and reasoning" }).closest("section")!;
    const model = within(section).getByLabelText("Model");
    await user.clear(model);
    await user.type(model, "gpt-5.6-luna");
    await user.click(screen.getAllByRole("button", { name: "Save changes" })[0]);

    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "config_validate",
      config: expect.objectContaining({ global: expect.objectContaining({ model: expect.objectContaining({ id: "gpt-5.6-luna" }) }) }),
    })));
    expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "config_update",
      expected_revision: 3,
    }));
    expect(await screen.findByText("Configuration saved")).toBeInTheDocument();
  });

  it("chooses safe model defaults when the provider changes", async () => {
    const user = userEvent.setup();
    await openSettings(user);
    const general = screen.getByRole("heading", { name: "Model and reasoning" }).closest("section")!;
    const provider = within(general).getByLabelText("Provider");
    await user.selectOptions(provider, "openrouter");
    expect(within(general).getByLabelText("Model")).toHaveValue("openrouter/auto");
    expect(within(general).getByLabelText(/Context window/)).toHaveValue(0);
    const standalone = screen.getByRole("heading", { name: "Standalone chat defaults" }).closest("section")!;
    expect(within(standalone).getByLabelText("Model")).toHaveValue("openrouter/auto");
  });

  it("sets the global process-sandbox default", async () => {
    const user = userEvent.setup();
    await openSettings(user);
    const tools = screen.getByRole("heading", { name: "Tools, sandbox and queues" }).closest("section")!;
    await user.selectOptions(within(tools).getByLabelText("Process sandbox default"), "off");
    await user.click(screen.getAllByRole("button", { name: "Save changes" })[0]);

    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "config_update",
      config: expect.objectContaining({
        global: expect.objectContaining({
          tools: expect.objectContaining({ sandbox: "off" }),
        }),
      }),
    })));
  });

  it("surfaces revision conflicts and reloads the authoritative document", async () => {
    const user = userEvent.setup();
    mockState.configConflict = true;
    await openSettings(user);
    const section = screen.getByRole("heading", { name: "Model and reasoning" }).closest("section")!;
    const provider = within(section).getByLabelText("Provider");
    await user.selectOptions(provider, "openrouter");
    await user.click(screen.getAllByRole("button", { name: "Save changes" })[0]);

    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Configuration changed elsewhere")).toBeInTheDocument();
    await user.click(within(alert).getByRole("button", { name: "Reload latest" }));
    await waitFor(() => expect(mockState.requests.filter((item) => item.type === "config_get")).toHaveLength(2));
  });

  it("edits skill trust, MCP, extension, subagent and desktop sections", async () => {
    const user = userEvent.setup();
    await openSettings(user);
    const skills = screen.getByRole("heading", { name: "Agent skills" }).closest("section")!;
    await user.selectOptions(within(skills).getByLabelText("Trust policy"), "trusted");
    const mcp = screen.getByRole("heading", { name: "MCP servers" }).closest("section")!;
    await user.type(within(mcp).getByLabelText("Server name"), "docs");
    await user.click(within(mcp).getByRole("button", { name: "Add server" }));
    expect(within(mcp).getByText("docs")).toBeInTheDocument();
    const subagents = screen.getByRole("heading", { name: "Subagents" }).closest("section")!;
    await user.clear(within(subagents).getByLabelText("Maximum depth"));
    await user.type(within(subagents).getByLabelText("Maximum depth"), "3");
    expect(screen.getByRole("heading", { name: "Codex and ChatGPT apps" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Extensions and plugins" })).toBeInTheDocument();
  });

  it("configures ordered terminal, TUI, and desktop workspace tabs", async () => {
    const user = userEvent.setup();
    await openSettings(user);
    const launchers = screen.getByRole("heading", { name: "Workspace tabs" }).closest("section")!;
    expect(within(launchers).getByText("lazygit")).toBeInTheDocument();
    await user.click(within(launchers).getByRole("button", { name: "Add desktop" }));
    const card = within(launchers).getByRole("article", { name: "Desktop app launcher" });
    const label = within(card).getByLabelText("Label");
    await user.clear(label);
    await user.type(label, "Visual Studio Code");
    await user.type(within(card).getByLabelText(/^Command/), "/usr/bin/open");
    await user.type(within(card).getByLabelText(/^Arguments/), "-a\nVisual Studio Code\n.");
    await user.type(within(card).getByLabelText(/^Shortcut/), "mod+shift+c");
    await user.click(within(launchers).getByRole("button", { name: "Move Visual Studio Code up" }));
    await user.click(screen.getAllByRole("button", { name: "Save changes" })[0]);

    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "config_validate",
      config: expect.objectContaining({
        global: expect.objectContaining({
          launchers: expect.arrayContaining([
            expect.objectContaining({
              id: "desktop",
              label: "Visual Studio Code",
              kind: "desktop",
              command: "/usr/bin/open",
              args: ["-a", "Visual Studio Code", "."],
              shortcut: "mod+shift+c",
            }),
          ]),
        }),
      }),
    })));
  });
});
