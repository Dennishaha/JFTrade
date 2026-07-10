// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  ApiClientError,
  fetchEnvelopeWithInit,
} from "../src/composables/apiClient";

afterEach(() => {
  vi.unstubAllGlobals();
  delete window.__JFTRADE_RUNTIME_CONFIG__;
});

describe("apiClient", () => {
  it("surfaces non-JSON error responses without throwing a JSON parse error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response("The server returned 404 Not Found", {
            status: 404,
            statusText: "Not Found",
            headers: {
              "Content-Type": "text/plain",
            },
          }),
      ),
    );

    await expect(
      fetchEnvelopeWithInit("/api/v1/settings/brokers/futu/integration", {
        method: "PUT",
      }),
    ).rejects.toThrow("404 Not Found: The server returned 404 Not Found");
  });

  it("adds the packaged desktop bearer token without changing the request URL", async () => {
    window.__JFTRADE_RUNTIME_CONFIG__ = {
      apiBaseUrl: "http://127.0.0.1:6699",
      desktopApiToken: "desktop-token",
    };
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify({ ok: true, data: { ready: true } }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await fetchEnvelopeWithInit("/api/v1/system/status", { method: "GET" });

    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:6699/api/v1/system/status",
      expect.objectContaining({
        headers: { Authorization: "Bearer desktop-token" },
      }),
    );
  });

  it("preserves API error code and status for UI-specific handling", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              ok: false,
              error: {
                code: "BAD_REQUEST",
                message: "运行实例 PineTS Worker 已达到上限",
              },
              timestamp: "2026-06-29T00:00:00Z",
            }),
            {
              status: 400,
              headers: {
                "Content-Type": "application/json",
              },
            },
          ),
      ),
    );

    await expect(
      fetchEnvelopeWithInit("/api/v1/strategies/instance-a/start", {
        method: "POST",
      }),
    ).rejects.toMatchObject({
      name: "ApiClientError",
      code: "BAD_REQUEST",
      status: 400,
      message: "运行实例 PineTS Worker 已达到上限",
    } satisfies Partial<ApiClientError>);
  });
});
