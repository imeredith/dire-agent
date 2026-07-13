import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { environmentFixture, mockState, projectFixture, resetMockDaemon } from "./test/mock-daemon";

vi.mock("./lib/daemon-client", async () => {
  const mock = await import("./test/mock-daemon");
  return {
    DaemonClient: mock.MockDaemonClient,
    unsupported: (error: unknown) => /unknown command|unsupported/i.test(error instanceof Error ? error.message : String(error)),
  };
});

vi.mock("./components/TerminalPanel", () => ({
  TerminalPanel: ({ launcher, active }: { launcher: { id: string; label: string }; active: boolean }) => (
    <div data-testid={`terminal-${launcher.id}`} role="tabpanel" aria-label={`${launcher.label} panel`} aria-hidden={!active}>
      {launcher.label} terminal session
    </div>
  ),
}));

import App from "./App";

describe("project categories and launchers", () => {
  afterEach(() => cleanup());
  beforeEach(() => {
    localStorage.clear();
    resetMockDaemon();
    mockState.projects = [
      { ...projectFixture, id: "project_a", name: "Alpha client project", category: "Client A" },
      { ...projectFixture, id: "project_b", name: "Beta client project", category: "Client B" },
      { ...projectFixture, id: "project_internal", name: "Internal project", category: "Internal" },
    ];
  });

  it("groups projects and persists a category-only screen-sharing view", async () => {
    const user = userEvent.setup();
    const first = render(<App />);
    const filter = await screen.findByLabelText("Project category filter");
    expect(screen.getByRole("region", { name: "Client A projects" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Client B projects" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Beta client project.*workspace/ }));
    expect(await screen.findByRole("heading", { name: "Beta client project" })).toBeInTheDocument();

    await user.selectOptions(filter, "category:Client A");
    expect(screen.getByRole("button", { name: /Alpha client project.*workspace/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Beta client project.*workspace/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Internal project.*workspace/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Beta client project" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Alpha client project" })).toBeInTheDocument();
    expect(localStorage.getItem("dire-agent.project.categoryFilter")).toBe("category:Client A");

    first.unmount();
    render(<App />);
    const restored = await screen.findByLabelText("Project category filter");
    expect(restored).toHaveValue("category:Client A");
    expect(screen.queryByRole("button", { name: /Beta client project.*workspace/ })).not.toBeInTheDocument();
  });

  it("keeps selection order stable and promotes only after a successful prompt", async () => {
    const user = userEvent.setup();
    mockState.projects = [
      { ...projectFixture, id: "project_first", name: "First project", category: "Internal" },
      { ...projectFixture, id: "project_second", name: "Second project", category: "Internal" },
    ];
    render(<App />);

    const group = await screen.findByRole("region", { name: "Internal projects" });
    const names = () => within(group).getAllByRole("button")
      .filter((button) => button.classList.contains("resource-select"))
      .map((button) => button.querySelector("strong")?.textContent);
    expect(names()).toEqual(["First project", "Second project"]);

    await user.click(within(group).getByRole("button", { name: /^Second project\// }));
    expect(await screen.findByRole("heading", { name: "Second project" })).toBeInTheDocument();
    expect(names()).toEqual(["First project", "Second project"]);

    await user.type(screen.getByLabelText("Message the agent"), "Promote this project");
    await user.click(screen.getByRole("button", { name: "Send message" }));
    await waitFor(() => expect(names()).toEqual(["Second project", "First project"]));
  });

  it("shows a loader on a cold history load and restores cached history immediately", async () => {
    const user = userEvent.setup();
    const first = { ...projectFixture, id: "project_cached", name: "Cached project", category: "Internal" };
    const second = { ...projectFixture, id: "project_cold", name: "Cold project", category: "Internal" };
    mockState.projects = [first, second];
    mockState.messages[first.id] = [{
      sequence: 1,
      kind: "message",
      role: "assistant",
      content: "CACHED_PROJECT_HISTORY",
      created_at: "2026-07-10T00:00:01Z",
    }];
    mockState.messages[second.id] = [{
      sequence: 1,
      kind: "message",
      role: "assistant",
      content: "COLD_PROJECT_HISTORY",
      created_at: "2026-07-10T00:00:02Z",
    }];
    let releaseCold: () => void = () => {};
    mockState.messageWaiters[second.id] = new Promise<void>((resolve) => { releaseCold = resolve; });

    render(<App />);
    expect(await screen.findByText("CACHED_PROJECT_HISTORY")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^Cold project\// }));
    expect(screen.getByRole("status")).toHaveTextContent("Loading conversation…");
    expect(screen.queryByText("CACHED_PROJECT_HISTORY")).not.toBeInTheDocument();
    expect(screen.queryByText("What should the agent work on?")).not.toBeInTheDocument();

    releaseCold();
    expect(await screen.findByText("COLD_PROJECT_HISTORY")).toBeInTheDocument();

    let releaseRefresh: () => void = () => {};
    mockState.messageWaiters[first.id] = new Promise<void>((resolve) => { releaseRefresh = resolve; });
    await user.click(screen.getByRole("button", { name: /^Cached project\// }));
    expect(screen.getByText("CACHED_PROJECT_HISTORY")).toBeInTheDocument();
    expect(screen.queryByText("Loading conversation…")).not.toBeInTheDocument();
    releaseRefresh();
  });

  it("creates and edits a project category", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole("button", { name: "New project" }));
    const dialog = screen.getByRole("dialog", { name: "Create project" });
    await user.type(within(dialog).getByPlaceholderText("My project"), "Customer portal");
    await user.type(within(dialog).getByLabelText("Project category"), "Client C");
    await user.type(within(dialog).getByPlaceholderText("/absolute/path/to/project"), "/workspace/client-c");
    await user.type(within(dialog).getByLabelText("Additional sandbox folders"), "/workspace/shared\n/workspace/docs");
    await user.click(within(dialog).getByRole("button", { name: "Create project" }));
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "create_project",
      options: expect.objectContaining({
        name: "Customer portal",
        category: "Client C",
        cwd: "/workspace/client-c",
        additional_folders: ["/workspace/shared", "/workspace/docs"],
      }),
    })));

    await user.click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const drawer = screen.getByRole("complementary", { name: "Conversation details" });
    const category = within(drawer).getByLabelText("Project category");
    fireEvent.change(category, { target: { value: "Client D" } });
    fireEvent.blur(category);
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "set_project_category", category: "Client D",
    })));

    const folders = within(drawer).getByLabelText("Additional sandbox folders");
    await user.clear(folders);
    await user.type(folders, "/workspace/assets\n/workspace/shared");
    await user.click(within(drawer).getByRole("button", { name: "Save sandbox folders" }));
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "set_project_sandbox_folders",
      additional_folders: ["/workspace/assets", "/workspace/shared"],
    })));
    expect(within(drawer).getByText("Main project folder · relative paths start here")).toBeInTheDocument();
  });

  it("overrides the global process-sandbox policy for a project", async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByLabelText("Message the agent");

    await user.click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const drawer = screen.getByRole("complementary", { name: "Conversation details" });
    const sandbox = await within(drawer).findByLabelText("Process sandbox");
    expect(sandbox).toHaveValue("inherit");

    await user.selectOptions(sandbox, "off");
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "set_project_sandbox",
      sandbox: "off",
    })));
    expect(await within(drawer).findByText("Local processes run with the daemon user's permissions.")).toBeInTheDocument();

    await user.selectOptions(sandbox, "inherit");
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "set_project_sandbox",
      sandbox: "inherit",
    })));
    expect(sandbox).toHaveValue("inherit");
  });

  it("creates an inspected worktree from a branch and local environment", async () => {
    const user = userEvent.setup();
    mockState.environments = [environmentFixture];
    mockState.workspaceInspections["/workspace/"] = {
      folder: "/workspace",
      git_repository: true,
      repository_root: "/workspace",
      head: "abc123",
      current_branch: "main",
      branches: ["main", "feature/worktrees"],
      environments: [environmentFixture],
    };
    let finishSetup: () => void = () => undefined;
    mockState.createProjectWaiter = new Promise<void>((resolve) => { finishSetup = resolve; });
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "New project" }));
    const dialog = screen.getByRole("dialog", { name: "Create project" });
    await user.type(within(dialog).getByLabelText("Project name"), "Isolated task");
    await user.selectOptions(within(dialog).getByLabelText("Project workspace"), "worktree");
    await user.type(within(dialog).getByLabelText("Source project folder"), "/workspace/");
    expect(within(dialog).getByRole("button", { name: "Create project" })).toBeDisabled();
    await user.click(within(dialog).getByRole("button", { name: "Inspect source folder" }));

    expect(await within(dialog).findByText("/workspace · main")).toBeInTheDocument();
    await user.clear(within(dialog).getByLabelText("Starting ref"));
    await user.type(within(dialog).getByLabelText("Starting ref"), "feature/worktrees");
    await user.selectOptions(within(dialog).getByLabelText("Local environment"), "environment.toml");
    await user.click(within(dialog).getByRole("button", { name: "Create project" }));

    expect(within(dialog).getByRole("status")).toHaveTextContent("Creating the worktree and running its setup script");
    expect(within(dialog).getByRole("button", { name: "Creating worktree…" })).toBeDisabled();

    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "create_project",
      options: expect.objectContaining({
        name: "Isolated task",
        cwd: "/workspace/",
        worktree: {
          base_ref: "feature/worktrees",
          environment_id: "environment.toml",
          source_project_id: "project_a",
        },
      }),
    })));
    expect(mockState.requests).toContainEqual({ type: "inspect_project_workspace", folder: "/workspace/" });
    finishSetup();
    expect(await screen.findByRole("button", { name: /Isolated task.*Worktree/ })).toBeInTheDocument();

    await user.click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const drawer = screen.getByRole("complementary", { name: "Conversation details" });
    expect(within(drawer).getByText("Isolated worktree")).toBeInTheDocument();
    expect(within(drawer).getByText("feature/worktrees")).toBeInTheDocument();
    expect(within(drawer).getByText("environment.toml")).toBeInTheDocument();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    await user.click(within(drawer).getByRole("button", { name: "Delete project and history · keep worktree · no cleanup" }));
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining(
      "will be preserved, and cleanup scripts will not run",
    ));
    confirm.mockRestore();
  });

  it("edits repo-local lifecycle scripts and project actions", async () => {
    const user = userEvent.setup();
    mockState.environments = [environmentFixture];
    render(<App />);
    await screen.findByLabelText("Message the agent");

    await user.click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const drawer = screen.getByRole("complementary", { name: "Conversation details" });
    await user.click(within(drawer).getByRole("button", { name: "Manage local environments" }));
    const dialog = await screen.findByRole("dialog", { name: "Local environments" });
    expect(await within(dialog).findByDisplayValue("Development")).toBeInTheDocument();

    const setup = within(dialog).getByLabelText("Setup scripts Default");
    await user.clear(setup);
    await user.type(setup, "npm ci\nnpm run build");
    await user.type(within(dialog).getByLabelText("Cleanup scripts Linux"), "npm run clean");
    await user.click(within(dialog).getByRole("button", { name: "Add action" }));
    await user.type(within(dialog).getByLabelText("Action 2 name"), "Dev server");
    await user.selectOptions(within(dialog).getByLabelText("Action 2 icon"), "run");
    await user.selectOptions(within(dialog).getByLabelText("Action 2 platform"), "linux");
    await user.type(within(dialog).getByLabelText("Action 2 command"), "npm run dev");
    await user.click(within(dialog).getByRole("button", { name: "Save environment" }));

    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "put_project_environment",
      project_id: "project_a",
      expected_hash: "environment-hash-1",
      environment: expect.objectContaining({
        id: "environment.toml",
        setup: expect.objectContaining({ script: "npm ci\nnpm run build" }),
        cleanup: expect.objectContaining({ linux: { script: "npm run clean" } }),
        actions: expect.arrayContaining([
          expect.objectContaining({ name: "Dev server", icon: "run", command: "npm run dev", platform: "linux" }),
        ]),
      }),
    })));
    expect(await screen.findByText("Local environment saved")).toBeInTheDocument();
  });

  it("keeps terminal tabs mounted, toggles shortcuts, closes sessions, and launches desktop apps", async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByLabelText("Message the agent");

    await user.click(await screen.findByRole("tab", { name: "Open Terminal" }));
    const terminal = await screen.findByTestId("terminal-shell");
    expect(terminal).toHaveAttribute("aria-hidden", "false");
    await user.click(screen.getByRole("tab", { name: "Hide Terminal" }));
    expect(terminal).toHaveAttribute("aria-hidden", "true");
    expect(screen.getByRole("tab", { name: "Chat" })).toHaveAttribute("aria-selected", "true");

    await user.keyboard("{Control>}{Shift>}g{/Shift}{/Control}");
    const lazygit = await screen.findByTestId("terminal-lazygit");
    expect(lazygit).toHaveAttribute("aria-hidden", "false");
    expect(screen.getByTestId("terminal-shell")).toBe(terminal);
    await user.keyboard("{Control>}{Shift>}g{/Shift}{/Control}");
    expect(lazygit).toHaveAttribute("aria-hidden", "true");

    await user.keyboard("{Control>}{Shift>}e{/Shift}{/Control}");
    expect(await screen.findByTestId("terminal-nvim")).toHaveAttribute("aria-hidden", "false");
    await user.click(screen.getByRole("button", { name: "Close nvim" }));
    expect(screen.queryByTestId("terminal-nvim")).not.toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "Open Finder" }));
    expect(await screen.findByRole("tabpanel", { name: "Finder desktop application" })).toBeInTheDocument();
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "launch_project_app",
      project_id: "project_a",
      launcher_id: "finder",
    })));
  });
});
