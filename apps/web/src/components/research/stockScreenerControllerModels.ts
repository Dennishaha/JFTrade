import type {
  StockScreenCatalog,
  StockScreenColumn,
  StockScreenEntry,
  StockScreenPreset,
} from "./stockScreenTypes";

export interface StockScreenerControllerProps {
  market: string;
  brokerId: string;
  initialPresetId: string;
  active: boolean;
}

export interface StockScreenerControllerEmit {
  (event: "select", entry: StockScreenEntry): void;
  (event: "open", entry: StockScreenEntry): void;
  (event: "presetChange", presetId: string): void;
  (
    event: "contextChange",
    context: { market: string; brokerId?: string },
  ): void;
}

export type PendingDraftAction =
  | { kind: "preset"; preset: StockScreenPreset }
  | { kind: "new" };

export type StockScreenerStatus =
  | "running"
  | "error"
  | "待更新"
  | "有未保存修改"
  | "已保存"
  | "未保存";

export function stockScreenerStatus(input: {
  loading: boolean;
  hasQueryError: boolean;
  resultStale: boolean;
  draftDirty: boolean;
  selectedPresetId: string;
}): StockScreenerStatus {
  if (input.loading) return "running";
  if (input.hasQueryError) return "error";
  if (input.resultStale) return "待更新";
  if (input.draftDirty) return "有未保存修改";
  if (input.selectedPresetId) return "已保存";
  return "未保存";
}

export function stockScreenerStatusLabel(status: StockScreenerStatus): string {
  switch (status) {
    case "running":
      return "执行中";
    case "error":
      return "需要修正";
    case "待更新":
      return "结果待更新";
    case "有未保存修改":
      return "有未保存修改";
    case "已保存":
      return "已保存";
    default:
      return "未保存";
  }
}

export function pendingDraftActionLabel(
  action: PendingDraftAction | null,
): string {
  if (!action) return "";
  return action.kind === "preset" ? `切换到“${action.preset.name}”` : "新建策略";
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function validationErrorFrom(
  error: unknown,
): { path: string; message: string } | null {
  const message = errorMessage(error);
  const match = message.match(
    /^((?:conditions|columns|sorts)\[\d+\](?:\.[A-Za-z][A-Za-z0-9]*)+):\s*(.+)$/,
  );
  if (!match) return null;
  const path = match[1]!
    .replaceAll(/\[(\d+)\]/g, ".$1")
    .replace(".factor.params.", ".params.")
    .replace(".factor.factorKey", ".factor")
    .replace(".secondFactor.factorKey", ".secondFactor");
  return { path, message: match[2]! };
}

export function defaultColumnsForCatalog(
  catalog: StockScreenCatalog,
): StockScreenColumn[] {
  const defaultFactorKeys = new Set([
    "basic.code",
    "basic.name",
    "simple.price",
    "simple.market_cap",
  ]);
  return catalog.factors
    .filter(
      (factor) =>
        defaultFactorKeys.has(factor.key) &&
        factor.retrieve &&
        factor.availability !== "unsupported",
    )
    .map((factor, index) => ({
      factor: factor.key,
      factorKey: factor.key,
      instanceId: `default-${factor.key}`,
      columnId: `column-${factor.key}-${index}`,
    }));
}
