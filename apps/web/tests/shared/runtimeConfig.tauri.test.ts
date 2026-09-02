// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

const invoke = vi.hoisted(() => vi.fn());

vi.mock("@tauri-apps/api/core", () => ({ invoke }));

import {
  initializeTauriRuntimeConfig,
  resolveApiBaseUrl,
  resolveDesktopApiToken,
  resolveDesktopMode,
} from "@/runtimeConfig";

afterEach(() => {
  delete window.__JFTRADE_RUNTIME_CONFIG__;
  delete (window as typeof window & { __TAURI_INTERNALS__?: unknown })
    .__TAURI_INTERNALS__;
  invoke.mockReset();
});

describe("Tauri runtime configuration", () => {
  it("loads the authenticated loopback API before the Vue application mounts", async () => {
    (window as typeof window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {};
    invoke.mockResolvedValue({
      apiBaseUrl: "http://127.0.0.1:3008/",
      authRequired: true,
      desktopMode: true,
      desktopApiToken: "a".repeat(64),
    });

    await initializeTauriRuntimeConfig();

    expect(invoke).toHaveBeenCalledWith("desktop_runtime_config");
    expect(resolveApiBaseUrl()).toBe("http://127.0.0.1:3008");
    expect(resolveDesktopApiToken()).toBe("a".repeat(64));
    expect(resolveDesktopMode()).toBe(true);
    expect(window.__JFTRADE_RUNTIME_CONFIG__?.authRequired).toBe(true);
  });

  it.each([
    "",
    "https://127.0.0.1:3008",
    "http://localhost:3008",
    "http://example.com:3008",
    "http://127.0.0.1:3008/path",
    "http://[",
  ])("rejects unsafe API base URL %s", async (apiBaseUrl) => {
    (window as typeof window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {};
    invoke.mockResolvedValue({
      apiBaseUrl,
      authRequired: true,
      desktopMode: true,
      desktopApiToken: "a".repeat(64),
    });

    await expect(initializeTauriRuntimeConfig()).rejects.toThrow(
      "unsafe desktop API configuration",
    );
    expect(window.__JFTRADE_RUNTIME_CONFIG__).toBeUndefined();
  });

  it("retries while the desktop API is still starting", async () => {
    (window as typeof window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {};
    invoke
      .mockRejectedValueOnce(new Error("DESKTOP_NOT_READY"))
      .mockResolvedValueOnce({
        apiBaseUrl: "http://127.0.0.1:3008",
        authRequired: true,
        desktopMode: true,
        desktopApiToken: "c".repeat(64),
      });

    await initializeTauriRuntimeConfig();

    expect(invoke).toHaveBeenCalledTimes(2);
    expect(resolveDesktopApiToken()).toBe("c".repeat(64));
  });

  it("does not invoke Tauri from a browser without the native bridge", async () => {
    await initializeTauriRuntimeConfig();
    window.__JFTRADE_RUNTIME_CONFIG__ = {
      apiBaseUrl: "http://127.0.0.1:3008",
      authRequired: false,
      desktopMode: true,
    };
    await initializeTauriRuntimeConfig();
    expect(invoke).not.toHaveBeenCalled();
  });

  it("refreshes a tokenless preloaded desktop config through Tauri IPC", async () => {
    window.__JFTRADE_RUNTIME_CONFIG__ = {
      apiBaseUrl: "http://127.0.0.1:3008",
      authRequired: false,
      desktopMode: true,
    };
    (window as typeof window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {};
    invoke.mockResolvedValue({
      apiBaseUrl: "http://127.0.0.1:3008/",
      authRequired: false,
      desktopMode: true,
      desktopApiToken: "b".repeat(64),
    });

    await initializeTauriRuntimeConfig();

    expect(invoke).toHaveBeenCalledWith("desktop_runtime_config");
    expect(resolveDesktopApiToken()).toBe("b".repeat(64));
  });
});
