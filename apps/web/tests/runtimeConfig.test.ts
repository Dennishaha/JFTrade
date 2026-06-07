// @vitest-environment jsdom

import { afterEach, describe, expect, it } from "vitest";

import { buildRuntimeApiUrl, resolveApiBaseUrl } from "../src/runtimeConfig";

afterEach(() => {
  delete window.__JFTRADE_RUNTIME_CONFIG__;
});

describe("runtimeConfig", () => {
  it("falls back to the development API address when no runtime override exists", () => {
    const hostname = window.location.hostname || "127.0.0.1";
    expect(resolveApiBaseUrl()).toBe(`http://${hostname}:3000`);
    expect(buildRuntimeApiUrl("/api/v1/system/status")).toBe(
      `http://${hostname}:3000/api/v1/system/status`,
    );
  });

  it("prefers the runtime-injected API address for release GUI requests", () => {
    window.__JFTRADE_RUNTIME_CONFIG__ = {
      apiBaseUrl: "http://127.0.0.1:6699/",
    };

    expect(resolveApiBaseUrl()).toBe("http://127.0.0.1:6699");
    expect(buildRuntimeApiUrl("/api/v1/system/status")).toBe(
      "http://127.0.0.1:6699/api/v1/system/status",
    );
  });
});
