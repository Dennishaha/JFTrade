import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const sourceRoot = new URL("../src/", import.meta.url);

function source(path: string): string {
  return readFileSync(new URL(path, sourceRoot), "utf8");
}

describe("dark theme background contract", () => {
  it("uses the standard neutral black-gray surface hierarchy", () => {
    const styles = source("style.css");
    const darkThemeStart = styles.indexOf(':root,\n[data-theme="dark"]');
    const lightThemeStart = styles.indexOf('[data-theme="light"]');
    const darkTheme = styles.slice(darkThemeStart, lightThemeStart);

    expect(darkThemeStart).toBeGreaterThanOrEqual(0);
    expect(lightThemeStart).toBeGreaterThan(darkThemeStart);
    expect(darkTheme).toContain("--tv-bg-app: #0a0a0a");
    expect(darkTheme).toContain("--tv-bg-surface: #141414");
    expect(darkTheme).toContain("--tv-bg-surface-2: #1a1a1a");
    expect(darkTheme).toContain("--tv-bg-elevated: #242424");
    expect(darkTheme).toContain("--tv-bg-hover: #242424");
    expect(darkTheme).toContain("--card-surface: rgba(20, 20, 20, 0.90)");
    expect(darkTheme).toContain(
      "--card-surface-raised: rgba(36, 36, 36, 0.70)",
    );
  });

  it("keeps independent dark surfaces aligned with the global hierarchy", () => {
    const styles = source("style.css");
    const vuetify = source("main.ts");
    const kline = source("components/KlineChart.vue");
    const backtest = source("components/BacktestChart.vue");
    const workflow = source("styles/adk-workflow-studio.css");

    expect(styles).toContain(
      "linear-gradient(180deg, #0a0a0a 0%, #121212 45%, #0d0d0d 100%)",
    );
    expect(styles).toContain(".auth-gate {");
    expect(styles).toContain("background: #0a0a0a");
    expect(styles).toContain("background: #141414");
    expect(vuetify).toContain('background: "#0a0a0a"');
    expect(vuetify).toContain('surface: "#141414"');
    expect(kline).toContain('bg: "#1a1a1a"');
    expect(backtest).toContain('bg: "#1a1a1a"');
    expect(workflow).toContain(
      "linear-gradient(180deg, rgba(20, 20, 20, 0.96), rgba(10, 10, 10, 0.96))",
    );
    expect(workflow).toContain(
      "--adk-workflow-minimap-mask: rgba(0, 0, 0, 0.46)",
    );

    expect(`${styles}\n${vuetify}\n${workflow}`).not.toMatch(
      /#07111f|#0d1729|#0b1323|#0b1220|#111827|#1e293b/,
    );
  });
});
