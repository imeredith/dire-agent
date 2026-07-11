const storagePrefix = "dire-agent.";
const legacyStoragePrefix = "goagent.";

export function readAppStorage(name: string): string | null {
  const key = storagePrefix + name;
  const value = localStorage.getItem(key);
  if (value !== null) return value;

  const legacyValue = localStorage.getItem(legacyStoragePrefix + name);
  if (legacyValue !== null) localStorage.setItem(key, legacyValue);
  return legacyValue;
}

export function writeAppStorage(name: string, value: string): void {
  localStorage.setItem(storagePrefix + name, value);
}

export function removeAppStorage(name: string): void {
  localStorage.removeItem(storagePrefix + name);
  localStorage.removeItem(legacyStoragePrefix + name);
}
