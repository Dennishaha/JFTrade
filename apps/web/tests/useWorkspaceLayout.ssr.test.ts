// @vitest-environment node

import { describe, expect, it, vi } from "vitest";

import { provideWorkspaceLayoutStore } from "../src/composables/useWorkspaceLayout";

describe("workspace layout server rendering", () => {
  it("initializes the default workspace safely without browser storage", () => {
    // The store is normally provided from setup. Calling it directly here is
    // intentional: Node rendering has no browser storage, and Vue's provide
    // warning is irrelevant to the initialization contract under test.
    vi.spyOn(console, "warn").mockImplementation(() => {});
    const layout = provideWorkspaceLayoutStore();

    expect(layout.prefs.value).toMatchObject({
      market: "HK",
      symbol: "00700",
      period: "1m",
      rightDockOpen: false,
      watchlistSidebarOpen: true,
    });
    layout.update({ market: "US", symbol: "AAPL", period: "5m" });
    expect(layout.prefs.value).toMatchObject({
      market: "US",
      symbol: "AAPL",
      period: "5m",
    });
  });
});
