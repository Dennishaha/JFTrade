import type {
  PineV6WorkflowBlock,
  PineV6WorkflowBlockKind,
  PineV6WorkflowDocument,
  PineV6WorkflowInput,
} from "@/contracts";

import { createPineV6WorkflowBlock } from "./pineV6Workflow";
import type { PineSourceBlock } from "./pineSourceStructureIndex";
import { SOURCE_EXTRA_ARGS_KEY } from "./pineSourceStructureText";

const orderStringArgNames = new Set([
  "alert_loss",
  "alert_message",
  "alert_profit",
  "alert_trailing",
  "comment",
  "comment_loss",
  "comment_profit",
  "comment_trailing",
  "from_entry",
  "id",
  "oca_name",
]);

export function renderBlockToSource(block: PineSourceBlock): string {
  switch (block.match.type) {
    case "strategy":
      return renderStrategyDeclaration(block.match.declaration);
    case "input":
      return renderInput(block.match.input, block.depth);
    case "instruction":
      return renderInstructionBlock(block.match.block, block.depth);
    default:
      return block.raw;
  }
}

export function renderDefaultSourceBlock(kind: PineV6WorkflowBlockKind, indentLevel: number): string {
  return renderInstructionBlock(createPineV6WorkflowBlock(kind), indentLevel);
}

function renderStrategyDeclaration(declaration: Partial<PineV6WorkflowDocument["declaration"]>): string {
  const title = quoteString(String(declaration.title ?? "Pine v6 策略"));
  const args = [
    title,
    `overlay=${declaration.overlay === false ? "false" : "true"}`,
    optionalNumberArg("initial_capital", declaration.initialCapital),
    optionalRawArg("currency", declaration.currency),
    optionalNumberArg("pyramiding", declaration.pyramiding),
    optionalRawArg("default_qty_type", declaration.defaultQtyType),
    optionalNumberArg("default_qty_value", declaration.defaultQtyValue),
    `calc_on_every_tick=${declaration.calcOnEveryTick === true ? "true" : "false"}`,
    `process_orders_on_close=${declaration.processOrdersOnClose === true ? "true" : "false"}`,
    ...readSourceExtraArgs(declaration),
  ].filter((value): value is string => value !== null && value !== "");
  return `strategy(${args.join(", ")})`;
}

function renderInput(input: PineV6WorkflowInput, indentLevel: number): string {
  const indent = "    ".repeat(indentLevel);
  const title = quoteString(input.title || input.name);
  const value = input.type === "string"
    ? quoteString(input.defaultValue)
    : input.type === "timeframe"
      ? quoteString(input.defaultValue || defaultInputValue(input.type))
      : input.defaultValue || defaultInputValue(input.type);
  return `${indent}${sanitizeIdentifier(input.name, "inputValue")} = input.${input.type}(${value}, ${title})`;
}

