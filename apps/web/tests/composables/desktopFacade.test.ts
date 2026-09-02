// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  invoke: vi.fn(),
  listen: vi.fn(),
}));

vi.mock("@tauri-apps/api/core", () => ({ invoke: mocks.invoke }));
vi.mock("@tauri-apps/api/event", () => ({ listen: mocks.listen }));

import {
  desktopFacade,
  resolveDesktopBackend,
} from "@/composables/shared/desktopFacade";

afterEach(() => {
  delete window.__JFTRADE_RUNTIME_CONFIG__;
  delete (window as typeof window & { __TAURI_INTERNALS__?: unknown })
    .__TAURI_INTERNALS__;
  mocks.invoke.mockReset();
  mocks.listen.mockReset();
});

describe("desktopFacade", () => {
  it("resolves server, Tauri, and browser runtimes", () => {
    const browserWindow = window;

    vi.stubGlobal("window", undefined);
    expect(resolveDesktopBackend()).toBe("browser");

    vi.stubGlobal("window", {
      __JFTRADE_RUNTIME_CONFIG__: { desktopMode: true },
    });
    expect(resolveDesktopBackend()).toBe("tauri");

    vi.stubGlobal("window", {
      __JFTRADE_RUNTIME_CONFIG__: { desktopMode: false },
      __TAURI_INTERNALS__: {},
    });
    expect(resolveDesktopBackend()).toBe("tauri");

    vi.stubGlobal("window", browserWindow);
    expect(resolveDesktopBackend()).toBe("browser");
    vi.unstubAllGlobals();
  });

  it("uses every exact Tauri command and event contract when Tauri is present", async () => {
    (window as typeof window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {};
    mocks.invoke.mockImplementation(async (command: string) => ({ command }));
    const unlisten = vi.fn();
    mocks.listen.mockImplementation(
      async (eventName: string, listener: (event: { payload: unknown }) => void) => {
        listener({ payload: { eventName } });
        return unlisten;
      },
    );

    expect(resolveDesktopBackend()).toBe("tauri");
    expect(desktopFacade.backend()).toBe("tauri");
    await desktopFacade.startup.snapshot();
    await desktopFacade.startup.quit();
    await desktopFacade.links.open("/docs/");
    await desktopFacade.logs.listDays();
    await desktopFacade.logs.readPage("2026-08-19", "WARN", "timeout", 20, 50);
    await desktopFacade.logs.openFolder();
    await desktopFacade.updates.check();
    await desktopFacade.updates.install();
    await desktopFacade.windows.showMain();
    await desktopFacade.windows.hideMain();
    await desktopFacade.windows.openLogs();

    expect(mocks.invoke.mock.calls).toEqual([
      ["desktop_startup_snapshot", undefined],
      ["desktop_startup_quit", undefined],
      ["desktop_open_link", { link: "/docs/" }],
      ["desktop_log_list_days", undefined],
      [
        "desktop_log_read_page",
        {
          day: "2026-08-19",
          level: "WARN",
          limit: 50,
          offset: 20,
          query: "timeout",
        },
      ],
      ["desktop_log_open_folder", undefined],
      ["desktop_update_check", undefined],
      ["desktop_update_install", undefined],
      ["desktop_window_show_main", undefined],
      ["desktop_window_hide_main", undefined],
      ["desktop_window_open_logs", undefined],
    ]);

    const payloads: unknown[] = [];
    const listeners = [
      await desktopFacade.logs.onAppend((event) => payloads.push(event)),
      await desktopFacade.updates.onAvailable((event) => payloads.push(event)),
      await desktopFacade.windows.onSecondInstance(() => payloads.push("second-instance")),
      await desktopFacade.menu.onOpenSettings(() => payloads.push("settings")),
    ];
    expect(mocks.listen.mock.calls.map(([eventName]) => eventName)).toEqual([
      "jftrade:desktop-log:append",
      "jftrade:desktop-update:available",
      "jftrade:desktop-second-instance",
      "jftrade:desktop-menu:settings",
    ]);
    expect(payloads).toEqual([
      { eventName: "jftrade:desktop-log:append" },
      { eventName: "jftrade:desktop-update:available" },
      "second-instance",
      "settings",
    ]);
    for (const cancel of listeners) cancel();
    expect(unlisten).toHaveBeenCalledTimes(4);
  });

  it("fails every desktop-only command closed and makes browser listeners inert", async () => {
    expect(resolveDesktopBackend()).toBe("browser");
    const commands = [
      desktopFacade.startup.snapshot(),
      desktopFacade.startup.quit(),
      desktopFacade.links.open("https://example.com"),
      desktopFacade.logs.listDays(),
      desktopFacade.logs.readPage("2026-08-19", "ALL", "", 0, 200),
      desktopFacade.logs.openFolder(),
      desktopFacade.updates.check(),
      desktopFacade.updates.install(),
      desktopFacade.windows.showMain(),
      desktopFacade.windows.hideMain(),
      desktopFacade.windows.openLogs(),
    ];
    for (const command of commands) {
      await expect(command).rejects.toThrow("desktop facade is unavailable");
    }

    const listeners = await Promise.all([
      desktopFacade.logs.onAppend(() => undefined),
      desktopFacade.updates.onAvailable(() => undefined),
      desktopFacade.windows.onSecondInstance(() => undefined),
      desktopFacade.menu.onOpenSettings(() => undefined),
    ]);
    for (const cancel of listeners) expect(cancel()).toBeUndefined();
    expect(mocks.listen).not.toHaveBeenCalled();
  });
});
