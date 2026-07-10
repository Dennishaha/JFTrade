// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

const openLinkBinding = vi.hoisted(() => vi.fn(async () => undefined));

vi.mock(
  "../src/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktoplinkservice",
  () => ({ OpenLink: openLinkBinding }),
);

import { openExternalUrl } from "../src/composables/externalLink";
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
});
