// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  ApiClientError,
  WEB_AUTH_REQUIRED_EVENT,
  apiDeleteBody,
  apiDeletePath,
  apiGet,
  apiGetPath,
  apiPostPathAction,
  apiRawRequest,
  setCSRFToken,
  webLogin,
  webLogout,
  webSession,
} from "@/composables/shared/apiClient";

afterEach(() => {
  setCSRFToken("");
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
      apiGet("/api/v1/system/status"),
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

    await apiGet("/api/v1/system/status");

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
      apiPostPathAction(
        "/api/v1/strategies/{instanceId}/start",
        "/api/v1/strategies/instance-a/start",
      ),
    ).rejects.toMatchObject({
      name: "ApiClientError",
      code: "BAD_REQUEST",
      status: 400,
      message: "运行实例 PineTS Worker 已达到上限",
    } satisfies Partial<ApiClientError>);
  });

  it("exposes Retry-After on rate-limited API errors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(
        JSON.stringify({
          ok: false,
          error: {
            code: "MARKET_SNAPSHOT_RATE_LIMITED",
            message: "行情快照请求过于频繁",
          },
          timestamp: "2026-07-18T00:00:00Z",
        }),
        {
          status: 429,
          headers: {
            "Content-Type": "application/json",
            "Retry-After": "4.25",
          },
        },
      )),
    );

    await expect(
      apiGetPath(
        "/api/v1/market-data/snapshots/{market}/{code}",
        "/api/v1/market-data/snapshots/US/AAPL",
      ),
    ).rejects.toMatchObject({
      name: "ApiClientError",
      code: "MARKET_SNAPSHOT_RATE_LIMITED",
      status: 429,
      retryAfterMs: 4_250,
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
      apiGet("/api/v1/system/status"),
    ).rejects.toMatchObject({ code: "WEB_AUTH_REQUIRED", status: 401 });
    expect(listener).toHaveBeenCalledTimes(1);

    window.removeEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
  });

  it("distinguishes malformed successful responses from an empty response body", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response("not-json", {
        status: 200,
        statusText: "OK",
      }))
      .mockResolvedValueOnce(new Response("", {
        status: 200,
        statusText: "OK",
      }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(apiGet("/api/v1/system/status")).rejects.toThrow(
      "API response is not valid JSON",
    );
    await expect(apiGet("/api/v1/system/status")).rejects.toThrow(
      "API response body is empty",
    );
  });

  it("keeps generic HTTP failures meaningful when the server sends no API envelope", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("", {
        status: 503,
        statusText: "Service Unavailable",
      })),
    );

    await expect(apiGet("/api/v1/system/status")).rejects.toThrow(
      "503 Service Unavailable",
    );
  });

  it("honors failed API envelopes even when an intermediary returns HTTP 200", async () => {
    const listener = vi.fn();
    window.addEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(
        JSON.stringify({
          ok: false,
          error: {
            code: "WEB_ACCESS_DISABLED",
            message: "Remote Web access is disabled",
          },
          timestamp: "2026-07-16T00:00:00Z",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      )),
    );

    await expect(apiGet("/api/v1/system/status")).rejects.toMatchObject({
      name: "ApiClientError",
      code: "WEB_ACCESS_DISABLED",
      status: 200,
    } satisfies Partial<ApiClientError>);
    expect(listener).toHaveBeenCalledTimes(1);
    window.removeEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
  });

  it("uses the common authenticated pipeline for DELETE requests", async () => {
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({ ok: true, data: { deleted: true } }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      apiDeletePath(
        "/api/v1/adk/sessions/{sessionId}",
        "/api/v1/adk/sessions/session-1",
      ),
    ).resolves.toEqual({ deleted: true });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/adk/sessions/session-1",
      expect.objectContaining({ method: "DELETE", credentials: "include" }),
    );
  });

  it("serializes typed DELETE request bodies through the common pipeline", async () => {
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({ ok: true, data: { realTradingEnabled: false } }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      apiDeleteBody("/api/v1/system/real-trade-risk-limits", {
        operatorId: "risk-operator",
        reason: "end of trading session",
      }),
    ).resolves.toEqual({ realTradingEnabled: false });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/system/real-trade-risk-limits",
      expect.objectContaining({
        method: "DELETE",
        credentials: "include",
        body: JSON.stringify({
          operatorId: "risk-operator",
          reason: "end of trading session",
        }),
      }),
    );
  });

  it("uses the authenticated raw boundary for SSE and reports expired sessions", async () => {
    setCSRFToken("csrf-stream");
    const listener = vi.fn();
    window.addEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
    const fetchMock = vi.fn(async () => new Response(
      JSON.stringify({
        ok: false,
        error: {
          code: "WEB_AUTH_REQUIRED",
          message: "Web authentication required",
        },
        timestamp: "2026-07-27T00:00:00Z",
      }),
      { status: 401, headers: { "Content-Type": "application/json" } },
    ));
    vi.stubGlobal("fetch", fetchMock);

    const response = await apiRawRequest("/api/v1/adk/chat/stream", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: "hello" }),
    });

    expect(response.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/adk/chat/stream",
      expect.objectContaining({
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": "csrf-stream",
        },
      }),
    );
    expect(listener).toHaveBeenCalledOnce();
    window.removeEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
  });

  it("does not emit authentication expiry for healthy, unrelated, or desktop raw responses", async () => {
    const listener = vi.fn();
    window.addEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response("", { status: 200 }))
      .mockResolvedValueOnce(new Response("", { status: 500 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        ok: false,
        error: { code: "CSRF_FAILED", message: "CSRF token rejected" },
        timestamp: "2026-07-27T00:00:00Z",
      }), { status: 403, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        ok: false,
        error: { code: "ORIGIN_FORBIDDEN", message: "Origin rejected" },
        timestamp: "2026-07-27T00:00:00Z",
      }), { status: 403, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response("", { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        ok: false,
        error: { code: "WEB_AUTH_REQUIRED", message: "Web authentication required" },
        timestamp: "2026-07-27T00:00:00Z",
      }), { status: 401, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ ok: true, data: { authenticated: true } }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ));
    vi.stubGlobal("fetch", fetchMock);

    await apiRawRequest("/api/v1/adk/chat/stream");
    await apiRawRequest("/api/v1/adk/chat/stream");
    await apiRawRequest("/api/v1/adk/chat/stream");
    await apiRawRequest("/api/v1/adk/chat/stream");
    await apiRawRequest("/api/v1/adk/chat/stream");
    window.__JFTRADE_RUNTIME_CONFIG__ = { desktopApiToken: "desktop-token" };
    await apiRawRequest("/api/v1/adk/chat/stream");
    await expect(webSession()).resolves.toEqual({ authenticated: true });

    expect(listener).not.toHaveBeenCalled();
    window.removeEventListener(WEB_AUTH_REQUIRED_EVENT, listener);
  });
});
