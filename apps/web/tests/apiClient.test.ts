// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchEnvelopeWithInit } from "../src/composables/apiClient";

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
});
