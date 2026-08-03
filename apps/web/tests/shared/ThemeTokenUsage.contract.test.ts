import { readdirSync, readFileSync } from "node:fs";
import { extname, join } from "node:path";

import { describe, expect, it } from "vitest";

const sourceRoot = new URL("../../src/", import.meta.url);
const sourceExtensions = new Set([".css", ".vue"]);

function productionStyleSources(): string {
  return walk(new URL(".", sourceRoot).pathname)
    .map((path) => readFileSync(path, "utf8"))
    .join("\n");
}

function walk(root: string): string[] {
  return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const path = join(root, entry.name);
    if (entry.isDirectory()) return walk(path);
    return sourceExtensions.has(extname(entry.name)) ? [path] : [];
  });
}

describe("theme token usage contract", () => {
  it("uses the canonical warning token without hard-coded fallbacks", () => {
    const sources = productionStyleSources();

    expect(sources).not.toContain("--tv-warning");
    expect(sources).not.toMatch(/var\(--[^,()]+,\s*#[0-9a-f]{3,8}\)/i);
  });

  it("routes every static letter spacing value through the tracking scale", () => {
    const sources = productionStyleSources();

    expect(sources).not.toMatch(
      /letter-spacing\s*:\s*-?(?:\d+(?:\.\d+)?|\.\d+)(?:em|rem|px)?\b/,
    );
    expect(sources).toContain("--jf-tracking-tight: -0.02em");
    expect(sources).toContain("--jf-tracking-11: 0.22em");
  });
});
