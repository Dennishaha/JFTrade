import type { StrategyVisualNodeDocument } from "@/types";

import {
  normalizeRiskRuleBlockProperties,
  normalizeStopLossBlockProperties,
} from "./strategyVisualBuilderCatalog";
import { renderVisualExpressionToPine } from "./strategyVisualBuilderExpressions";
import {
  entryPositionPolicyToSnakeCase,
  normalizeDecimal,
  normalizeEntryPositionPolicy,
  normalizeOrderSide,
  normalizeOrderType,
  normalizePineOrderAction,
  normalizePineRiskAllowEntryDirection,
  normalizeQuantityModeForSide,
} from "./strategyVisualBuilderScriptSupport";
import {
  formatNumber,
  toPineStringLiteral,
} from "./strategyVisualBuilderPineFormat";

export function buildOrderStatement(node: StrategyVisualNodeDocument): string {
  const side = normalizeOrderSide(node.properties.side);
  const action = inferPineOrderAction(node.properties.orderAction, side);
  const orderId = normalizePineOrderId(node.properties.orderId);
  if (action === "closeAll") {
    const closeAllArgs = [
      renderBooleanOrderArg("immediately", node.properties.immediately),
      renderStringOrderArg("comment", node.properties.comment),
      renderStringOrderArg("alert_message", node.properties.alert_message),
      renderBooleanOrderArg("disable_alert", node.properties.disable_alert),
    ].filter((arg) => arg !== "");
    return closeAllArgs.length === 0
      ? "strategy.close_all()"
      : `strategy.close_all(${closeAllArgs.join(", ")})`;
  }
  if (action === "cancel") {
    return `strategy.cancel(${toPineStringLiteral(orderId ?? "Long")})`;
  }
  if (action === "cancelAll") {
    return "strategy.cancel_all()";
  }
  if (action === "riskAllowEntryIn") {
    const direction = normalizePineRiskAllowEntryDirection(node.properties.riskAllowedDirection);
    const pineDirection = direction === "long"
      ? "strategy.direction.long"
      : direction === "short"
        ? "strategy.direction.short"
        : "strategy.direction.all";
    return `strategy.risk.allow_entry_in(${pineDirection})`;
  }

  const quantityMode = normalizeQuantityModeForSide(node.properties.quantityMode, side);
  const quantityValue = normalizeDecimal(node.properties.quantityValue, 100);
  const quantityOption = quantityMode === "equityPercent"
    ? `qty_percent=${formatNumber(quantityValue)}`
    : `qty=${buildPineQuantityExpression(quantityMode, quantityValue)}`;
  const orderType = normalizeOrderType(node.properties.orderType);
  const limitPrice = normalizeDecimal(node.properties.limitPrice, 0);
  const stopPrice = normalizeDecimal(node.properties.stopPrice, 0);
  const limitOption = orderType === "LIMIT" && (limitPrice > 0 || node.properties.limitPriceExpressionAst !== undefined)
    ? `, limit=${renderVisualExpressionToPine(node.properties.limitPriceExpressionAst, formatNumber(limitPrice))}`
    : "";
  const stopOption = stopPrice > 0 || node.properties.stopPriceExpressionAst !== undefined
    ? `, stop=${renderVisualExpressionToPine(node.properties.stopPriceExpressionAst, formatNumber(stopPrice))}`
    : "";

  const entryPolicy = normalizeEntryPositionPolicy(node.properties.entryPositionPolicy);
  const entryPolicyAnnotation = action === "entry" && entryPolicy !== "sameDirection"
    ? `// @entry_policy ${entryPositionPolicyToSnakeCase(entryPolicy)}\n`
    : "";

  if (action === "close") {
    const closeId = orderId ?? (side === "BUY_COVER" ? "Short" : "Long");
    const closeArgs = [
      quantityOption,
      renderOrderExpressionArg("limit", node.properties.limitPriceExpressionAst, limitPrice),
      renderOrderExpressionArg("stop", node.properties.stopPriceExpressionAst, stopPrice),
      renderStringOrderArg("comment", node.properties.comment),
      renderStringOrderArg("alert_message", node.properties.alert_message),
      renderBooleanOrderArg("immediately", node.properties.immediately),
      renderBooleanOrderArg("disable_alert", node.properties.disable_alert),
      renderRawOrderArg("when", node.properties.when),
    ].filter((arg) => arg !== "");
    return `strategy.close(${toPineStringLiteral(closeId)}, ${closeArgs.join(", ")})`;
  }
  const direction = side === "SELL_SHORT" || side === "SELL"
    ? "strategy.short"
    : "strategy.long";
  const defaultOrderId = direction === "strategy.short" ? "Short" : "Long";
  const functionName = action === "order" ? "strategy.order" : "strategy.entry";
  const orderArgs = [
    quantityOption,
    limitOption.slice(2),
    stopOption.slice(2),
    renderStringOrderArg("comment", node.properties.comment),
    renderStringOrderArg("alert_message", node.properties.alert_message),
    renderBooleanOrderArg("disable_alert", node.properties.disable_alert),
    renderRawOrderArg("when", node.properties.when),
  ].filter((arg) => arg !== "");
  return `${entryPolicyAnnotation}${functionName}(${[
    toPineStringLiteral(orderId ?? defaultOrderId),
    direction,
    ...orderArgs,
  ].join(", ")})`;
}

