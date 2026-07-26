import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const sourceRoot = new URL("../src/", import.meta.url);

function source(path: string): string {
  return readFileSync(new URL(path, sourceRoot), "utf8");
}

function declarations(styles: string, selector: string): string {
  const start = styles.indexOf(`${selector} {`);
  expect(start).toBeGreaterThanOrEqual(0);
  const bodyStart = styles.indexOf("{", start) + 1;
  const bodyEnd = styles.indexOf("}", bodyStart);
  return styles.slice(bodyStart, bodyEnd);
}

describe("split pane theme contract", () => {
  it("uses the strategy designer divider treatment for every splitter orientation", () => {
    const styles = source("style.css");

    const sharedTrack = declarations(
      styles,
      ".tv-splitpanes .splitpanes__splitter,\n.tv-resizer--vertical",
    );
    expect(sharedTrack).toContain("border: 0");
    expect(sharedTrack).toContain("background: transparent");
    expect(sharedTrack).toContain("overflow: visible");

    const sharedSplitter = declarations(
      styles,
      ".tv-splitpanes .splitpanes__splitter",
    );
    expect(sharedSplitter).toContain("z-index: 1");

    const horizontalTrack = declarations(
      styles,
      ".tv-splitpanes--horizontal > .splitpanes__splitter",
    );
    expect(horizontalTrack).toContain("height: 1px");
    expect(horizontalTrack).toContain("min-height: 1px");
    expect(horizontalTrack).toContain("cursor: row-resize");

    const horizontalHitTarget = declarations(
      styles,
      ".tv-splitpanes--horizontal > .splitpanes__splitter::before",
    );
    expect(horizontalHitTarget).toContain("right: 0");
    expect(horizontalHitTarget).toContain("left: 0");
    expect(horizontalHitTarget).toContain("height: 5px");
    expect(horizontalHitTarget).toContain("background: transparent");

    const horizontalLine = declarations(
      styles,
      ".tv-splitpanes--horizontal > .splitpanes__splitter::after",
    );
    expect(horizontalLine).toContain("height: 1px");
    expect(horizontalLine).toContain("background: var(--tv-border-strong)");
    expect(horizontalLine).toContain("pointer-events: none");

    const horizontalActiveLine = declarations(
      styles,
      ".tv-splitpanes--horizontal > .splitpanes__splitter:is(:hover, :focus-visible, :active)::after",
    );
    expect(horizontalActiveLine).toContain("height: 2px");
    expect(horizontalActiveLine).toContain("background: var(--tv-accent)");

    const verticalTrack = declarations(
      styles,
      ".tv-splitpanes--vertical > .splitpanes__splitter,\n.tv-resizer--vertical",
    );
    expect(verticalTrack).toContain("width: 1px");
    expect(verticalTrack).toContain("min-width: 1px");
    expect(verticalTrack).toContain("cursor: col-resize");

    const verticalHitTarget = declarations(
      styles,
      ".tv-splitpanes--vertical > .splitpanes__splitter::before,\n.tv-resizer--vertical::before",
    );
    expect(verticalHitTarget).toContain("top: 0");
    expect(verticalHitTarget).toContain("bottom: 0");
    expect(verticalHitTarget).toContain("width: 5px");
    expect(verticalHitTarget).toContain("background: transparent");

    const verticalLine = declarations(
      styles,
      ".tv-splitpanes--vertical > .splitpanes__splitter::after,\n.tv-resizer--vertical::after",
    );
    expect(verticalLine).toContain("width: 1px");
    expect(verticalLine).toContain("background: var(--tv-border-strong)");
    expect(verticalLine).toContain("pointer-events: none");

    const verticalActiveLine = declarations(
      styles,
      ".tv-splitpanes--vertical > .splitpanes__splitter:is(:hover, :focus-visible, :active)::after,\n.tv-resizer--vertical:is(:hover, :focus-visible, :active)::after",
    );
    expect(verticalActiveLine).toContain("width: 2px");
    expect(verticalActiveLine).toContain("background: var(--tv-accent)");

    const mobileVerticalTrack = declarations(
      styles,
      "  .tv-splitpanes--vertical > .splitpanes__splitter",
    );
    expect(mobileVerticalTrack).toContain("height: 1px");
    expect(mobileVerticalTrack).toContain("min-height: 1px");
    expect(mobileVerticalTrack).toContain("cursor: row-resize");
    expect(mobileVerticalTrack).not.toContain("border-");

    const mobileVerticalHitTarget = declarations(
      styles,
      "  .tv-splitpanes--vertical > .splitpanes__splitter::before",
    );
    expect(mobileVerticalHitTarget).toContain("right: 0");
    expect(mobileVerticalHitTarget).toContain("left: 0");
    expect(mobileVerticalHitTarget).toContain("width: auto");
    expect(mobileVerticalHitTarget).toContain("height: 5px");

    const mobileVerticalLine = declarations(
      styles,
      "  .tv-splitpanes--vertical > .splitpanes__splitter::after",
    );
    expect(mobileVerticalLine).toContain("width: auto");
    expect(mobileVerticalLine).toContain("height: 1px");

    const mobileVerticalActiveLine = declarations(
      styles,
      "  .tv-splitpanes--vertical > .splitpanes__splitter:is(:hover, :focus-visible, :active)::after",
    );
    expect(mobileVerticalActiveLine).toContain("width: auto");
    expect(mobileVerticalActiveLine).toContain("height: 2px");
  });

  it("keeps the strategy designer on the shared theme without local visual overrides", () => {
    const strategyDesigner = source("components/StrategyDesignStage.vue");

    expect(strategyDesigner).not.toContain("splitpanes__splitter::before");
    expect(strategyDesigner).not.toContain(
      "splitpanes__splitter:is(:hover, :focus-visible, :active)",
    );
  });

  it("aligns the watchlist resizer with the panel edge", () => {
    const styles = source("style.css");
    const workspace = source("pages/WorkspacePage.vue");
    const rightDockSlot = declarations(styles, ".tv-rightdock-slot");
    const rightDockResizer = declarations(styles, ".tv-rightdock-resizer");
    const watchlistResizer = declarations(
      workspace,
      ".tv-workspace__watchlist-resizer",
    );

    expect(rightDockSlot).toContain("overflow: visible");
    expect(rightDockResizer).toContain("left: 0");
    expect(watchlistResizer).toContain("right: 0");
    expect(watchlistResizer).not.toContain("right: -3px");
  });
});
