// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  buildRuntimeApiUrl,
  buildRuntimeLiveSocketUrl,
  resolveApiBaseUrl,
  resolveAuthRequired,
  resolveDesktopBridgeAvailable,
  resolveDesktopApiToken,
  resolveDesktopMode,
} from "../../src/runtimeConfig";

afterEach(() => {
  delete window.__JFTRADE_RUNTIME_CONFIG__;
  delete (window as typeof window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__;
});

describe("runtimeConfig", () => {
  it("falls back to the Vite proxy path when no runtime override exists", () => {
    expect(resolveApiBaseUrl()).toBe("");
    expect(buildRuntimeApiUrl("/api/v1/system/status")).toBe(
      "/api/v1/system/status",
    );
  });

  it("prefers the runtime-injected API address for release GUI requests", () => {
    window.__JFTRADE_RUNTIME_CONFIG__ = {
      apiBaseUrl: "http://127.0.0.1:6699/",
      desktopMode: true,
      desktopApiToken: "release-token",
    };

    expect(resolveApiBaseUrl()).toBe("http://127.0.0.1:6699");
    expect(resolveDesktopMode()).toBe(true);
    expect(resolveDesktopApiToken()).toBe("release-token");
    expect(buildRuntimeApiUrl("/api/v1/system/status")).toBe(
      "http://127.0.0.1:6699/api/v1/system/status",
    );
    expect(buildRuntimeLiveSocketUrl("/api/v1/ws/live")).toBe(
      "ws://127.0.0.1:6699/api/v1/ws/live",
    );
  });

  it("treats missing desktop mode as web mode", () => {
    expect(resolveDesktopMode()).toBe(false);
    expect(resolveDesktopBridgeAvailable()).toBe(false);
    expect(resolveDesktopApiToken()).toBeNull();
  });

  it("detects the native bridge independently of desktop runtime config", () => {
    (window as typeof window & { __TAURI_INTERNALS__?: { invoke: () => void } }).__TAURI_INTERNALS__ = {
      invoke: () => undefined,
    };

    expect(resolveDesktopBridgeAvailable()).toBe(true);
  });

  it("keeps the injected authRequired flag compatible for Web clients", () => {
    window.__JFTRADE_RUNTIME_CONFIG__ = { authRequired: true };
    expect(resolveAuthRequired()).toBe(true);

    window.__JFTRADE_RUNTIME_CONFIG__ = { authRequired: false };
    expect(resolveAuthRequired()).toBe(false);
  });

  it("keeps desktop-only settings disabled during server-side rendering", async () => {
    vi.resetModules();
    vi.stubGlobal("window", undefined);
    try {
      const serverConfig = await import("../../src/runtimeConfig");
      expect(serverConfig.resolveDesktopMode()).toBe(false);
      expect(serverConfig.resolveDesktopBridgeAvailable()).toBe(false);
      expect(serverConfig.resolveDesktopApiToken()).toBeNull();
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
