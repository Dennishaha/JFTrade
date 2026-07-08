// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";

import { openExternalUrl } from "../src/composables/externalLink";
import { useDocsLink } from "../src/composables/useDocsLink";

afterEach(() => {
  delete window.wails;
  vi.restoreAllMocks();
});

describe("externalLink", () => {
  it("uses the Wails desktop binding when available", async () => {
    const byID = vi.fn(async () => undefined);
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    window.wails = { Call: { ByID: byID } };

    await openExternalUrl("/docs/index.html");

    expect(byID).toHaveBeenCalledWith(0x4a465401, "/docs/index.html");
    expect(open).not.toHaveBeenCalled();
  });

  it("falls back to window.open when the desktop binding fails", async () => {
    const byID = vi.fn(async () => {
      throw new Error("desktop unavailable");
    });
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    window.wails = { Call: { ByID: byID } };

    await openExternalUrl("https://example.com/docs");

    expect(byID).toHaveBeenCalledWith(0x4a465401, "https://example.com/docs");
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
