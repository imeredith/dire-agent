import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { DesignLabApp } from "./DesignLabApp";

describe("DesignLabApp", () => {
  beforeEach(() => window.history.replaceState({}, "", "/designs"));
  afterEach(() => cleanup());

  it("presents all ten directions and opens an interactive concept", async () => {
    const user = userEvent.setup();
    render(<DesignLabApp />);

    expect(screen.getByRole("heading", { name: "Ten ways to make parallel agent work feel legible." })).toBeInTheDocument();
    expect(screen.getByText("6 library-backed")).toBeInTheDocument();
    expect(screen.getAllByRole("heading", { level: 2 })).toHaveLength(10);

    const commandHeading = screen.getByRole("heading", { name: "Command Center", level: 2 });
    const commandCard = commandHeading.closest("button");
    expect(commandCard).not.toBeNull();
    await user.click(commandCard!);

    expect(window.location.pathname).toBe("/designs/command-center");
    expect(await screen.findByRole("heading", { name: "Command Center", level: 1 })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Desktop viewport" })).toHaveAttribute("aria-pressed", "true");

    await user.click(screen.getByRole("button", { name: "Next concept" }));
    expect(window.location.pathname).toBe("/designs/operations-cockpit");
    expect(await screen.findByRole("heading", { name: "Operations Cockpit", level: 1 })).toBeInTheDocument();
  });
});
