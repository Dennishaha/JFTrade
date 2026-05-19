// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { MockEventSource, mountApp } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
  MockEventSource.instances = [];
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
      "EventSource",
      MockEventSource as unknown as typeof EventSource,
    );

    const { wrapper } = await mountApp("/system");

    expect(wrapper.text()).toContain("API offline");

    wrapper.unmount();
  });
});
