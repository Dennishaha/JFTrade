/**
 * 研究页视图共享的 entry 字段读取与格式化工具。
 * 字段名兼容思路沿用 ProductFeaturePanel 的列映射：同一语义尝试多个候选字段名，
 * 全部缺失时由 format* 系列输出 "--"。
 */

export function pickString(
  entry: Record<string, unknown>,
  keys: readonly string[],
): string {
  for (const key of keys) {
    const value = entry[key];
    if (typeof value === "string" && value.trim() !== "") return value;
  }
  return "";
}

export function pickNumber(
  entry: Record<string, unknown>,
  keys: readonly string[],
): number | null {
  for (const key of keys) {
    const value = entry[key];
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (typeof value === "string" && value.trim() !== "") {
      const parsed = Number(value);
      if (Number.isFinite(parsed)) return parsed;
    }
  }
  return null;
}

const numberFormatter = new Intl.NumberFormat("zh-CN", {
  maximumFractionDigits: 4,
});

export function formatPrice(value: number | null): string {
  if (value == null) return "--";
  return numberFormatter.format(value);
}

export function formatSigned(value: number | null, suffix = ""): string {
  if (value == null) return "--";
  const formatted = `${Math.abs(value).toFixed(2)}${suffix}`;
  if (value > 0) return `+${formatted}`;
  if (value < 0) return `-${formatted}`;
  return formatted;
}

/** 大数字格式化：万亿 / 亿 / 万 */
export function formatCompactNumber(value: number | null): string {
  if (value == null) return "--";
  const abs = Math.abs(value);
  if (abs >= 1e12) return `${(value / 1e12).toFixed(2)}万亿`;
  if (abs >= 1e8) return `${(value / 1e8).toFixed(2)}亿`;
  if (abs >= 1e4) return `${(value / 1e4).toFixed(2)}万`;
  return numberFormatter.format(value);
}

export function directionClass(value: number | null): "tv-up" | "tv-down" | "" {
  if (value == null || value === 0) return "";
  return value > 0 ? "tv-up" : "tv-down";
}

const AVATAR_COLORS = [
  "#2563eb",
  "#0d9488",
  "#7c3aed",
  "#c026d3",
  "#ea580c",
  "#65a30d",
  "#0891b2",
  "#be185d",
] as const;

/** 按名称 hash 取一组柔和色，用于首字 avatar / 机构 logo 块背景 */
export function hashColor(name: string): string {
  let hash = 0;
  for (let index = 0; index < name.length; index += 1) {
    hash = (hash * 31 + name.charCodeAt(index)) >>> 0;
  }
  return AVATAR_COLORS[hash % AVATAR_COLORS.length]!;
}

/**
 * 从 entry 的日期字段（date/reportDate/earningsDate/eventDate/time 等）提取
 * 本地 "yyyy-mm-dd" 日键。日期型字符串只取前 10 位，避免 UTC 解析的时区偏移。
 */
export function entryDayKey(
  entry: Record<string, unknown>,
  keys: readonly string[] = ["date", "reportDate", "earningsDate", "eventDate", "time"],
): string {
  for (const key of keys) {
    const value = entry[key];
    if (typeof value !== "string") continue;
    const match = value.match(/^(\d{4})-(\d{2})-(\d{2})/);
    if (match) return `${match[1]}-${match[2]}-${match[3]}`;
  }
  return "";
}

export function dayKeyOf(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}
