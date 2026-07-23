import { readdirSync, readFileSync } from "node:fs";
import { extname, relative } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const sourceRoot = fileURLToPath(new URL("../src/", import.meta.url));
const sourceExtensions = new Set([".css", ".ts", ".vue"]);

function sourceFiles(directory = sourceRoot): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = `${directory}/${entry.name}`;
    if (entry.isDirectory()) return sourceFiles(path);
    return sourceExtensions.has(extname(entry.name)) ? [path] : [];
  });
}

function source(path: string): string {
  return readFileSync(`${sourceRoot}/${path}`, "utf8");
}

function matchingFiles(pattern: RegExp): string[] {
  return sourceFiles()
    .filter((path) => pattern.test(readFileSync(path, "utf8")))
    .map((path) => relative(sourceRoot, path))
    .sort();
}

describe("directional color contract", () => {
  it("keeps price colors runtime-owned and removes the ambiguous legacy variables", () => {
    expect(matchingFiles(/--tv-(?:up|down)\b/)).toEqual([]);
    expect(source("style.css")).not.toMatch(
      /^\s*--tv-price-(?:up|down)\s*:/m,
    );

    const preferences = source("composables/useUIColorPreferences.ts");
    expect(preferences).toContain(
      'setProperty("--tv-price-up", colors.upColor)',
    );
    expect(preferences).toContain(
      'setProperty("--tv-price-down", colors.downColor)',
    );
  });

  it("limits price-direction tokens to directional market and trading surfaces", () => {
    expect(matchingFiles(/--tv-price-(?:up|down)\b/)).toEqual([
      "components/SettingsAppearanceSection.vue",
      "components/domain/market-data/OrderBookDepthTable.vue",
      "components/domain/shared/DenseMetricStrip.vue",
      "components/domain/watchlist/WatchlistVirtualTable.vue",
      "components/product/OptionChainTable.vue",
      "components/product/OptionComboConfirmDialog.vue",
      "components/product/OptionComboLegEditor.vue",
      "components/product/OptionComboRiskStrip.vue",
      "components/research/SectorHeatmap.vue",
      "components/research/SparklineChart.vue",
      "components/workspace/InstrumentOverviewPanel.vue",
      "components/workspace/OrderBookPanel.vue",
      "composables/useUIColorPreferences.ts",
      "pages/BacktestPage.vue",
      "style.css",
    ]);

    expect(matchingFiles(/\btv-(?:up|down)\b/)).toEqual([
      "components/SettingsAppearanceSection.vue",
      "components/domain/account/AccountAssetStrip.vue",
      "components/domain/account/AccountSummarySidebar.vue",
      "components/domain/account/PositionsTable.vue",
      "components/domain/market-data/QuoteSummaryCard.vue",
      "components/domain/market-data/useVerticalQuoteWorkbench.ts",
      "components/research/RankListPanel.vue",
      "components/research/researchEntry.ts",
      "components/workspace/OrderBookPanel.vue",
      "components/workspace/PositionsPanel.vue",
      "pages/BacktestPage.vue",
      "style.css",
    ]);
  });

  it("keeps non-directional feedback on semantic status and category colors", () => {
    expect(source("components/product/OptionComboPreviewResult.vue")).toContain(
      "var(--tv-status-success-fg)",
    );
    expect(source("components/product/OptionComboPreviewResult.vue")).toContain(
      "var(--tv-status-error-fg)",
    );
    expect(source("components/product/OptionComboBuilder.css")).toContain(
      "var(--tv-status-warning-fg)",
    );
    expect(source("components/product/OptionTradingDock.vue")).toContain(
      "var(--tv-status-warning-fg)",
    );
    expect(source("components/product/NewsWorkspacePanel.vue")).toContain(
      ".news-workspace__meta > span.is-rating {\n  color: var(--tv-accent);",
    );
    expect(source("components/workspace/OrderBookPanel.vue")).not.toContain(
      "tv-ob-preset-error",
    );
  });
});