export function buildRiskRuleStatement(node: StrategyVisualNodeDocument): string {
  const properties = normalizeRiskRuleBlockProperties(node.properties ?? {});
  switch (properties.riskRuleType) {
    case "allowEntryIn": {
      const direction = normalizePineRiskAllowEntryDirection(properties.riskAllowedDirection);
      const pineDirection = direction === "long"
        ? "strategy.direction.long"
        : direction === "short"
          ? "strategy.direction.short"
          : "strategy.direction.all";
      return `strategy.risk.allow_entry_in(${pineDirection})`;
    }
    case "maxIntradayLoss":
    case "maxDrawdown": {
      const functionName = properties.riskRuleType === "maxIntradayLoss"
        ? "strategy.risk.max_intraday_loss"
        : "strategy.risk.max_drawdown";
      const args = [
        formatNumber(properties.riskValue ?? 10),
        properties.riskAmountType ?? "strategy.percent_of_equity",
        renderStringOrderArg("alert_message", properties.alert_message),
      ].filter((arg) => arg !== "");
      return `${functionName}(${args.join(", ")})`;
    }
    case "maxIntradayFilledOrders":
    case "maxConsLossDays": {
      const functionName = properties.riskRuleType === "maxIntradayFilledOrders"
        ? "strategy.risk.max_intraday_filled_orders"
        : "strategy.risk.max_cons_loss_days";
      const args = [
        formatNumber(properties.riskCount ?? 3),
        renderStringOrderArg("alert_message", properties.alert_message),
      ].filter((arg) => arg !== "");
      return `${functionName}(${args.join(", ")})`;
    }
    case "maxPositionSize":
      return `strategy.risk.max_position_size(${formatNumber(properties.riskContracts ?? 10)})`;
    default:
      return "strategy.risk.max_drawdown(10, strategy.percent_of_equity)";
  }
}

export function inferPineOrderAction(
  value: unknown,
  side: ReturnType<typeof normalizeOrderSide>,
): ReturnType<typeof normalizePineOrderAction> {
  if (typeof value === "string" && value.trim() !== "") {
    return normalizePineOrderAction(value);
  }
  return side === "SELL" || side === "BUY_COVER" ? "close" : "entry";
}

export function normalizePineOrderId(value: unknown): string | null {
  if (typeof value !== "string") {
    return null;
  }
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized === "" ? null : normalized;
}

