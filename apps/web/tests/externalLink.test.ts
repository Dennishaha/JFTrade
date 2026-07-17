// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

const openLinkBinding = vi.hoisted(() => vi.fn(async () => undefined));

vi.mock(
  "../src/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplinkservice",
  () => ({ OpenLink: openLinkBinding }),
);

import {
  handleExternalLinkClick,
  openExternalUrl,
  useExternalLink,
} from "../src/composables/externalLink";
import { useDocsLink } from "../src/composables/useDocsLink";

afterEach(() => {
  delete window.__JFTRADE_RUNTIME_CONFIG__;
  openLinkBinding.mockReset();
  openLinkBinding.mockResolvedValue(undefined);
  vi.restoreAllMocks();
});

describe("externalLink", () => {
  it("uses the Wails desktop binding when available", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    window.__JFTRADE_RUNTIME_CONFIG__ = { desktopMode: true };

    await openExternalUrl("/docs/index.html");

    expect(openLinkBinding).toHaveBeenCalledWith("/docs/index.html");
    expect(open).not.toHaveBeenCalled();
  });

  it("falls back to window.open when the desktop binding fails", async () => {
    openLinkBinding.mockRejectedValue(new Error("desktop unavailable"));
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    window.__JFTRADE_RUNTIME_CONFIG__ = { desktopMode: true };

    await openExternalUrl("https://example.com/docs");

    expect(openLinkBinding).toHaveBeenCalledWith("https://example.com/docs");
    expect(open).toHaveBeenCalledWith(
      "https://example.com/docs",
      "_blank",
      "noopener,noreferrer",
    );
  });

  it("keeps bundled documentation reachable when a desktop popup is blocked", async () => {
    openLinkBinding.mockRejectedValue(new Error("desktop binding unavailable"));
    const open = vi.fn(() => null);
    const desktopWindow = {
      __JFTRADE_RUNTIME_CONFIG__: { desktopMode: true },
      location: { protocol: "wails:", hostname: "wails.localhost", href: "" },
      open,
    };
    vi.resetModules();
    vi.stubGlobal("window", desktopWindow);
    try {
      const desktopLinks = await import("../src/composables/externalLink");
      await desktopLinks.openExternalUrl("/docs/reference/index.html");
      expect(open).toHaveBeenCalledWith(
        "/docs/reference/index.html",
        "_blank",
        "noopener,noreferrer",
      );
      expect(desktopWindow.location.href).toBe("/docs/reference/index.html");
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("does not attempt a browser navigation while rendering on the server", async () => {
    vi.resetModules();
    vi.stubGlobal("window", undefined);
    try {
      const serverLinks = await import("../src/composables/externalLink");
      await expect(
        serverLinks.openExternalUrl("https://example.com/docs"),
      ).resolves.toBeUndefined();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("opens docs through the shared opener", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);

    useDocsLink().openDocs("reference/index.html");
    await vi.waitFor(() => {
      expect(open).toHaveBeenCalledWith(
        "/docs/reference/index.html",
        "_blank",
        "noopener,noreferrer",
      );
    });
  });

  it("ignores blank URLs and preserves native modified-link behavior", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);

    await openExternalUrl("   ");
    expect(open).not.toHaveBeenCalled();

    for (const init of [
      { button: 1 },
      { metaKey: true },
      { ctrlKey: true },
      { shiftKey: true },
      { altKey: true },
    ]) {
      const event = new MouseEvent("click", { bubbles: true, cancelable: true, ...init });
      handleExternalLinkClick(event, "https://example.com/native");
      expect(event.defaultPrevented).toBe(false);
    }
    expect(open).not.toHaveBeenCalled();
  });

  it("intercepts ordinary links and exposes the same shared actions", async () => {
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const event = new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 });

    handleExternalLinkClick(event, " https://example.com/route ");
    expect(event.defaultPrevented).toBe(true);
    await vi.waitFor(() => {
      expect(open).toHaveBeenCalledWith(
        "https://example.com/route",
        "_blank",
        "noopener,noreferrer",
      );
    });

    const links = useExternalLink();
    expect(links.openExternalUrl).toBe(openExternalUrl);
    expect(links.handleExternalLinkClick).toBe(handleExternalLinkClick);

    const alreadyPrevented = new MouseEvent("click", { bubbles: true, cancelable: true });
    alreadyPrevented.preventDefault();
    links.handleExternalLinkClick(alreadyPrevented, "https://example.com/ignored");
    expect(open).toHaveBeenCalledTimes(1);
  });
});
