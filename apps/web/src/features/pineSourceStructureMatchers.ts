import type {
  PineV6WorkflowBlock,
  PineV6WorkflowBlockKind,
  PineV6WorkflowDocument,
  PineV6WorkflowInput,
} from "@/contracts";

import { createPineV6WorkflowBlock, createWorkflowId } from "./pineV6Workflow";
import { SOURCE_EXTRA_ARGS_KEY, splitCallArgs, unquote } from "./pineSourceStructureText";

export function parseStrategyDeclaration(text: string): Partial<PineV6WorkflowDocument["declaration"]> {
  const args = splitCallArgs(readCallSummary(text, "strategy"));
  return {
    title: unquote(args[0] ?? "Pine v6 策略"),
    overlay: readNamedBool(args, "overlay", true),
    initialCapital: readNamedNumber(args, "initial_capital", null),
    currency: readNamedRaw(args, "currency", ""),
    pyramiding: readNamedNumber(args, "pyramiding", 0),
    defaultQtyType: readNamedRaw(args, "default_qty_type", "strategy.percent_of_equity"),
    defaultQtyValue: readNamedNumber(args, "default_qty_value", 10),
    calcOnEveryTick: readNamedBool(args, "calc_on_every_tick", false),
    processOrdersOnClose: readNamedBool(args, "process_orders_on_close", false),
    [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set([
      "overlay",
      "initial_capital",
      "currency",
      "pyramiding",
      "default_qty_type",
      "default_qty_value",
      "calc_on_every_tick",
      "process_orders_on_close",
    ]), 1),
  } as Partial<PineV6WorkflowDocument["declaration"]>;
}

export function parseInputLine(text: string): PineV6WorkflowInput | null {
  const match = text.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*input\.(int|float|bool|string|source|time|timeframe|color)\s*\((.*)\)$/);
  if (match === null) {
    return null;
  }
  const args = splitCallArgs(match[3] ?? "");
  return {
    id: `source-input-${match[1]}`,
    name: match[1] ?? "inputValue",
    type: inputType(match[2] ?? "int"),
    title: unquote(args[1] ?? match[1] ?? "输入参数"),
    defaultValue: unquote(args[0] ?? ""),
  };
}

export function parseRequestSecurityLine(text: string): PineV6WorkflowBlock | null {
  const match = text.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*request\.security\s*\((.*)\)$/);
  if (match === null) return null;
  const args = splitCallArgs(match[2] ?? "");
  if (args.length < 3) return null;
  return makeBlock("request_security", {
    name: match[1] ?? "mtfValue",
    symbol: unquote(args[0] ?? "syminfo.tickerid"),
    timeframe: unquote(args[1] ?? "D"),
    expression: args[2] ?? "close",
    [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(), 3),
  });
}

