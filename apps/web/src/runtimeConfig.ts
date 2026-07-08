type JFTradeRuntimeConfig = {
  apiBaseUrl?: string;
  authRequired?: boolean;
  desktopMode?: boolean;
};

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
  if (typeof window !== "undefined" && window.__JFTRADE_RUNTIME_CONFIG__?.authRequired !== undefined) {
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
