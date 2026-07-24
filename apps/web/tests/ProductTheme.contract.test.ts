import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const sourceRoot = new URL("../src/", import.meta.url);

function source(path: string): string {
  return readFileSync(new URL(path, sourceRoot), "utf8");
}

describe("product surface theme contract", () => {
  it("keeps workspace controls themed and gives research prediction native semantic tabs", () => {
    const controls = source("styles/product-controls.css");
    const optionWorkspace = source(
      "components/product/OptionWorkspacePanel.vue",
    );
    const newsWorkspace = source("components/product/NewsWorkspacePanel.vue");
    const predictionWorkspace = source(
      "components/product/PredictionResearchPanel.vue",
    );

    expect(controls).toContain(".product-segmented-control.v-btn-group");
    expect(controls).toContain("background: var(--tv-bg-surface-2)");
    expect(controls).toContain(".product-segmented-control .v-btn__overlay");
    expect(optionWorkspace).toContain(
      'class="product-segmented-control tv-scrollbar"',
    );
    expect(newsWorkspace).toContain(
      "news-workspace__categories product-segmented-control",
    );
    expect(predictionWorkspace).toContain('role="tablist"');
    expect(predictionWorkspace).toContain('role="tab"');
    expect(predictionWorkspace).toContain(
      'class="prediction-research__segments"',
    );
    expect(predictionWorkspace).not.toContain("product-segmented-control");
  });

  it("themes compact fields and avoids fixed dark native controls", () => {
    const controls = source("styles/product-controls.css");
    const optionStyles = source("components/product/optionWorkspace.css");
    const predictionWorkspace = source(
      "components/product/PredictionResearchPanel.vue",
    );

    expect(controls).toContain("background: var(--tv-bg-surface)");
    expect(controls).toContain("background: var(--tv-bg-elevated)");
    expect(controls).toContain(
      "background: color-mix(in srgb, var(--tv-accent) 12%",
    );
    expect(optionStyles).toContain("color-scheme: inherit");
    expect(optionStyles).not.toContain("color-scheme: dark");
    expect(predictionWorkspace).not.toMatch(/--v-theme|--v-border/);
  });

  it("keeps stock-screen categories in an arrow-controlled horizontal track", () => {
    const stockScreener = source(
      "components/research/StockScreenerView.vue",
    );
    const categoryNavRule = stockScreener.match(
      /\.stock-screener-view__category-nav\s*\{([^}]+)\}/,
    )?.[1];
    const categoriesRule = stockScreener.match(
      /\.stock-screener-view__categories\s*\{([^}]+)\}/,
    )?.[1];

    expect(categoryNavRule).toContain("height: 32px");
    expect(categoryNavRule).toContain(
      "grid-template-columns: 28px minmax(0, 1fr) 28px",
    );
    expect(categoriesRule).toContain("overflow-x: auto");
    expect(categoriesRule).toContain("scrollbar-width: none");
    expect(stockScreener).toContain(
      'class="stock-screener-view__category-scroll stock-screener-view__category-scroll--previous"',
    );
    expect(stockScreener).toContain(
      'class="stock-screener-view__category-scroll stock-screener-view__category-scroll--next"',
    );
    expect(stockScreener).toContain('aria-label="向左滚动因子分类"');
    expect(stockScreener).toContain('aria-label="向右滚动因子分类"');
  });
});
