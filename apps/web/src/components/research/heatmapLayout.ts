/**
 * Squarified treemap 布局纯函数（Bruls et al.），不依赖任何第三方库。
 * 输入带 value 权重的条目列表与容器尺寸，输出每个条目的绝对定位矩形。
 * 面积与 value 成正比（总面积守恒），矩形保证不越出容器边界。
 */

export interface TreemapRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface TreemapLayoutEntry<T> {
  item: T;
  /** 条目在原始输入数组中的下标 */
  index: number;
  rect: TreemapRect;
}

interface ScaledItem<T> {
  item: T;
  index: number;
  area: number;
}

function worstAspectRatio<T>(row: readonly ScaledItem<T>[], side: number): number {
  const sum = row.reduce((total, entry) => total + entry.area, 0);
  if (sum <= 0 || side <= 0) return Number.POSITIVE_INFINITY;
  let max = 0;
  let min = Number.POSITIVE_INFINITY;
  for (const entry of row) {
    max = Math.max(max, entry.area);
    min = Math.min(min, entry.area);
  }
  const sideSquared = side * side;
  return Math.max(
    (sideSquared * max) / (sum * sum),
    (sum * sum) / (sideSquared * min),
  );
}

export function squarifiedLayout<T extends { value: number }>(
  items: readonly T[],
  width: number,
  height: number,
): TreemapLayoutEntry<T>[] {
  if (!(width > 0) || !(height > 0)) return [];

  // 权重按降序排列是 squarified 算法的经典前提，同时过滤无效权重
  const scaled: ScaledItem<T>[] = items
    .map((item, index) => ({ item, index, area: 0 }))
    .filter((entry) => Number.isFinite(entry.item.value) && entry.item.value > 0)
    .sort((left, right) => right.item.value - left.item.value);

  const totalValue = scaled.reduce((total, entry) => total + entry.item.value, 0);
  if (totalValue <= 0) return [];

  const totalArea = width * height;
  for (const entry of scaled) {
    entry.area = (entry.item.value / totalValue) * totalArea;
  }

  const result: TreemapLayoutEntry<T>[] = [];
  let x = 0;
  let y = 0;
  let remainingWidth = width;
  let remainingHeight = height;
  let row: ScaledItem<T>[] = [];

  function layoutRow(rowItems: readonly ScaledItem<T>[]): void {
    const rowArea = rowItems.reduce((total, entry) => total + entry.area, 0);
    if (rowArea <= 0) return;
    if (remainingWidth >= remainingHeight) {
      // 竖切一行（沿左侧排一列）
      const rowWidth = rowArea / remainingHeight;
      let cursorY = y;
      for (const entry of rowItems) {
        const entryHeight = entry.area / rowWidth;
        result.push({
          item: entry.item,
          index: entry.index,
          rect: clampRect(
            { x, y: cursorY, width: rowWidth, height: entryHeight },
            width,
            height,
          ),
        });
        cursorY += entryHeight;
      }
      x += rowWidth;
      remainingWidth -= rowWidth;
    } else {
      // 横切一行（沿顶部排一行）
      const rowHeight = rowArea / remainingWidth;
      let cursorX = x;
      for (const entry of rowItems) {
        const entryWidth = entry.area / rowHeight;
        result.push({
          item: entry.item,
          index: entry.index,
          rect: clampRect(
            { x: cursorX, y, width: entryWidth, height: rowHeight },
            width,
            height,
          ),
        });
        cursorX += entryWidth;
      }
      y += rowHeight;
      remainingHeight -= rowHeight;
    }
  }

  for (const entry of scaled) {
    const side = Math.min(remainingWidth, remainingHeight);
    if (
      row.length === 0 ||
      worstAspectRatio([...row, entry], side) <= worstAspectRatio(row, side)
    ) {
      row.push(entry);
    } else {
      layoutRow(row);
      row = [entry];
    }
  }
  if (row.length > 0) layoutRow(row);

  return result;
}

/** 浮点误差可能导致最后一条边轻微越界，统一钳回容器内 */
function clampRect(rect: TreemapRect, width: number, height: number): TreemapRect {
  const x = Math.max(0, Math.min(rect.x, width));
  const y = Math.max(0, Math.min(rect.y, height));
  return {
    x,
    y,
    width: Math.max(0, Math.min(rect.width, width - x)),
    height: Math.max(0, Math.min(rect.height, height - y)),
  };
}
