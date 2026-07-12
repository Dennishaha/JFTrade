// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  ApiClientError,
  WEB_AUTH_REQUIRED_EVENT,
  fetchEnvelopeWithInit,
  webLogin,
  webLogout,
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

  it("logs browser users in with a Web password payload", async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            ok: true,
            data: { authenticated: true, csrfToken: "csrf-token" },
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await webLogin("browser-password");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/auth/login",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        body: JSON.stringify({ password: "browser-password" }),
      }),
    );
  });

  it("logs a browser session out and asks the app to show the login gate", async () => {
    const listener = vi.fn();
    window.addEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
    const fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({ ok: true, data: { authenticated: false } }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await webLogout();

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/auth/logout",
      expect.objectContaining({ method: "POST", credentials: "include" }),
    );
    expect(listener).toHaveBeenCalledTimes(1);
    window.removeEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
  });

  it("notifies the app when a Web session expires", async () => {
    const listener = vi.fn();
    window.addEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              ok: false,
              error: {
                code: "WEB_AUTH_REQUIRED",
                message: "Web authentication is required",
              },
              timestamp: "2026-07-11T00:00:00Z",
            }),
            {
              status: 401,
              headers: { "Content-Type": "application/json" },
            },
          ),
      ),
    );

    await expect(
      fetchEnvelopeWithInit("/api/v1/system/status", { method: "GET" }),
    ).rejects.toMatchObject({ code: "WEB_AUTH_REQUIRED", status: 401 });
    expect(listener).toHaveBeenCalledTimes(1);

    window.removeEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
  });
});
