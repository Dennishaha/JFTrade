import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const productRoot = new URL("../src/components/product/", import.meta.url);
const globalStyles = readFileSync(
  new URL("../src/style.css", import.meta.url),
  "utf8",
);
const workspacePage = readFileSync(
  new URL("../src/pages/WorkspacePage.vue", import.meta.url),
  "utf8",
);

function source(name: string): string {
  return readFileSync(new URL(name, productRoot), "utf8");
}

describe("option combo layout contract", () => {
  it("sizes the ticket from its own container instead of the viewport", () => {
    const builder = source("OptionComboBuilder.vue");
    const styles = source("OptionComboBuilder.css");

    expect(builder).toContain('class="option-combo-ticket__legs"');
    expect(builder.match(/tv-scrollbar/g)).toHaveLength(2);
    expect(styles).toContain("container-name: option-combo-ticket");
    expect(styles).toContain(
      "grid-template-columns: minmax(0, 1fr) clamp(260px, 34%, 340px)",
    );
    expect(styles).toContain(
      "@container option-combo-ticket (max-width: 940px)",
    );
    expect(styles).toContain(
      "@container option-combo-ticket (max-width: 720px)",
    );
    expect(styles).toContain(
      "grid-template-columns: minmax(0, 58fr) minmax(0, 42fr)",
    );
    expect(styles).not.toContain("border-left: 0");
    expect(styles).toContain("height: 100%");
    expect(styles).toContain("flex: 1 1 auto");
    expect(styles).not.toContain("min-height: 190px");
    expect(styles).toContain("overflow-y: auto");
    expect(styles).toContain("overscroll-behavior: contain");
    expect(styles).toContain("scrollbar-gutter: auto");
    expect(styles).toContain("grid-template-rows: max-content max-content");
    expect(styles).toContain(".option-combo-ticket__legs");
    expect(globalStyles).toContain(".tv-scrollbar::-webkit-scrollbar");
    expect(globalStyles).toContain(".tv-scrollbar::-webkit-scrollbar-thumb");
  });

  it("collapses dense leg and dock controls from their actual container width", () => {
    const legEditor = source("OptionComboLegEditor.vue");
    const dock = source("OptionTradingDock.vue");

    expect(legEditor).toContain(
      "@container option-combo-ticket (max-width: 940px)",
    );
    expect(legEditor).toContain("combo-leg-editor__quote-heading");
    expect(dock).toContain("container-name: option-trading-dock");
    expect(dock).toContain(
      "@container option-trading-dock (max-width: 620px)",
    );
    expect(dock).toContain(".option-trading-dock__body > *");
    expect(dock).toMatch(
      /\.option-trading-dock__body\s*\{[\s\S]*?overflow: hidden/,
    );
    expect(dock).not.toContain("@media (max-width: 840px)");
  });

  it("uses the shared thin scrollbar across the option chain surfaces", () => {
    const workspace = source("OptionWorkspacePanel.vue");
    const chainTable = source("OptionChainTable.vue");
    const workspaceStyles = source("optionWorkspace.css");

    expect(workspace.match(/tv-scrollbar/g)).toHaveLength(4);
    expect(chainTable).toContain(
      'class="option-chain-table__scroll tv-scrollbar"',
    );
    expect(workspaceStyles.match(/scrollbar-gutter: auto/g)).toHaveLength(4);
  });

  it("collapses the workspace pane together with the option trading dock", () => {
    expect(workspacePage.match(/v-model:collapsed/g)).toHaveLength(2);
    expect(workspacePage).toContain("tv-workspace__left-split");
    expect(workspacePage).toContain(
      ".tv-workspace__left-split.is-option-dock-collapsed",
    );
    expect(workspacePage).toContain("height: calc(100% - 36px) !important");
    expect(workspacePage).toContain("height: 36px !important");
  });
});
