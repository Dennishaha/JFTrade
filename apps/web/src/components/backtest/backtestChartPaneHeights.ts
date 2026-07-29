const PANE_HEIGHT_RATIOS = [0.52, 0.12, 0.22, 0.14] as const;
const PANE_MIN_HEIGHTS = [180, 44, 72, 44] as const;

export function resolveBacktestChartPaneHeights(totalHeight: number): number[] {
  const targetHeight = Math.max(1, Math.floor(totalHeight));
  const baseMinimum = PANE_MIN_HEIGHTS.reduce((sum, height) => sum + height, 0);
  const scale = targetHeight < baseMinimum ? targetHeight / baseMinimum : 1;
  const minimums = PANE_MIN_HEIGHTS.map((height) => Math.floor(height * scale));
  const heights = PANE_HEIGHT_RATIOS.map((ratio, index) =>
    Math.max(minimums[index]!, Math.floor(targetHeight * ratio)),
  );

  let drift = targetHeight - heights.reduce((sum, height) => sum + height, 0);
  if (drift > 0) {
    heights[0] = heights[0]! + drift;
    return heights;
  }

  for (const index of [2, 3, 0, 1]) {
    if (drift === 0) {
      break;
    }
    const available = heights[index]! - minimums[index]!;
    const reduction = Math.min(available, -drift);
    heights[index] = heights[index]! - reduction;
    drift += reduction;
  }

  for (const index of [3, 2, 1, 0]) {
    if (drift === 0) {
      break;
    }
    const reduction = Math.min(heights[index]!, -drift);
    heights[index] = heights[index]! - reduction;
    drift += reduction;
  }

  return heights;
}