export function buildPineQuantityExpression(
  quantityMode: Exclude<ReturnType<typeof normalizeQuantityModeForSide>, "equityPercent">,
  quantityValue: number,
): string {
  const value = formatNumber(quantityValue);
  switch (quantityMode) {
    case "amount":
      return `(${value} / close)`;
    case "shares":
    default:
      return value;
  }
}

export function renderOrderExpressionArg(
  name: string,
  expressionAst: unknown,
  fallbackNumber: number,
): string {
  if (fallbackNumber <= 0 && expressionAst === undefined) {
    return "";
  }
  return `${name}=${renderVisualExpressionToPine(expressionAst, formatNumber(fallbackNumber))}`;
}

export function renderStringOrderArg(name: string, value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  const normalized = value.replace(/[\r\n]+/g, " ").trim();
  return normalized === "" ? "" : `${name}=${toPineStringLiteral(normalized)}`;
}

export function renderRawOrderArg(name: string, value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  const normalized = value.trim();
  return normalized === "" ? "" : `${name}=${normalized}`;
}

export function renderBooleanOrderArg(name: string, value: unknown): string {
  if (typeof value === "boolean") {
    return `${name}=${value ? "true" : "false"}`;
  }
  if (typeof value !== "string") {
    return "";
  }
  const normalized = value.trim().toLowerCase();
  if (normalized !== "true" && normalized !== "false") {
    return "";
  }
  return `${name}=${normalized}`;
}

