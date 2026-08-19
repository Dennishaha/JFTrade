// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  invoke: vi.fn(),
  listen: vi.fn(),
  wailsCheck: vi.fn(),
  wailsListDays: vi.fn(),
  wailsOpen: vi.fn(),
  wailsOpenFolder: vi.fn(),
  wailsQuit: vi.fn(),
  wailsReadPage: vi.fn(),
  wailsSnapshot: vi.fn(),
  wailsEventsOn: vi.fn(),
}));

vi.mock("@tauri-apps/api/core", () => ({ invoke: mocks.invoke }));
vi.mock("@tauri-apps/api/event", () => ({ listen: mocks.listen }));
vi.mock("@wailsio/runtime", () => ({ Events: { On: mocks.wailsEventsOn } }));
vi.mock(
  "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplinkservice",
  () => ({ OpenLink: mocks.wailsOpen }),
);
vi.mock(
  "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktopstartupservice",
  () => ({ Snapshot: mocks.wailsSnapshot, Quit: mocks.wailsQuit }),
);
vi.mock(
  "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplogservice",
  () => ({
    ListDays: mocks.wailsListDays,
    OpenFolder: mocks.wailsOpenFolder,
    ReadPage: mocks.wailsReadPage,
  }),
);
vi.mock(
  "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktopupdateservice",
  () => ({ Check: mocks.wailsCheck }),
);

import {
  desktopFacade,
  resolveDesktopBackend,
} from "@/composables/shared/desktopFacade";

afterEach(() => {
  delete window.__JFTRADE_RUNTIME_CONFIG__;
  delete (window as typeof window & { __TAURI_INTERNALS__?: unknown })
    .__TAURI_INTERNALS__;
  for (const mock of Object.values(mocks)) mock.mockReset();
});

describe("desktopFacade", () => {
  it("resolves server, Tauri, Wails protocol, Wails host, and browser runtimes", () => {
    const browserWindow = window;

    vi.stubGlobal("window", undefined);
    expect(resolveDesktopBackend()).toBe("browser");

    vi.stubGlobal("window", {
      __JFTRADE_RUNTIME_CONFIG__: { desktopMode: false },
      location: { hostname: "example.com", protocol: "wails:" },
    });
    expect(resolveDesktopBackend()).toBe("wails");

    vi.stubGlobal("window", {
      __JFTRADE_RUNTIME_CONFIG__: { desktopMode: false },
      location: { hostname: "wails.localhost", protocol: "https:" },
    });
    expect(resolveDesktopBackend()).toBe("wails");

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

  it("keeps every Wails command and event adapter active until the native cutover", async () => {
    window.__JFTRADE_RUNTIME_CONFIG__ = { desktopMode: true };
    mocks.wailsSnapshot.mockResolvedValue({ state: "starting" });
    mocks.wailsQuit.mockResolvedValue(undefined);
    mocks.wailsOpen.mockResolvedValue(undefined);
    mocks.wailsListDays.mockResolvedValue([{ day: "2026-08-19" }]);
    mocks.wailsReadPage.mockResolvedValue({ day: "2026-08-19", items: [] });
    mocks.wailsOpenFolder.mockResolvedValue(undefined);
    mocks.wailsCheck.mockResolvedValue({ available: false, currentVersion: "1.0.0" });
    const cancel = vi.fn();
    mocks.wailsEventsOn.mockImplementation(
      (eventName: string, listener: (event: { data: unknown }) => void) => {
        listener({ data: { eventName } });
        return cancel;
      },
    );

    expect(resolveDesktopBackend()).toBe("wails");
    await desktopFacade.startup.snapshot();
    await desktopFacade.startup.quit();
    await desktopFacade.links.open("https://example.com");
    await desktopFacade.logs.listDays();
    await desktopFacade.logs.readPage("2026-08-19", "ALL", "", 0, 200);
    await desktopFacade.logs.openFolder();
    await desktopFacade.updates.check();
    expect(mocks.wailsSnapshot).toHaveBeenCalledOnce();
    expect(mocks.wailsQuit).toHaveBeenCalledOnce();
    expect(mocks.wailsOpen).toHaveBeenCalledWith("https://example.com");
    expect(mocks.wailsListDays).toHaveBeenCalledOnce();
    expect(mocks.wailsReadPage).toHaveBeenCalledWith(
      "2026-08-19",
      "ALL",
      "",
      0,
      200,
    );
    expect(mocks.wailsOpenFolder).toHaveBeenCalledOnce();
    expect(mocks.wailsCheck).toHaveBeenCalledOnce();

    const payloads: unknown[] = [];
    const listeners = [
      await desktopFacade.logs.onAppend((event) => payloads.push(event)),
      await desktopFacade.updates.onAvailable((event) => payloads.push(event)),
      await desktopFacade.windows.onSecondInstance(() => payloads.push("second-instance")),
      await desktopFacade.menu.onOpenSettings(() => payloads.push("settings")),
    ];
    expect(mocks.wailsEventsOn.mock.calls.map(([eventName]) => eventName)).toEqual([
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
    for (const stop of listeners) stop();
    expect(cancel).toHaveBeenCalledTimes(4);
    expect(mocks.invoke).not.toHaveBeenCalled();
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
    expect(mocks.wailsEventsOn).not.toHaveBeenCalled();
  });
});