export function parseOrderLine(text: string): PineV6WorkflowBlock | null {
  const riskMatch = text.match(/^strategy\.risk\.allow_entry_in\s*\((.*)\)$/);
  if (riskMatch !== null) {
    const args = splitCallArgs(riskMatch[1] ?? "");
    return makeBlock("strategy_risk_allow_entry_in", {
      direction: normalizeRiskDirection(args[0] ?? "strategy.direction.all"),
    });
  }

  const riskAmountMatch = text.match(/^strategy\.risk\.(max_drawdown|max_intraday_loss)\s*\((.*)\)$/);
  if (riskAmountMatch !== null) {
    const fn = riskAmountMatch[1] ?? "max_drawdown";
    const args = splitCallArgs(riskAmountMatch[2] ?? "");
    return makeBlock(fn === "max_drawdown" ? "strategy_risk_max_drawdown" : "strategy_risk_max_intraday_loss", {
      value: readNamedRaw(args, "value", args[0] ?? "10"),
      type: readNamedRaw(args, "type", args[1] ?? "strategy.percent_of_equity"),
      alert_message: unquote(readNamedRaw(args, "alert_message", args[2] ?? "")),
      [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(["value", "type", "alert_message"]), 3),
    });
  }

  const riskCountMatch = text.match(/^strategy\.risk\.(max_intraday_filled_orders|max_cons_loss_days)\s*\((.*)\)$/);
  if (riskCountMatch !== null) {
    const fn = riskCountMatch[1] ?? "max_intraday_filled_orders";
    const args = splitCallArgs(riskCountMatch[2] ?? "");
    return makeBlock(
      fn === "max_intraday_filled_orders"
        ? "strategy_risk_max_intraday_filled_orders"
        : "strategy_risk_max_cons_loss_days",
      {
        count: readNamedRaw(args, "count", args[0] ?? (fn === "max_cons_loss_days" ? "3" : "10")),
        alert_message: unquote(readNamedRaw(args, "alert_message", args[1] ?? "")),
        [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(["count", "alert_message"]), 2),
      },
    );
  }

  const riskPositionMatch = text.match(/^strategy\.risk\.max_position_size\s*\((.*)\)$/);
  if (riskPositionMatch !== null) {
    const args = splitCallArgs(riskPositionMatch[1] ?? "");
    return makeBlock("strategy_risk_max_position_size", {
      contracts: readNamedRaw(args, "contracts", args[0] ?? "1"),
      [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(["contracts"]), 1),
    });
  }

  const match = text.match(/^strategy\.(entry|order|exit|close|close_all|cancel|cancel_all)\s*\((.*)\)$/);
  if (match === null) return null;
  const fn = match[1] ?? "";
  const args = splitCallArgs(match[2] ?? "");
  if (fn === "entry" || fn === "order") {
    return makeBlock(fn === "entry" ? "strategy_entry" : "strategy_order", {
      id: unquote(args[0] ?? (fn === "entry" ? "Long" : "Order")),
      direction: args[1] ?? "strategy.long",
      ...readNamedArgs(args, ["qty", "qty_percent", "limit", "stop", "oca_name", "oca_type", "comment", "alert_message", "disable_alert", "when"]),
      [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(["qty", "qty_percent", "limit", "stop", "oca_name", "oca_type", "comment", "alert_message", "disable_alert", "when"]), 2),
    });
  }
  if (fn === "exit") {
    return makeBlock("strategy_exit", {
      id: unquote(args[0] ?? "Exit"),
      from_entry: unquote(readNamedRaw(args, "from_entry", "Long")),
      ...readNamedArgs(args, [
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
      [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set([
        "from_entry",
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
      ]), 1),
    });
  }
  if (fn === "close_all") {
    return makeBlock("strategy_close_all", {
      immediately: readNamedRaw(args, "immediately", args[0] ?? ""),
      comment: unquote(readNamedRaw(args, "comment", args[1] ?? "")),
      alert_message: unquote(readNamedRaw(args, "alert_message", args[2] ?? "")),
      disable_alert: readNamedRaw(args, "disable_alert", args[3] ?? ""),
      ...readNamedArgs(args, ["immediately", "comment", "alert_message", "disable_alert"]),
      [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(["immediately", "comment", "alert_message", "disable_alert"]), 0),
    });
  }
  if (fn === "cancel") {
    return makeBlock("strategy_cancel", {
      id: unquote(args[0] ?? "Order"),
      [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(), 1),
    });
  }
  if (fn === "cancel_all") {
    return makeBlock("strategy_cancel_all", {
      [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(), 0),
    });
  }
  return makeBlock("strategy_close", {
    id: unquote(args[0] ?? "Long"),
    ...readNamedArgs(args, ["qty", "qty_percent", "comment", "alert_message", "immediately", "disable_alert", "when"]),
    [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(["qty", "qty_percent", "comment", "alert_message", "immediately", "disable_alert", "when"]), 1),
  });
}

export function parseVisualLine(text: string): PineV6WorkflowBlock | null {
  const plotMatch = text.match(/^plot\s*\((.*)\)$/);
  if (plotMatch !== null) {
    const args = splitCallArgs(plotMatch[1] ?? "");
    return makeBlock("plot", {
      series: args[0] ?? "close",
      title: unquote(readNamedRaw(args, "title", args[1] ?? "")),
      color: readNamedRaw(args, "color", args[2] ?? ""),
      [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(["title", "color"]), 3),
    });
  }
  const alertMatch = text.match(/^alertcondition\s*\((.*)\)$/);
  if (alertMatch !== null) {
    const args = splitCallArgs(alertMatch[1] ?? "");
    return makeBlock("alertcondition", {
      condition: args[0] ?? "false",
      title: unquote(readNamedRaw(args, "title", args[1] ?? "提醒")),
      message: unquote(readNamedRaw(args, "message", args[2] ?? "Pine v6 工作流提醒")),
      [SOURCE_EXTRA_ARGS_KEY]: collectExtraNamedArgs(args, new Set(["title", "message"]), 3),
    });
  }
  return null;
}

export function parseLogLine(text: string): PineV6WorkflowBlock | null {
  const match = text.match(/^log\.info\s*\((.*)\)$/);
  if (match === null) return null;
  return makeBlock("log", { message: unquote(match[1] ?? "Pine v6 工作流日志") });
}

export function parseCollectionLine(text: string): PineV6WorkflowBlock | null {
  const newMatch = text.match(/^var\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*array\.new_float\s*\(\s*\)$/);
  if (newMatch !== null) return makeBlock("array_op", { name: newMatch[1] ?? "values", mode: "new_float" });
  const pushMatch = text.match(/^array\.push\s*\(([A-Za-z_][A-Za-z0-9_]*),\s*(.*)\)$/);
  if (pushMatch !== null) return makeBlock("array_op", { name: pushMatch[1] ?? "values", mode: "push", value: pushMatch[2] ?? "close" });
  const medianMatch = text.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*array\.median\s*\(([A-Za-z_][A-Za-z0-9_]*)\)$/);
  if (medianMatch !== null) return makeBlock("array_op", { name: medianMatch[2] ?? "values", mode: "median", output: medianMatch[1] ?? "medianValue" });
  return null;
}

export function parseAssignmentLine(text: string): PineV6WorkflowBlock | null {
  const varMatch = text.match(/^var\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/);
  if (varMatch !== null) return makeBlock("var_state", { name: varMatch[1] ?? "stateValue", initial: varMatch[2] ?? "na" });
  const match = text.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/);
  if (match === null || (match[2] ?? "").startsWith("input.")) return null;
  return makeBlock("series_assign", { name: match[1] ?? "seriesValue", expression: match[2] ?? "close" });
}

