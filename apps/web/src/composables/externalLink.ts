const DESKTOP_OPEN_LINK_METHOD_ID = 0x4a465401;
const WAILS_RUNTIME_URL = "/wails/runtime.js";

type WailsCallRuntime = {
  Call?: {
    ByID?: (methodID: number, ...args: unknown[]) => Promise<unknown>;
  };
};
type WailsByID = NonNullable<WailsCallRuntime["Call"]>["ByID"];

declare global {
  interface Window {
    wails?: WailsCallRuntime;
  }
}

let runtimeLoadPromise: Promise<void> | null = null;

function isDesktopWailsOrigin(): boolean {
  if (typeof window === "undefined") return false;
  const { protocol, hostname } = window.location;
  return protocol === "wails:" || hostname === "wails.localhost";
}

function getWailsCallByID(): WailsByID | null {
  if (typeof window === "undefined") return null;
  const byID = window.wails?.Call?.ByID;
  return typeof byID === "function" ? byID : null;
}

async function loadWailsRuntimeIfNeeded(): Promise<void> {
  if (getWailsCallByID() != null) return;
  if (!isDesktopWailsOrigin()) return;
  if (runtimeLoadPromise != null) return runtimeLoadPromise;

  runtimeLoadPromise = import(/* @vite-ignore */ WAILS_RUNTIME_URL)
    .then((runtime) => {
      const maybeRuntime = runtime as WailsCallRuntime;
      if (typeof maybeRuntime.Call?.ByID === "function" && typeof window !== "undefined") {
        window.wails = {
          ...(window.wails ?? {}),
          Call: maybeRuntime.Call,
        };
      }
    })
    .catch(() => {
      runtimeLoadPromise = null;
    });

  return runtimeLoadPromise;
}

function openWithBrowserFallback(url: string): void {
  if (typeof window === "undefined") return;
  const opened = window.open(url, "_blank", "noopener,noreferrer");
  if (opened == null && isDesktopWailsOrigin() && isDocsLink(url)) {
    window.location.href = url;
  }
}

function isDocsLink(url: string): boolean {
  const trimmed = url.trim();
  return trimmed === "/docs" || trimmed === "docs" || trimmed.startsWith("/docs/") || trimmed.startsWith("docs/");
}

export async function openExternalUrl(url: string): Promise<void> {
  const target = url.trim();
  if (target === "") return;

  await loadWailsRuntimeIfNeeded();
  const byID = getWailsCallByID();
  if (byID != null) {
    try {
      await byID(DESKTOP_OPEN_LINK_METHOD_ID, target);
      return;
    } catch {
      // Fall through to the browser behavior used by the web build.
    }
  }

  openWithBrowserFallback(target);
}

export function handleExternalLinkClick(event: MouseEvent, url: string): void {
  if (event.defaultPrevented) return;
  if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
    return;
  }
  event.preventDefault();
  void openExternalUrl(url);
}

export function useExternalLink(): {
  openExternalUrl: (url: string) => Promise<void>;
  handleExternalLinkClick: (event: MouseEvent, url: string) => void;
} {
  return { openExternalUrl, handleExternalLinkClick };
}
