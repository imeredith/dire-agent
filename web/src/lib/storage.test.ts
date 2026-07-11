import { beforeEach, describe, expect, it } from "vitest";
import { readAppStorage, removeAppStorage, writeAppStorage } from "./storage";

describe("app storage namespace", () => {
  beforeEach(() => localStorage.clear());

  it("migrates a legacy value when the Dire Agent key is absent", () => {
    localStorage.setItem("goagent.endpoint", "ws://legacy.test/ws");

    expect(readAppStorage("endpoint")).toBe("ws://legacy.test/ws");
    expect(localStorage.getItem("dire-agent.endpoint")).toBe("ws://legacy.test/ws");
  });

  it("prefers the Dire Agent value", () => {
    localStorage.setItem("goagent.endpoint", "ws://legacy.test/ws");
    localStorage.setItem("dire-agent.endpoint", "ws://current.test/ws");

    expect(readAppStorage("endpoint")).toBe("ws://current.test/ws");
  });

  it("writes the current namespace and removes both namespaces", () => {
    localStorage.setItem("goagent.project.category", "Legacy");
    writeAppStorage("project.category", "Current");
    expect(localStorage.getItem("dire-agent.project.category")).toBe("Current");

    removeAppStorage("project.category");
    expect(readAppStorage("project.category")).toBeNull();
  });
});