export function orderLabelFromBlock(kind: PineV6WorkflowBlockKind): string {
  if (kind === "strategy_entry") return "入场订单";
  if (kind === "strategy_exit") return "退出订单";
  if (kind === "strategy_close") return "平仓指令";
  if (kind === "strategy_close_all") return "全部平仓";
  if (kind === "strategy_cancel") return "撤销订单";
  if (kind === "strategy_cancel_all") return "撤销全部订单";
  if (kind === "strategy_risk_allow_entry_in") return "允许入场方向";
  if (kind === "strategy_risk_max_drawdown") return "最大回撤风控";
  if (kind === "strategy_risk_max_intraday_loss") return "日内最大亏损";
  if (kind === "strategy_risk_max_intraday_filled_orders") return "日内成交上限";
  if (kind === "strategy_risk_max_position_size") return "最大持仓";
  if (kind === "strategy_risk_max_cons_loss_days") return "连续亏损天数";
  return "通用订单";
}

export function visualLabelFromBlock(kind: PineV6WorkflowBlockKind): string {
  if (kind === "alertcondition") return "提醒条件";
  return "绘图输出";
}

export function readCallSummary(text: string, callName: string): string {
  return text.replace(new RegExp(`^${callName}\\s*\\(`), "").replace(/\)\s*$/, "");
}

export function makeBlock(kind: PineV6WorkflowBlockKind, params: Record<string, unknown>): PineV6WorkflowBlock {
  const block = createPineV6WorkflowBlock(kind);
  return {
    ...block,
    id: `source-${kind}-${createWorkflowId("block")}`,
    params: { ...block.params, ...params },
  };
}

function readNamedRaw(args: string[], name: string, fallback: string): string {
  for (const arg of args) {
    const named = readNamedArg(arg);
    if (named?.name === name) {
      return named.value;
    }
  }
  return fallback;
}

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

function readNamedArgs(args: string[], names: string[]): Record<string, string> {
  return Object.fromEntries(
    names.map((name) => [name, readNamedOrderArg(args, name)]).filter(([, value]) => value !== ""),
  );
}

function readNamedOrderArg(args: string[], name: string): string {
  const raw = readNamedRaw(args, name, "");
  return orderStringArgNames.has(name) ? unquote(raw) : raw;
}

function collectExtraNamedArgs(args: string[], consumedNames: Set<string>, positionalCount: number): string[] {
  return args.slice(positionalCount).filter((arg) => {
    const named = readNamedArg(arg);
    return named !== null && !consumedNames.has(named.name);
  });
}

function readNamedArg(arg: string): { name: string; value: string } | null {
  const match = arg.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/);
  if (match === null) return null;
  return { name: match[1] ?? "", value: match[2]?.trim() ?? "" };
}

function readNamedNumber(args: string[], name: string, fallback: number | null): number | null {
  const raw = readNamedRaw(args, name, "");
  if (raw === "") return fallback;
  const numeric = Number(raw);
  return Number.isFinite(numeric) ? numeric : fallback;
}

function readNamedBool(args: string[], name: string, fallback: boolean): boolean {
  const raw = readNamedRaw(args, name, "");
  if (raw === "true") return true;
  if (raw === "false") return false;
  return fallback;
}

function inputType(value: string): PineV6WorkflowInput["type"] {
  return value === "float" ||
    value === "bool" ||
    value === "string" ||
    value === "source" ||
    value === "time" ||
    value === "timeframe" ||
    value === "color"
    ? value
    : "int";
}

function normalizeRiskDirection(value: string): string {
  return value === "strategy.direction.long" ||
    value === "strategy.direction.short" ||
    value === "strategy.direction.all"
    ? value
    : "strategy.direction.all";
}