export function buildProtectStatements(node: StrategyVisualNodeDocument): string[] {
  const properties = normalizeStopLossBlockProperties(node.properties ?? {});
  if (
    (properties.windowPolicy ?? "continuous") !== "continuous" ||
    (properties.timeUnit ?? "day") !== "bar" ||
    (properties.timeValue ?? 1) !== 1
  ) {
    return [
      `runtime.error(${toPineStringLiteral("JFTrade Pine 暂不支持带时间窗口或交易时段感知的自动退出图块")})`,
    ];
  }

  const percentage = formatNumber(properties.percentage ?? 2);
  const profitTicks = properties.profitTicks === undefined ? null : formatNumber(properties.profitTicks);
  const lossTicks = properties.lossTicks === undefined ? null : formatNumber(properties.lossTicks);
  const quantityPercentage = properties.quantityPercentage ?? 100;
  const quantityOption = quantityPercentage > 0 && quantityPercentage < 100
    ? `, qty_percent=${formatNumber(quantityPercentage)}`
    : "";
  const explicitStopPrice = properties.stopPriceExpressionAst === undefined
    ? null
    : renderVisualExpressionToPine(properties.stopPriceExpressionAst, "close");
  const explicitTakeProfitPrice = properties.takeProfitPriceExpressionAst === undefined
    ? null
    : renderVisualExpressionToPine(properties.takeProfitPriceExpressionAst, "close");
  const explicitTrailingPrice = properties.trailingPriceExpressionAst === undefined
    ? null
    : renderVisualExpressionToPine(properties.trailingPriceExpressionAst, "close");
  const explicitTrailingOffset = properties.trailingOffsetExpressionAst === undefined
    ? explicitTrailingPrice
    : renderVisualExpressionToPine(properties.trailingOffsetExpressionAst, explicitTrailingPrice ?? "close");
  const preservedFromEntryId = properties.fromEntryMode === "auto"
    ? ""
    : (properties.fromEntryId ?? "").trim();
  const directions = preservedFromEntryId !== ""
    ? [properties.direction === "short" ? "short" : "long"]
    : properties.fromEntryMode === "auto"
    ? ["auto"]
    : properties.direction === "long"
    ? ["long"]
    : properties.direction === "short"
      ? ["short"]
      : ["long", "short"];
  return directions.map((direction) => {
    const entryId = preservedFromEntryId || (direction === "short" ? "Short" : "Long");
    const generatedExitId = direction === "auto" ? `Auto ${properties.mode ?? "stopLoss"}` : `${entryId} ${properties.mode ?? "stopLoss"}`;
    const exitId = directions.length === 1 && (properties.exitId ?? "").trim() !== ""
      ? properties.exitId!.trim()
      : generatedExitId;
    const fromEntryArg = direction === "auto" ? "" : `, ${toPineStringLiteral(entryId)}`;
    const metadataArgs = buildProtectMetadataArgs(properties);
    switch (properties.mode) {
      case "takeProfit":
        if (explicitTakeProfitPrice !== null) {
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, limit=${explicitTakeProfitPrice}${quantityOption}${metadataArgs})`;
        }
        if (profitTicks !== null) {
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, profit=${profitTicks}${quantityOption}${metadataArgs})`;
        }
        return direction === "short"
          ? `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, limit=close * (1 - ${percentage} / 100)${quantityOption}${metadataArgs})`
          : `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, limit=close * (1 + ${percentage} / 100)${quantityOption}${metadataArgs})`;
      case "trailingStop":
        if (explicitTrailingPrice !== null) {
          const trailingArg = properties.trailingPriceMode === "price" ? "trail_price" : "trail_points";
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, ${trailingArg}=${explicitTrailingPrice}, trail_offset=${explicitTrailingOffset ?? explicitTrailingPrice}${quantityOption}${metadataArgs})`;
        }
        return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, trail_points=close * ${percentage} / 100, trail_offset=close * ${percentage} / 100${quantityOption}${metadataArgs})`;
      case "bracketExit": {
        const takeProfitPercentage = formatNumber(properties.takeProfitPercentage ?? 4);
        const bracketArgs = [
          explicitStopPrice === null ? "" : `stop=${explicitStopPrice}`,
          explicitTakeProfitPrice === null ? "" : `limit=${explicitTakeProfitPrice}`,
          lossTicks === null ? "" : `loss=${lossTicks}`,
          profitTicks === null ? "" : `profit=${profitTicks}`,
        ].filter((arg) => arg !== "");
        if (bracketArgs.length > 0) {
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, ${bracketArgs.join(", ")}${quantityOption}${metadataArgs})`;
        }
        return direction === "short"
          ? `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, stop=close * (1 + ${percentage} / 100), limit=close * (1 - ${takeProfitPercentage} / 100)${quantityOption}${metadataArgs})`
          : `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, stop=close * (1 - ${percentage} / 100), limit=close * (1 + ${takeProfitPercentage} / 100)${quantityOption}${metadataArgs})`;
      }
      case "stopLoss":
      default:
        if (explicitStopPrice !== null) {
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, stop=${explicitStopPrice}${quantityOption}${metadataArgs})`;
        }
        if (lossTicks !== null) {
          return `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, loss=${lossTicks}${quantityOption}${metadataArgs})`;
        }
        return direction === "short"
          ? `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, stop=close * (1 + ${percentage} / 100)${quantityOption}${metadataArgs})`
          : `strategy.exit(${toPineStringLiteral(exitId)}${fromEntryArg}, stop=close * (1 - ${percentage} / 100)${quantityOption}${metadataArgs})`;
    }
  });
}

export function buildProtectMetadataArgs(
  properties: ReturnType<typeof normalizeStopLossBlockProperties>,
): string {
  const args = [
    renderStringOrderArg("comment", properties.comment),
    renderStringOrderArg("comment_profit", properties.comment_profit),
    renderStringOrderArg("comment_loss", properties.comment_loss),
    renderStringOrderArg("comment_trailing", properties.comment_trailing),
    renderStringOrderArg("alert_message", properties.alert_message),
    renderStringOrderArg("alert_profit", properties.alert_profit),
    renderStringOrderArg("alert_loss", properties.alert_loss),
    renderStringOrderArg("alert_trailing", properties.alert_trailing),
    renderBooleanOrderArg("disable_alert", properties.disable_alert),
    renderRawOrderArg("when", properties.when),
  ].filter((arg) => arg !== "");
  return args.length === 0 ? "" : `, ${args.join(", ")}`;
}
