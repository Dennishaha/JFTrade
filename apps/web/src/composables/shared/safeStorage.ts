export function readLocalStorage(key: string): string | null {
  try {
    return globalThis.window?.localStorage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

export function writeLocalStorage(key: string, value: string): void {
  try {
    globalThis.window?.localStorage?.setItem(key, value);
  } catch {
    // Storage may be disabled by browser policy or an opaque document origin.
  }
}
