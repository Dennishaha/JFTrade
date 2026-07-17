import { createMemoryHistory } from "vue-router";
import { describe, expect, it, vi } from "vitest";

vi.mock("../src/pages/WorkspacePage.vue", () => ({ default: { name: "WorkspacePage" } }));
vi.mock("../src/pages/WatchlistPage.vue", () => ({ default: { name: "WatchlistPage" } }));
vi.mock("../src/pages/SystemPage.vue", () => ({ default: { name: "SystemPage" } }));
vi.mock("../src/pages/SettingsPage.vue", () => ({ default: { name: "SettingsPage" } }));
vi.mock("../src/pages/AccountPage.vue", () => ({ default: { name: "AccountPage" } }));
vi.mock("../src/pages/RiskPage.vue", () => ({ default: { name: "RiskPage" } }));
vi.mock("../src/pages/StrategyRuntimePage.vue", () => ({ default: { name: "StrategyRuntimePage" } }));
vi.mock("../src/pages/StrategyDesignPage.vue", () => ({ default: { name: "StrategyDesignPage" } }));
vi.mock("../src/pages/ADKPage.vue", () => ({ default: { name: "ADKPage" } }));
vi.mock("../src/pages/BacktestPage.vue", () => ({ default: { name: "BacktestPage" } }));
vi.mock("../src/pages/DesktopLogsPage.vue", () => ({ default: { name: "DesktopLogsPage" } }));

import { createConsoleRouter } from "../src/router";

describe("console router", () => {
  it("preserves product routes, redirects, route metadata, and lazy page loaders", async () => {
    const router = createConsoleRouter(createMemoryHistory());

    expect(router.resolve("/").matched.at(-1)?.redirect).toBe("/workspace");
    expect(router.resolve("/workspace").meta.title).toBe("交易");
    expect(router.resolve("/settings/security").meta.title).toBe("设置");
    expect(router.resolve("/desktop-logs").meta).toMatchObject({
      title: "桌面日志",
      standalone: true,
    });
    expect(router.resolve("/broker").matched.at(-1)?.redirect).toBe("/account");
    expect(router.resolve("/strategy").matched.at(-1)?.redirect).toBe("/strategy/runtime");

    const routes = router.getRoutes().filter((route) => typeof route.components?.default === "function");
    const loaded = await Promise.all(
      routes.map((route) => (route.components?.default as () => Promise<{ default: { name: string } }>)()),
    );
    expect(loaded.map((entry) => entry.default.name)).toEqual(
      expect.arrayContaining([
        "WorkspacePage",
        "WatchlistPage",
        "SystemPage",
        "SettingsPage",
        "AccountPage",
        "RiskPage",
        "StrategyRuntimePage",
        "StrategyDesignPage",
        "ADKPage",
        "BacktestPage",
        "DesktopLogsPage",
      ]),
    );
  });
});
