import type { ProjectLauncher } from "./configuration";

/** Defaults used when loading a pre-launcher v1 configuration document. */
export const defaultProjectLaunchers: ProjectLauncher[] = [
  { id: "shell", label: "Terminal", kind: "terminal", shortcut: "mod+backquote" },
  { id: "lazygit", label: "lazygit", kind: "terminal", command: "lazygit", shortcut: "mod+shift+g" },
  { id: "nvim", label: "nvim", kind: "terminal", command: "nvim", args: ["."], shortcut: "mod+shift+e" },
];

export function effectiveProjectLaunchers(value: ProjectLauncher[] | null | undefined): ProjectLauncher[] {
  return value == null ? defaultProjectLaunchers.map((launcher) => ({
    ...launcher,
    args: launcher.args ? [...launcher.args] : undefined,
  })) : value;
}

export const terminalPrimaryFont = '"Cascadia Code"';
export const terminalIconFont = '"Dire Agent Nerd Symbols"';
export const terminalIconProbe = "\ue0a0\ue0b0\uf013\uf121\u{f0001}";
export const terminalFontFamily = `${terminalPrimaryFont}, ${terminalIconFont}, "CaskaydiaCove Nerd Font Mono", "Symbols Nerd Font Mono", "SFMono-Regular", Menlo, Monaco, Consolas, monospace`;

export const terminalLigatureFeatures = '"calt" on, "liga" on';

// The browser cannot inspect a cross-origin Google Font file, so the ligature
// add-on needs explicit character ranges. Cascadia Code performs the actual
// glyph substitution; these ranges tell xterm which adjacent cells to join.
export const terminalFallbackLigatures: string[] = [
  "<---->", "<===>", "<--->", "<!---", "!==", "===", "==>", "<==", "-->", "<--",
  "<=>", "<->", "->>", "<<-", "=>>", ">>=", "->", "<-", "=>", "<=", ">=", "==",
  "!=", "/=", "~=", "::", ":=", "&&", "||", "/*", "*/", "<!--", "</>", "~~>",
];

// Tokyo Night Moon gives 16-color applications a balanced dark palette. Apps
// such as Neovim that emit 24-bit colors continue to supply their own theme.
export const terminalTheme = {
  background: "#222436",
  foreground: "#c8d3f5",
  cursor: "#c8d3f5",
  cursorAccent: "#222436",
  selectionBackground: "#2d3f76",
  selectionInactiveBackground: "#2f334d",
  black: "#1b1d2b",
  red: "#ff757f",
  green: "#c3e88d",
  yellow: "#ffc777",
  blue: "#82aaff",
  magenta: "#c099ff",
  cyan: "#86e1fc",
  white: "#c8d3f5",
  brightBlack: "#828bb8",
  brightRed: "#ff966c",
  brightGreen: "#c3e88d",
  brightYellow: "#ffc777",
  brightBlue: "#82aaff",
  brightMagenta: "#c099ff",
  brightCyan: "#86e1fc",
  brightWhite: "#ffffff",
} as const;

export function terminalWebSocketURL(endpoint: string, projectID: string, launcherID: string): string {
  const url = new URL(endpoint);
  url.pathname = "/terminal";
  url.search = "";
  url.hash = "";
  url.searchParams.set("project_id", projectID);
  url.searchParams.set("launcher_id", launcherID);
  return url.toString();
}

const keyAliases: Record<string, string> = {
  "`": "backquote",
  grave: "backquote",
  ctrl: "control",
  cmd: "meta",
  command: "meta",
  option: "alt",
};

function shortcutParts(shortcut: string): string[] {
  return shortcut.toLowerCase().split("+").map((part) => keyAliases[part.trim()] || part.trim()).filter(Boolean);
}

export function matchesLauncherShortcut(event: KeyboardEvent, shortcut: string | undefined): boolean {
  if (!shortcut) return false;
  const parts = shortcutParts(shortcut);
  const needsMod = parts.includes("mod");
  const needsMeta = parts.includes("meta");
  const needsControl = parts.includes("control");
  const needsShift = parts.includes("shift");
  const needsAlt = parts.includes("alt");
  if (needsMod ? !(event.metaKey || event.ctrlKey) : event.metaKey !== needsMeta || event.ctrlKey !== needsControl) return false;
  if (event.shiftKey !== needsShift || event.altKey !== needsAlt) return false;
  const key = parts.find((part) => !["mod", "meta", "control", "shift", "alt"].includes(part));
  if (!key) return false;
  if (key === "backquote") return event.code === "Backquote" || event.key === "`";
  return event.key.toLowerCase() === key;
}

export function formatLauncherShortcut(shortcut: string | undefined): string {
  if (!shortcut) return "";
  return shortcutParts(shortcut).map((part) => ({
    mod: "⌘/Ctrl",
    meta: "⌘",
    control: "Ctrl",
    shift: "Shift",
    alt: "Alt",
    backquote: "`",
  })[part] || part.toUpperCase()).join(" + ");
}
