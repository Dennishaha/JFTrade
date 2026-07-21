// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { MockWebSocket, mountApp } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockWebSocket.instances = [];
});

describe("App error handling", () => {
  it("shows a warning when the initial API request fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("API offline");
      }),
    );
    vi.stubGlobal(
      "WebSocket",
      MockWebSocket as unknown as typeof WebSocket,
    );

    const { wrapper } = await mountApp("/workspace");

    expect(wrapper.text()).toContain("API offline");

    wrapper.unmount();
  });
});
