import { readdirSync, readFileSync } from "node:fs";
import { extname, join } from "node:path";

import { describe, expect, it } from "vitest";

const sourceRoot = new URL("../../src/", import.meta.url).pathname;

function vueSources(root: string): string[] {
  return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const path = join(root, entry.name);
    if (entry.isDirectory()) return vueSources(path);
    return extname(entry.name) === ".vue" ? [path] : [];
  });
}

describe("shared tab semantics contract", () => {
  it("routes every production tablist through AppTabs", () => {
    const offenders = vueSources(sourceRoot)
      .filter((path) => !path.endsWith("/components/shared/AppTabs.vue"))
      .filter((path) => {
        const source = readFileSync(path, "utf8");
        return /<v-tabs\b|<v-tab(?:\s|>)|role=["']tablist["']/.test(source);
      })
      .map((path) => path.slice(sourceRoot.length));

    expect(offenders).toEqual([]);
  });

  it("keeps filtering and mode choices out of tab semantics", () => {
    const segmentedSources = [
      "components/domain/market-data/CompactInstrumentNews.vue",
      "components/research/EarningsCalendarToolbar.vue",
      "components/research/StockScreenerDialogs.vue",
      "components/settings/SettingsDataManagementSection.vue",
    ];

    for (const path of segmentedSources) {
      const source = readFileSync(join(sourceRoot, path), "utf8");
      expect(source).toContain("SegmentedControl");
      expect(source).not.toMatch(/role=["']tablist["']/);
    }
  });
});