function renderInstructionBlock(block: PineV6WorkflowBlock, indentLevel: number): string {
  const indent = "    ".repeat(indentLevel);
  const params = block.params;
  switch (block.kind) {
    case "series_assign":
      return `${indent}${sanitizeIdentifier(readString(params.name), "seriesValue")} = ${readExpression(params.expression, "close")}`;
    case "var_state":
      return `${indent}var ${sanitizeIdentifier(readString(params.name), "stateValue")} = ${readExpression(params.initial, "na")}`;
    case "if":
      return `${indent}if ${readExpression(params.condition, "false")}`;
    case "request_security":
      return `${indent}${sanitizeIdentifier(readString(params.name), "mtfValue")} = request.security(${renderCallArgs([
        quoteString(readString(params.symbol) || "syminfo.tickerid"),
        quoteString(readString(params.timeframe) || "D"),
        readExpression(params.expression, "close"),
        ...readSourceExtraArgs(params),
      ])})`;
    case "array_op":
      return renderArrayOperation(params, indent);
    case "strategy_entry":
      return `${indent}strategy.entry(${renderCallArgs([
        quoteString(readString(params.id) || "Long"),
        normalizeDirection(readString(params.direction), "strategy.long"),
        ...renderNamedOrderArgs(params, ["qty", "qty_percent", "limit", "stop", "oca_name", "oca_type", "comment", "alert_message", "disable_alert", "when"]),
        ...readSourceExtraArgs(params),
      ])})`;
    case "strategy_exit":
      return `${indent}strategy.exit(${renderCallArgs([
        quoteString(readString(params.id) || "Exit"),
        optionalStringArg("from_entry", params.from_entry),
        ...renderNamedOrderArgs(params, [
          "qty",
          "qty_percent",
          "profit",
          "limit",
          "loss",
          "stop",
          "trail_price",
          "trail_points",
          "trail_offset",
          "oca_name",
          "comment",
          "comment_profit",
          "comment_loss",
          "comment_trailing",
          "alert_message",
          "alert_profit",
          "alert_loss",
          "alert_trailing",
          "disable_alert",
          "when",
        ]),
        ...readSourceExtraArgs(params),
      ])})`;
    case "strategy_order":
      return `${indent}strategy.order(${renderCallArgs([
        quoteString(readString(params.id) || "Order"),
        normalizeDirection(readString(params.direction), "strategy.long"),
        ...renderNamedOrderArgs(params, ["qty", "qty_percent", "limit", "stop", "oca_name", "oca_type", "comment", "alert_message", "disable_alert", "when"]),
        ...readSourceExtraArgs(params),
      ])})`;
    case "strategy_close":
      return `${indent}strategy.close(${renderCallArgs([
        quoteString(readString(params.id) || "Long"),
        ...renderNamedOrderArgs(params, ["qty", "qty_percent", "limit", "stop", "comment", "alert_message", "immediately", "disable_alert", "when"]),
        ...readSourceExtraArgs(params),
      ])})`;
    case "strategy_close_all":
      return `${indent}strategy.close_all(${renderCallArgs([
        ...renderNamedOrderArgs(params, ["immediately", "comment", "alert_message", "disable_alert"]),
        ...readSourceExtraArgs(params),
      ])})`;
    case "strategy_cancel":
      return `${indent}strategy.cancel(${renderCallArgs([
        quoteString(readString(params.id) || "Order"),
        ...readSourceExtraArgs(params),
      ])})`;
    case "strategy_cancel_all":
      return `${indent}strategy.cancel_all(${renderCallArgs(readSourceExtraArgs(params))})`;
    case "strategy_risk_allow_entry_in":
      return `${indent}strategy.risk.allow_entry_in(${normalizeRiskDirection(readString(params.direction))})`;
    case "strategy_risk_max_drawdown":
      return `${indent}strategy.risk.max_drawdown(${renderCallArgs([
        readExpression(params.value, "10"),
        normalizeRiskAmountType(readString(params.type)),
        riskAlertMessageArg(params.alert_message),
        ...readSourceExtraArgs(params),
      ])})`;
    case "strategy_risk_max_intraday_loss":
      return `${indent}strategy.risk.max_intraday_loss(${renderCallArgs([
        readExpression(params.value, "10"),
        normalizeRiskAmountType(readString(params.type)),
        riskAlertMessageArg(params.alert_message),
        ...readSourceExtraArgs(params),
      ])})`;
    case "strategy_risk_max_intraday_filled_orders":
      return `${indent}strategy.risk.max_intraday_filled_orders(${renderCallArgs([
        readExpression(params.count, "10"),
        riskAlertMessageArg(params.alert_message),
        ...readSourceExtraArgs(params),
      ])})`;
    case "strategy_risk_max_position_size":
      return `${indent}strategy.risk.max_position_size(${renderCallArgs([
        readExpression(params.contracts, "1"),
        ...readSourceExtraArgs(params),
      ])})`;
    case "strategy_risk_max_cons_loss_days":
      return `${indent}strategy.risk.max_cons_loss_days(${renderCallArgs([
        readExpression(params.count, "3"),
        riskAlertMessageArg(params.alert_message),
        ...readSourceExtraArgs(params),
      ])})`;
    case "plot":
      return `${indent}plot(${renderCallArgs([
        readExpression(params.series, "close"),
        optionalRawArg("title", quoteString(readString(params.title))),
        optionalRawArg("color", params.color),
        ...readSourceExtraArgs(params),
      ])})`;
    case "alertcondition":
      return `${indent}alertcondition(${renderCallArgs([
        readExpression(params.condition, "false"),
        optionalRawArg("title", quoteString(readString(params.title) || "提醒")),
        optionalRawArg("message", quoteString(readString(params.message) || "Pine v6 工作流提醒")),
        ...readSourceExtraArgs(params),
      ])})`;
    case "log":
      return `${indent}log.info(${quoteString(readString(params.message) || "Pine v6 工作流日志")})`;
    default:
      return `${indent}${block.kind}`;
  }
}

