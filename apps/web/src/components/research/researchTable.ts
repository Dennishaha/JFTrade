export interface ResearchTableColumn {
  key: string;
  label: string;
  align?: "left" | "right" | "center";
  width?: string;
  value: (entry: Record<string, unknown>) => unknown;
  format?: (value: unknown, entry: Record<string, unknown>) => string;
  className?: (
    value: unknown,
    entry: Record<string, unknown>,
  ) => string | undefined;
}

export function formatResearchCell(value: unknown): string {
  if (value == null || value === "") return "--";
  if (typeof value === "number") {
    return new Intl.NumberFormat("zh-CN", {
      maximumFractionDigits: 4,
    }).format(value);
  }
  if (typeof value === "boolean") return value ? "是" : "否";
  if (typeof value === "string") return value;
  return "--";
}
