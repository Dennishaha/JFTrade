import { resolveDesktopMode } from "@/runtimeConfig";
import { desktopFacade } from "@/composables/shared/desktopFacade";

function isDesktopOrigin(): boolean {
  if (typeof window === "undefined") return false;
  return resolveDesktopMode() || desktopFacade.backend() === "tauri";
}

async function openWithDesktopBinding(url: string): Promise<boolean> {
  if (!isDesktopOrigin()) return false;
  try {
    await desktopFacade.links.open(url);
    return true;
  } catch {
    return false;
  }
}

function openWithBrowserFallback(url: string): void {
  if (typeof window === "undefined") return;
  const opened = window.open(url, "_blank", "noopener,noreferrer");
  if (opened == null && isDesktopOrigin() && isDocsLink(url)) {
    window.location.href = url;
  }
}

function isDocsLink(url: string): boolean {
  const trimmed = url.trim();
  return (
    trimmed === "/docs" ||
    trimmed === "docs" ||
    trimmed.startsWith("/docs/") ||
    trimmed.startsWith("docs/")
  );
}

export async function openExternalUrl(url: string): Promise<void> {
  const target = url.trim();
  if (target === "") return;
  if (await openWithDesktopBinding(target)) return;
  openWithBrowserFallback(target);
}

export function handleExternalLinkClick(event: MouseEvent, url: string): void {
  if (event.defaultPrevented) return;
  if (
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.shiftKey ||
    event.altKey
  ) {
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
