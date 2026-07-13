import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  chatFixture,
  mockState,
  projectFixture,
  resetMockDaemon,
  scheduledPromptFixture,
} from "./test/mock-daemon";

vi.mock("./lib/daemon-client", async () => {
  const mock = await import("./test/mock-daemon");
  return {
    DaemonClient: mock.MockDaemonClient,
    unsupported: (error: unknown) => /unknown command|unsupported/i.test(error instanceof Error ? error.message : String(error)),
  };
});

import App from "./App";

describe("scheduled prompts", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  beforeEach(() => {
    localStorage.clear();
    resetMockDaemon();
    mockState.projects = [{ ...projectFixture }];
    mockState.chats = [{ ...chatFixture }];
  });

  it("lists, toggles, runs, edits, and deletes scheduled prompts", async () => {
    const user = userEvent.setup();
    mockState.schedules = [{ ...scheduledPromptFixture }];
    vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "Scheduled prompts" }));
    const card = await screen.findByRole("article", { name: "Weekday review" });
    expect(within(card).getByText("Web project")).toBeInTheDocument();
    expect(within(card).getByText(/0 9 \* \* 1-5/)).toBeInTheDocument();
    expect(mockState.requests).toContainEqual({ type: "subscribe_scheduled_prompts" });

    await user.click(within(card).getByRole("checkbox", { name: "Enable Weekday review" }));
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "update_scheduled_prompt",
      schedule_id: "schedule_web_test",
      schedule: expect.objectContaining({ enabled: false }),
    })));

    const updatedCard = screen.getByRole("article", { name: "Weekday review" });
    await user.click(within(updatedCard).getByRole("button", { name: "Run Weekday review now" }));
    await waitFor(() => expect(mockState.requests).toContainEqual({
      type: "run_scheduled_prompt",
      schedule_id: "schedule_web_test",
    }));

    await user.click(within(screen.getByRole("article", { name: "Weekday review" })).getByRole("button", { name: "Edit Weekday review" }));
    const editDialog = screen.getByRole("dialog", { name: "Edit scheduled prompt" });
    expect(within(editDialog).getByLabelText("Target")).toHaveValue(`project:${projectFixture.id}`);
    await user.clear(within(editDialog).getByLabelText("Cron expression"));
    await user.type(within(editDialog).getByLabelText("Cron expression"), "30 8 * * *");
    await user.click(within(editDialog).getByRole("button", { name: "Save scheduled prompt" }));
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "update_scheduled_prompt",
      schedule_id: "schedule_web_test",
      schedule: expect.objectContaining({ cron: "30 8 * * *" }),
    })));

    await user.click(within(screen.getByRole("article", { name: "Weekday review" })).getByRole("button", { name: "Delete Weekday review" }));
    await waitFor(() => expect(mockState.requests).toContainEqual({
      type: "delete_scheduled_prompt",
      schedule_id: "schedule_web_test",
    }));
    expect(screen.queryByRole("article", { name: "Weekday review" })).not.toBeInTheDocument();
  });

  it("creates recurring project prompts and one-time one-off chats", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole("button", { name: "Scheduled prompts" }));
    await user.click((await screen.findAllByRole("button", { name: "New scheduled prompt" }))[0]);

    let dialog = screen.getByRole("dialog", { name: "Create scheduled prompt" });
    await user.type(within(dialog).getByLabelText("Name"), "Project pulse");
    await user.type(within(dialog).getByLabelText("Prompt"), "Summarize project progress");
    await user.selectOptions(within(dialog).getByLabelText("Target"), `project:${projectFixture.id}`);
    await user.clear(within(dialog).getByLabelText("Cron expression"));
    await user.type(within(dialog).getByLabelText("Cron expression"), "@daily");
    await user.click(within(dialog).getByRole("button", { name: "Create scheduled prompt" }));
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "create_scheduled_prompt",
      schedule: expect.objectContaining({
        name: "Project pulse",
        prompt: "Summarize project progress",
        target_type: "project",
        conversation_id: projectFixture.id,
        schedule_type: "cron",
        cron: "@daily",
        enabled: true,
      }),
    })));

    await user.click(screen.getByRole("button", { name: "New scheduled prompt" }));
    dialog = screen.getByRole("dialog", { name: "Create scheduled prompt" });
    await user.type(within(dialog).getByLabelText("Name"), "One-time research");
    await user.type(within(dialog).getByLabelText("Prompt"), "Research this topic");
    await user.selectOptions(within(dialog).getByLabelText("Schedule type"), "once");
    await user.clear(within(dialog).getByLabelText("Run at"));
    await user.type(within(dialog).getByLabelText("Run at"), "2026-08-15T14:30");
    await user.click(within(dialog).getByRole("button", { name: "Create scheduled prompt" }));
    await waitFor(() => expect(mockState.requests).toContainEqual(expect.objectContaining({
      type: "create_scheduled_prompt",
      schedule: expect.objectContaining({
        name: "One-time research",
        target_type: "one_off",
        schedule_type: "once",
        run_at: expect.stringMatching(/^2026-08-15T/),
      }),
    })));
    const oneOffRequest = mockState.requests.find((item) => item.type === "create_scheduled_prompt" && item.schedule?.name === "One-time research");
    expect(oneOffRequest?.schedule).not.toHaveProperty("conversation_id");
    expect(oneOffRequest?.schedule).not.toHaveProperty("cron");
  });

  it("adds a project-targeted schedule from conversation details", async () => {
    const user = userEvent.setup();
    render(<App />);
    await screen.findByLabelText("Message the agent");
    await user.click(screen.getByRole("button", { name: /^Web project\// }));
    await user.click(screen.getAllByRole("button", { name: "Open conversation details" })[0]);
    const drawer = screen.getByRole("complementary", { name: "Conversation details" });
    await user.click(await within(drawer).findByRole("button", { name: /No scheduled prompts/ }));

    const dialog = screen.getByRole("dialog", { name: "Create scheduled prompt" });
    expect(within(dialog).getByLabelText("Target")).toHaveValue(`project:${projectFixture.id}`);
    expect(within(dialog).getByText("Runs in the selected conversation and keeps its context.")).toBeInTheDocument();
  });

  it("applies live pending and deletion events from the global schedule subscription", async () => {
    const user = userEvent.setup();
    mockState.schedules = [{ ...scheduledPromptFixture }];
    render(<App />);
    await user.click(await screen.findByRole("button", { name: "Scheduled prompts" }));
    let card = await screen.findByRole("article", { name: "Weekday review" });

    act(() => {
      for (const listener of mockState.eventListeners) listener({
        type: "scheduled_prompt_triggered",
        scope: { kind: "schedule", id: scheduledPromptFixture.id },
        timestamp: "2026-07-12T00:00:02Z",
        data: { ...scheduledPromptFixture, pending: true, last_status: "queued" },
      });
    });
    card = screen.getByRole("article", { name: "Weekday review" });
    expect(within(card).getByRole("button", { name: "Run Weekday review now" })).toBeDisabled();
    expect(within(card).getByRole("button", { name: "Edit Weekday review" })).toBeDisabled();
    expect(within(card).getByRole("button", { name: "Delete Weekday review" })).toBeDisabled();
    expect(within(card).getByRole("checkbox", { name: "Enable Weekday review" })).toBeEnabled();

    act(() => {
      for (const listener of mockState.eventListeners) listener({
        type: "scheduled_prompt_deleted",
        scope: { kind: "schedule", id: scheduledPromptFixture.id },
        timestamp: "2026-07-12T00:00:03Z",
        data: { ...scheduledPromptFixture, pending: false },
      });
    });
    expect(screen.queryByRole("article", { name: "Weekday review" })).not.toBeInTheDocument();
  });

  it("opens the result conversation when it is available", async () => {
    const user = userEvent.setup();
    mockState.schedules = [{
      ...scheduledPromptFixture,
      target_type: "one_off",
      conversation_id: undefined,
      last_conversation_id: chatFixture.id,
      last_run_at: "2026-07-12T00:00:02Z",
      last_status: "completed",
    }];
    render(<App />);
    await user.click(await screen.findByRole("button", { name: "Scheduled prompts" }));
    const card = await screen.findByRole("article", { name: "Weekday review" });
    await user.click(within(card).getByRole("button", { name: "Open result" }));
    expect(await screen.findByRole("heading", { name: "Web chat" })).toBeInTheDocument();
  });

  it("warns that deleting a conversation also deletes attached schedules", async () => {
    const user = userEvent.setup();
    mockState.schedules = [{ ...scheduledPromptFixture }];
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    render(<App />);
    await user.click(await screen.findByRole("button", { name: "Scheduled prompts" }));
    await screen.findByRole("article", { name: "Weekday review" });
    await user.click(screen.getByRole("button", { name: /^Web project\// }));
    await user.click(screen.getByRole("button", { name: "Delete Web project" }));
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining("1 attached scheduled prompt"));
  });
});
