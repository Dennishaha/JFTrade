// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiClientError, fetchEnvelopeWithInit } from "../src/composables/apiClient";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("apiClient", () => {
  it("surfaces non-JSON error responses without throwing a JSON parse error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
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

  it("preserves API error code and status for UI-specific handling", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
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
