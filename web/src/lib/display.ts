import type { ModelInfo, Usage } from "./protocol";

export const thinkingLevels = [
  "none",
  "minimal",
  "low",
  "medium",
  "high",
  "xhigh",
  "max",
  "off",
] as const;

export const configurationThinkingLevels = [
  "none",
  "minimal",
  "low",
  "medium",
  "high",
  "max",
] as const;

export const firstPartyModels = [
  "gpt-5.6",
  "gpt-5.6-sol",
  "gpt-5.6-terra",
  "gpt-5.6-luna",
];

const tokenFormatter = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 });

export function formatTokens(value: number): string {
  return tokenFormatter.format(Math.max(0, Math.trunc(value)));
}

export function formatContext(used: number, window: number): string {
  if (window <= 0) return `${formatTokens(used)} used`;
  const percent = Math.min(100, (used / window) * 100);
  const percentText = `${percent > 0 && percent < 10 ? percent.toFixed(1) : Math.round(percent)}%`;
  return `${formatTokens(used)} / ${formatTokens(window)} (${percentText})`;
}

export function relativeTime(value: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return "recently";
  const seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000));
  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86_400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86_400)}d ago`;
}

export function compactEndpoint(value: string): string {
  return value.replace(/^wss?:\/\//, "").replace(/\/ws$/, "");
}

export function compactPath(value = ""): string {
  const normalized = value.replace(/\/+$/, "") || "/";
  if (normalized.length <= 31) return normalized;
  const parts = normalized.split("/").filter(Boolean);
  return parts.length > 1 ? `…/${parts.slice(-2).join("/")}` : normalized;
}

export function shortID(value: string): string {
  return value.length > 22 ? `${value.slice(0, 11)}…${value.slice(-7)}` : value;
}

export function summarizeArguments(value: unknown): string {
  if (value == null) return "Waiting for output";
  const text = typeof value === "string" ? value : JSON.stringify(value);
  return text.length > 100 ? `${text.slice(0, 100)}…` : text;
}

export function arraysEqual(left: string[], right: string[]): boolean {
  if (left.length !== right.length) return false;
  const sorted = [...right].sort();
  return [...left].sort().every((value, index) => value === sorted[index]);
}

export function mergeModelOptions(models: ModelInfo[], selected?: string): ModelInfo[] {
  const options = new Map<string, ModelInfo>();
  const openRouterActive = models.some((model) => model.provider?.toLowerCase() === "openrouter");
  if (!openRouterActive) {
    for (const id of firstPartyModels) options.set(id, { id, provider: "OpenAI" });
  }
  for (const model of models) {
    if (model?.id) options.set(model.id, { ...options.get(model.id), ...model });
  }
  if (selected && !options.has(selected)) options.set(selected, { id: selected, provider: "current" });
  return [...options.values()];
}

export function usageContextWindow(usage: Usage, models: ModelInfo[], model: string): number {
  return usage.context_window || models.find((item) => item.id === model)?.context_window || 0;
}
