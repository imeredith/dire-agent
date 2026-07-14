import { describe, expect, it } from "vitest";
import { mergeModelOptions } from "./display";

describe("mergeModelOptions", () => {
  it("keeps the Codex fallback catalog for older daemons", () => {
    const models = mergeModelOptions([{ provider: "test", id: "server-model" }]);
    expect(models.map((model) => model.id)).toContain("gpt-5.6-luna");
    expect(models.map((model) => model.id)).toContain("server-model");
  });

  it("does not mix Codex model IDs into an OpenRouter catalog", () => {
    const models = mergeModelOptions([
      { provider: "openrouter", id: "openrouter/auto", context_window: 2_000_000 },
    ]);
    expect(models).toEqual([
      { provider: "openrouter", id: "openrouter/auto", context_window: 2_000_000 },
    ]);
  });
});
