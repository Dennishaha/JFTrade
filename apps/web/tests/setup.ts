import { vi } from "vitest";

function installStorage(name: "localStorage" | "sessionStorage"): void {
  const values = new Map<string, string>();
  const storage: Storage = {
    get length() {
      return values.size;
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(String(key)) ?? null,
    key: (index) => Array.from(values.keys())[index] ?? null,
    removeItem: (key) => values.delete(String(key)),
    setItem: (key, value) => values.set(String(key), String(value)),
  };

  Object.defineProperty(window, name, {
    configurable: true,
    value: storage,
  });
}

if (typeof window !== "undefined") {
  installStorage("localStorage");
  installStorage("sessionStorage");
  vi.stubGlobal("localStorage", window.localStorage);
  vi.stubGlobal("sessionStorage", window.sessionStorage);
}
