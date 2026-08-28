import { invoke as tauriInvoke } from "@tauri-apps/api/core";

export type JFTradeRuntimeConfig = {
  apiBaseUrl?: string;
  authRequired?: boolean;
  desktopMode?: boolean;
  desktopApiToken?: string;
};

export async function initializeTauriRuntimeConfig(): Promise<void> {
  if (typeof window === "undefined") return;
  const runtimeWindow = window as typeof window & {
    __TAURI_INTERNALS__?: unknown;
  };
  if (runtimeWindow.__TAURI_INTERNALS__ == null) {
    return;
  }
  const maxAttempts = 50;
  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    try {
      const config = await tauriInvoke<JFTradeRuntimeConfig>("desktop_runtime_config");
      const apiBaseUrl = normalizeApiBaseUrl(config.apiBaseUrl);
      const token = config.desktopApiToken?.trim();
      if (!isLoopbackHttpUrl(apiBaseUrl) || !token || token.length < 32) {
        throw new Error("Tauri runtime returned an unsafe desktop API configuration");
      }
      window.__JFTRADE_RUNTIME_CONFIG__ = {
        apiBaseUrl,
        authRequired: config.authRequired === true,
        desktopMode: true,
        desktopApiToken: token,
      };
      return;
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      if (
        (message.includes("DESKTOP_NOT_READY") || message.includes("starting")) &&
        attempt < maxAttempts - 1
      ) {
        await new Promise((resolve) => setTimeout(resolve, 100));
        continue;
      }
      throw error;
    }
  }
}

declare global {
  interface Window {
    __JFTRADE_RUNTIME_CONFIG__?: JFTradeRuntimeConfig;
  }
}

const buildTimeApiBaseUrl = (
  import.meta.env.VITE_API_BASE_URL as string | undefined
)?.replace(/\/$/, "");

function normalizeApiBaseUrl(value: string | null | undefined): string | null {
  const trimmedValue = value?.trim().replace(/\/$/, "");
  return trimmedValue ? trimmedValue : null;
}

function isLoopbackHttpUrl(value: string | null): value is string {
  if (!value) return false;
  try {
    const url = new URL(value);
    return (
      url.protocol === "http:" &&
      (url.hostname === "127.0.0.1" || url.hostname === "[::1]") &&
      url.username === "" &&
      url.password === "" &&
      url.pathname === "/" &&
      url.search === "" &&
      url.hash === ""
    );
  } catch {
    return false;
  }
}

function resolveRuntimeApiBaseUrl(): string | null {
  if (typeof window === "undefined") {
    return null;
  }

  return normalizeApiBaseUrl(window.__JFTRADE_RUNTIME_CONFIG__?.apiBaseUrl);
}

export function resolveApiBaseUrl(): string {
  return (
    resolveRuntimeApiBaseUrl() ??
    normalizeApiBaseUrl(buildTimeApiBaseUrl) ??
    resolveDevelopmentApiBaseUrl()
  );
}

function resolveDevelopmentApiBaseUrl(): string {
  if (import.meta.env.PROD) {
    return "";
  }
  return "";
}

export function resolveAuthRequired(): boolean {
  if (
    typeof window !== "undefined" &&
    window.__JFTRADE_RUNTIME_CONFIG__?.authRequired !== undefined
  ) {
    return window.__JFTRADE_RUNTIME_CONFIG__.authRequired;
  }
  return import.meta.env.MODE !== "test";
}

export function resolveDesktopMode(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  return window.__JFTRADE_RUNTIME_CONFIG__?.desktopMode === true;
}

export function resolveDesktopBridgeAvailable(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  const runtimeWindow = window as typeof window & {
    chrome?: { webview?: { postMessage?: unknown } };
    __TAURI_INTERNALS__?: unknown;
    __TAURI__?: unknown;
    webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } };
  };
  return (
    typeof runtimeWindow.chrome?.webview?.postMessage === "function" ||
    runtimeWindow.__TAURI_INTERNALS__ != null ||
    runtimeWindow.__TAURI__ != null ||
    typeof runtimeWindow.webkit?.messageHandlers?.external?.postMessage ===
      "function"
  );
}

export function resolveDesktopApiToken(): string | null {
  if (typeof window === "undefined") {
    return null;
  }
  const token = window.__JFTRADE_RUNTIME_CONFIG__?.desktopApiToken?.trim();
  return token || null;
}

export function buildRuntimeApiUrl(path: string): string {
  const apiBaseUrl = resolveApiBaseUrl();
  return apiBaseUrl ? `${apiBaseUrl}${path}` : path;
}

export function buildRuntimeLiveSocketUrl(path: string): string {
  const apiBaseUrl = resolveApiBaseUrl();
  const url = new URL(
    apiBaseUrl ||
      (typeof window === "undefined"
        ? "http://127.0.0.1"
        : window.location.origin),
  );
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = path;
  url.search = "";
  url.hash = "";
  return url.toString();
}
