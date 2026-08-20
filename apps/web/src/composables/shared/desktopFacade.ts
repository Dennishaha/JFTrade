import { resolveDesktopMode } from "@/runtimeConfig";

export type DesktopBackend = "browser" | "tauri" | "wails";
export type DesktopUnlisten = () => void;

export interface DesktopStartupSnapshot {
  state: string;
  phase: string;
  message: string;
  startedAt: string;
}

export interface DesktopLogDay {
  day: string;
}

export interface DesktopLogLine {
  level: string;
  text: string;
}

export interface DesktopLogPage {
  day: string;
  items: DesktopLogLine[] | null;
  offset: number;
  limit: number;
  total: number;
  logDir: string;
}

export interface DesktopUpdateResult {
  currentVersion: string;
  available: boolean;
  latestVersion?: string;
  releaseUrl?: string;
  publishedAt?: string;
  notes?: string;
}

export interface DesktopLogAppend {
  day: string;
  line: DesktopLogLine;
}

const logAppendEvent = "jftrade:desktop-log:append";
const updateAvailableEvent = "jftrade:desktop-update:available";
const secondInstanceEvent = "jftrade:desktop-second-instance";
const menuSettingsEvent = "jftrade:desktop-menu:settings";

export function resolveDesktopBackend(): DesktopBackend {
  if (typeof window === "undefined") return "browser";
  const runtimeWindow = window as typeof window & {
    __TAURI_INTERNALS__?: unknown;
  };
  if (runtimeWindow.__TAURI_INTERNALS__ != null) return "tauri";
  const { protocol, hostname } = window.location;
  if (
    resolveDesktopMode() ||
    protocol === "wails:" ||
    hostname === "wails.localhost"
  ) {
    return "wails";
  }
  return "browser";
}

function unavailable(operation: string): Error {
  return new Error(`desktop facade is unavailable for ${operation}`);
}

async function tauriInvoke<T>(command: string, args?: Record<string, unknown>): Promise<T> {
  const { invoke } = await import("@tauri-apps/api/core");
  return invoke<T>(command, args);
}

async function listenDesktopEvent<T>(
  eventName: string,
  listener: (payload: T) => void,
): Promise<DesktopUnlisten> {
  switch (resolveDesktopBackend()) {
    case "tauri": {
      const { listen } = await import("@tauri-apps/api/event");
      return listen<T>(eventName, (event) => listener(event.payload));
    }
    case "wails": {
      const { Events } = await import("@wailsio/runtime");
      return Events.On(eventName, (event) => listener(event.data as T));
    }
    case "browser":
      return () => undefined;
  }
}

export const desktopFacade = {
  backend: resolveDesktopBackend,
  startup: {
    async snapshot(): Promise<DesktopStartupSnapshot> {
      if (resolveDesktopBackend() === "tauri") {
        return tauriInvoke("desktop_startup_snapshot");
      }
      if (resolveDesktopBackend() === "wails") {
        const { Snapshot } = await import(
          "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktopstartupservice"
        );
        return Snapshot();
      }
      throw unavailable("startup snapshot");
    },
    async quit(): Promise<void> {
      if (resolveDesktopBackend() === "tauri") {
        return tauriInvoke("desktop_startup_quit");
      }
      if (resolveDesktopBackend() === "wails") {
        const { Quit } = await import(
          "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktopstartupservice"
        );
        return Quit();
      }
      throw unavailable("quit");
    },
  },
  links: {
    async open(link: string): Promise<void> {
      if (resolveDesktopBackend() === "tauri") {
        return tauriInvoke("desktop_open_link", { link });
      }
      if (resolveDesktopBackend() === "wails") {
        const { OpenLink } = await import(
          "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplinkservice"
        );
        return OpenLink(link);
      }
      throw unavailable("open link");
    },
  },
  logs: {
    async listDays(): Promise<DesktopLogDay[] | null> {
      if (resolveDesktopBackend() === "tauri") {
        return tauriInvoke("desktop_log_list_days");
      }
      if (resolveDesktopBackend() === "wails") {
        const { ListDays } = await import(
          "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplogservice"
        );
        return ListDays();
      }
      throw unavailable("list log days");
    },
    async readPage(
      day: string,
      level: string,
      query: string,
      offset: number,
      limit: number,
    ): Promise<DesktopLogPage> {
      if (resolveDesktopBackend() === "tauri") {
        return tauriInvoke("desktop_log_read_page", {
          day,
          level,
          query,
          offset,
          limit,
        });
      }
      if (resolveDesktopBackend() === "wails") {
        const { ReadPage } = await import(
          "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplogservice"
        );
        return ReadPage(day, level, query, offset, limit);
      }
      throw unavailable("read logs");
    },
    async openFolder(): Promise<void> {
      if (resolveDesktopBackend() === "tauri") {
        return tauriInvoke("desktop_log_open_folder");
      }
      if (resolveDesktopBackend() === "wails") {
        const { OpenFolder } = await import(
          "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplogservice"
        );
        return OpenFolder();
      }
      throw unavailable("open log folder");
    },
    onAppend(listener: (payload: DesktopLogAppend) => void): Promise<DesktopUnlisten> {
      return listenDesktopEvent(logAppendEvent, listener);
    },
  },
  updates: {
    async check(): Promise<DesktopUpdateResult> {
      if (resolveDesktopBackend() === "tauri") {
        return tauriInvoke("desktop_update_check");
      }
      if (resolveDesktopBackend() === "wails") {
        const { Check } = await import(
          "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktopupdateservice"
        );
        return Check();
      }
      throw unavailable("check updates");
    },
    async install(): Promise<void> {
      if (resolveDesktopBackend() === "tauri") {
        return tauriInvoke("desktop_update_install");
      }
      throw unavailable("install updates");
    },
    onAvailable(
      listener: (payload: DesktopUpdateResult) => void,
    ): Promise<DesktopUnlisten> {
      return listenDesktopEvent(updateAvailableEvent, listener);
    },
  },
  windows: {
    async showMain(): Promise<void> {
      if (resolveDesktopBackend() !== "tauri") throw unavailable("show main window");
      return tauriInvoke("desktop_window_show_main");
    },
    async hideMain(): Promise<void> {
      if (resolveDesktopBackend() !== "tauri") throw unavailable("hide main window");
      return tauriInvoke("desktop_window_hide_main");
    },
    async openLogs(): Promise<void> {
      if (resolveDesktopBackend() !== "tauri") throw unavailable("open log window");
      return tauriInvoke("desktop_window_open_logs");
    },
    onSecondInstance(listener: () => void): Promise<DesktopUnlisten> {
      return listenDesktopEvent(secondInstanceEvent, listener);
    },
  },
  menu: {
    onOpenSettings(listener: () => void): Promise<DesktopUnlisten> {
      return listenDesktopEvent(menuSettingsEvent, listener);
    },
  },
};
