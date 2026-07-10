// @vitest-environment jsdom

import { afterEach, describe, expect, it } from "vitest";

import {
  buildRuntimeApiUrl,
  buildRuntimeLiveSocketUrl,
  resolveDesktopMode,
  resolveDesktopApiToken,
  resolveApiBaseUrl,
} from "../src/runtimeConfig";

afterEach(() => {
  delete window.__JFTRADE_RUNTIME_CONFIG__;
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
    expect(resolveDesktopApiToken()).toBeNull();
  });
});
