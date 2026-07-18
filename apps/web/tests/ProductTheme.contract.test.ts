import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const sourceRoot = new URL("../src/", import.meta.url);

function source(path: string): string {
  return readFileSync(new URL(path, sourceRoot), "utf8");
}

describe("product surface theme contract", () => {
  it("uses one theme-aware segmented control across product workspaces", () => {
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
    expect(predictionWorkspace.match(/product-segmented-control/g)).toHaveLength(
      3,
    );
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
});
