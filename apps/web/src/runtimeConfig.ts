type JFTradeRuntimeConfig = {
  apiBaseUrl?: string;
};

declare global {
  interface Window {
    __JFTRADE_RUNTIME_CONFIG__?: JFTradeRuntimeConfig;
  }
}

const buildTimeApiBaseUrl = (
  import.meta.env.VITE_API_BASE_URL as string | undefined
)?.replace(/\/$/, "");
const defaultDevelopmentApiBaseUrl = "http://127.0.0.1:3000";

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
    defaultDevelopmentApiBaseUrl
  );
}

export function buildRuntimeApiUrl(path: string): string {
  return `${resolveApiBaseUrl()}${path}`;
}

export function buildRuntimeLiveSocketUrl(path: string): string {
  const url = new URL(resolveApiBaseUrl());
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = path;
  url.search = "";
  url.hash = "";
  return url.toString();
}