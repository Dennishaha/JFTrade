// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

const storageKey = "jftrade.market-data.consumer.market-page";

afterEach(() => {
  vi.resetModules();
  vi.unstubAllGlobals();
  window.sessionStorage?.clear();
});

describe("createStableWebConsumerId", () => {
  it("keeps cloned sessionStorage bases but separates browser windows", async () => {
    window.sessionStorage.setItem(storageKey, "web:market-page:cloned");

    vi.stubGlobal("crypto", {
      randomUUID: vi.fn(() => "window-a"),
    });
    const firstModule = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const firstId = firstModule.createStableWebConsumerId("market-page");

    vi.resetModules();
    vi.stubGlobal("crypto", {
      randomUUID: vi.fn(() => "window-b"),
    });
    const secondModule = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );
    const secondId = secondModule.createStableWebConsumerId("market-page");

    expect(firstId).toBe("web:market-page:cloned:window:window-a");
    expect(secondId).toBe("web:market-page:cloned:window:window-b");
    expect(firstId).not.toBe(secondId);
    expect(window.sessionStorage.getItem(storageKey)).toBe(
      "web:market-page:cloned",
    );
  });

  it("returns a stable id within the same page instance", async () => {
    vi.stubGlobal("crypto", {
      randomUUID: vi
        .fn()
        .mockReturnValueOnce("window-a")
        .mockReturnValueOnce("base-a"),
    });
    const module = await import(
      "../src/composables/consoleDataMarketSubscriptions"
    );

    expect(module.createStableWebConsumerId("market-page")).toBe(
      "web:market-page:base-a:window:window-a",
    );
    expect(module.createStableWebConsumerId("market-page")).toBe(
      "web:market-page:base-a:window:window-a",
    );
  });
});