function renderArrayOperation(params: Record<string, unknown>, indent: string): string {
  const mode = readString(params.mode) || "new_float";
  const name = sanitizeIdentifier(readString(params.name), "values");
  if (mode === "push") return `${indent}array.push(${name}, ${readExpression(params.value, "close")})`;
  if (mode === "median") return `${indent}${sanitizeIdentifier(readString(params.output), "medianValue")} = array.median(${name})`;
  return `${indent}var ${name} = array.new_float()`;
}

function defaultInputValue(type: PineV6WorkflowInput["type"]): string {
  if (type === "float") return "0.0";
  if (type === "bool") return "false";
  if (type === "source") return "close";
  if (type === "time") return "timestamp(2026, 1, 1, 0, 0)";
  if (type === "timeframe") return "D";
  if (type === "color") return "color.blue";
  return "1";
}

function sanitizeIdentifier(value: string, fallback: string): string {
  const candidate = value.trim().replace(/[^A-Za-z0-9_]/g, "_");
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(candidate) ? candidate : fallback;
}

function renderCallArgs(values: Array<string | null>): string {
  return values.filter((value): value is string => value !== null && value !== "").join(", ");
}

function readSourceExtraArgs(source: Record<string, unknown>): string[] {
  const value = source[SOURCE_EXTRA_ARGS_KEY];
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string" && item.trim() !== "") : [];
}

function optionalNumberArg(name: string, value: unknown): string | null {
  const numeric = typeof value === "number" ? value : Number(value);
  return Number.isFinite(numeric) ? `${name}=${numeric}` : null;
}

function optionalRawArg(name: string, value: unknown): string | null {
  const text = readString(value);
  return text === "" ? null : `${name}=${text}`;
}

function optionalStringArg(name: string, value: unknown): string | null {
  const text = readString(value);
  return text === "" ? null : `${name}=${quoteString(text)}`;
}

function renderNamedOrderArgs(params: Record<string, unknown>, names: string[]): Array<string | null> {
  return names.map((name) => optionalRawArg(name, orderArgValue(name, params[name])));
}

function riskAlertMessageArg(value: unknown): string | null {
  const text = readString(value);
  return text === "" ? null : `alert_message=${quoteString(text)}`;
}

function orderArgValue(name: string, value: unknown): string {
  const text = readString(value);
  if (text === "") return "";
  return orderStringArgNames.has(name) ? quoteString(text) : text;
}

function quoteString(value: string): string {
  return `"${value.replaceAll("\\", "\\\\").replaceAll("\"", "\\\"")}"`;
}

function normalizeDirection(value: string, fallback: string): string {
  return value === "strategy.short" || value === "strategy.long" ? value : fallback;
}

function normalizeRiskDirection(value: string): string {
  return value === "strategy.direction.long" ||
    value === "strategy.direction.short" ||
    value === "strategy.direction.all"
    ? value
    : "strategy.direction.all";
}

function normalizeRiskAmountType(value: string): string {
  return value === "strategy.percent_of_equity" || value === "strategy.cash"
    ? value
    : "strategy.percent_of_equity";
}

function readExpression(value: unknown, fallback: string): string {
  const text = readString(value);
  return text === "" ? fallback : text;
}

function readString(value: unknown): string {
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return typeof value === "string" ? value.trim() : "";
}
