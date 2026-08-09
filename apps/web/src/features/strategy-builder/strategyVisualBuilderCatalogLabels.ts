import type { TradingSessionScope } from "./strategyVisualBuilderCatalog";
import { normalizeDayOfWeek } from "./strategyVisualBuilderCatalogNormalization";

export function dayOfWeekLabel(value: number): string {
  switch (normalizeDayOfWeek(value, 2)) {
    case 1:
      return "周日";
    case 2:
      return "周一";
    case 3:
      return "周二";
    case 4:
      return "周三";
    case 5:
      return "周四";
    case 6:
      return "周五";
    case 7:
    default:
      return "周六";
  }
}

export function sessionScopeLabel(value: TradingSessionScope): string {
  switch (value) {
    case "premarket":
      return "盘前";
    case "postmarket":
      return "盘后";
    case "market":
    default:
      return "常规交易时段";
  }
}
