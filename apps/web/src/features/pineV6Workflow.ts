import type {
  PineV6WorkflowBlock,
  PineV6WorkflowBlockKind,
  PineV6WorkflowDeclaration,
  PineV6WorkflowDocument,
  PineV6WorkflowInput,
  PineV6WorkflowRuntimeBindingDraft,
  StrategyExecutionMode,
  StrategyRuntimeRiskSettings,
} from "@/contracts";

export interface PineV6WorkflowDiagnostic {
  blockId?: string;
  severity: "info" | "warning" | "error";
  code: string;
  message: string;
}

export const PINE_V6_WORKFLOW_ENGINE = "pine-v6-workflow" as const;

export const PINE_V6_BLOCK_KINDS: Array<{
  kind: PineV6WorkflowBlockKind;
  label: string;
  description: string;
}> = [
  { kind: "series_assign", label: "序列赋值", description: "声明或更新 Pine 序列变量" },
  { kind: "var_state", label: "状态变量", description: "声明跨 K 线保留的 var 状态" },
  { kind: "if", label: "条件分支", description: "Pine 条件分支，包含 then / else" },
  { kind: "request_security", label: "跨周期请求", description: "请求高周期或其他标的序列" },
  { kind: "array_op", label: "数组操作", description: "数组初始化、push 或统计" },
  { kind: "strategy_entry", label: "入场订单", description: "提交入场订单" },
  { kind: "strategy_exit", label: "退出订单", description: "提交退出/止盈止损订单" },
  { kind: "strategy_order", label: "通用订单", description: "提交通用策略订单" },
  { kind: "strategy_close", label: "平仓", description: "关闭指定入场 ID" },
  { kind: "plot", label: "绘图", description: "绘制序列" },
  { kind: "alertcondition", label: "提醒条件", description: "声明提醒条件" },
  { kind: "log", label: "日志", description: "写入 Pine runtime 日志" },
];

export function createDefaultPineV6Workflow(title = "Pine v6 原生策略"): PineV6WorkflowDocument {
  return {
    engine: PINE_V6_WORKFLOW_ENGINE,
    version: 1,
    declaration: {
      title,
      overlay: true,
      initialCapital: 100000,
      currency: "",
      pyramiding: 0,
      defaultQtyType: "strategy.percent_of_equity",
      defaultQtyValue: 10,
      calcOnEveryTick: false,
      processOrdersOnClose: false,
    },
    inputs: [
      {
        id: createWorkflowId("input"),
        name: "fastLen",
        title: "快线周期",
        type: "int",
        defaultValue: "12",
      },
      {
        id: createWorkflowId("input"),
        name: "slowLen",
        title: "慢线周期",
        type: "int",
        defaultValue: "26",
      },
    ],
    blocks: [
      {
        id: createWorkflowId("block"),
        kind: "series_assign",
        enabled: true,
        title: "计算快线 EMA",
        params: { name: "fast", expression: "ta.ema(close, fastLen)" },
      },
      {
        id: createWorkflowId("block"),
        kind: "series_assign",
        enabled: true,
        title: "计算慢线 EMA",
        params: { name: "slow", expression: "ta.ema(close, slowLen)" },
      },
      {
        id: createWorkflowId("block"),
        kind: "if",
        enabled: true,
        title: "快线上穿慢线",
        params: { condition: "ta.crossover(fast, slow)" },
        thenBlocks: [
          {
            id: createWorkflowId("block"),
            kind: "strategy_entry",
            enabled: true,
            title: "提交多头入场",
            params: { id: "Long", direction: "strategy.long", qty: "" },
          },
        ],
        elseBlocks: [
          {
            id: createWorkflowId("block"),
            kind: "strategy_close",
            enabled: true,
            title: "关闭多头仓位",
            params: { id: "Long", when: "ta.crossunder(fast, slow)" },
          },
        ],
      },
      {
        id: createWorkflowId("block"),
        kind: "plot",
        enabled: true,
        title: "绘制快线",
        params: { series: "fast", title: "快线 EMA", color: "color.teal" },
      },
      {
        id: createWorkflowId("block"),
        kind: "plot",
        enabled: true,
        title: "绘制慢线",
        params: { series: "slow", title: "慢线 EMA", color: "color.orange" },
      },
    ],
    runtimeBindingDraft: {
      market: "HK",
      code: "00700",
      interval: "5m",
      executionMode: "live",
      useExtendedHours: false,
    },
  };
}

export function createPineV6WorkflowBlock(kind: PineV6WorkflowBlockKind): PineV6WorkflowBlock {
  const definition = PINE_V6_BLOCK_KINDS.find((item) => item.kind === kind);
  return {
    id: createWorkflowId("block"),
    kind,
    enabled: true,
    title: definition?.label ?? kind,
    params: defaultParamsForBlock(kind),
    ...(kind === "if" ? { thenBlocks: [], elseBlocks: [] } : {}),
  };
}

export function normalizePineV6Workflow(value: unknown): PineV6WorkflowDocument {
  if (!isRecord(value) || value.engine !== PINE_V6_WORKFLOW_ENGINE) {
    return createDefaultPineV6Workflow();
  }
  const fallback = createDefaultPineV6Workflow();
  return {
    engine: PINE_V6_WORKFLOW_ENGINE,
    version: readPositiveInteger(value.version, fallback.version),
    declaration: normalizeDeclaration(value.declaration, fallback.declaration),
    inputs: normalizeInputs(value.inputs, fallback.inputs),
    blocks: normalizeBlocks(value.blocks, fallback.blocks),
    runtimeBindingDraft: normalizeRuntimeBindingDraft(
      value.runtimeBindingDraft,
      fallback.runtimeBindingDraft,
    ),
  };
}

export function buildPineV6WorkflowScript(workflow: PineV6WorkflowDocument): string {
  const normalized = normalizePineV6Workflow(workflow);
  const lines = [
    "//@version=6",
    renderStrategyDeclaration(normalized.declaration),
    "",
    "// Pine v6 工作流按已确认的 K 线收盘事件运行。",
    "barClosed = barstate.isconfirmed",
    "",
    ...normalized.inputs.map(renderInput),
  ];
  if (normalized.inputs.length > 0) {
    lines.push("");
  }
  lines.push("if barClosed");
  const blockLines = renderBlocks(normalized.blocks, 1);
  lines.push(...(blockLines.length > 0 ? blockLines : ["    // 在这里添加 Pine v6 工作流块。"]));
  return `${lines.join("\n").trimEnd()}\n`;
}

export function assessPineV6Workflow(workflow: PineV6WorkflowDocument): PineV6WorkflowDiagnostic[] {
  const diagnostics: PineV6WorkflowDiagnostic[] = [];
  const normalized = normalizePineV6Workflow(workflow);
  if (normalized.declaration.title.trim() === "") {
    diagnostics.push({
      severity: "error",
      code: "PINE_WORKFLOW_EMPTY_TITLE",
      message: "strategy title is required.",
    });
  }
  forEachBlock(normalized.blocks, (block) => {
    if (!block.enabled) {
      return;
    }
    if (block.kind === "strategy_entry" || block.kind === "strategy_order") {
      if (readString(block.params.oca_name) !== "" || readString(block.params.oca_type) !== "") {
        diagnostics.push({
          blockId: block.id,
          severity: "warning",
          code: "PINE_ORDER_OCA_UNSUPPORTED",
          message: "OCA fields are shown for Pine v6 parity, but this runtime currently rejects OCA order semantics.",
        });
      }
    }
    if (block.kind === "if" && readString(block.params.condition) === "") {
      diagnostics.push({
        blockId: block.id,
        severity: "error",
        code: "PINE_WORKFLOW_EMPTY_IF",
        message: "if condition is required.",
      });
    }
  });
  return diagnostics;
}

export function createWorkflowId(prefix: string): string {
  const random = globalThis.crypto?.randomUUID?.() ?? Math.random().toString(36).slice(2);
  return `${prefix}-${random}`;
}

function renderStrategyDeclaration(declaration: PineV6WorkflowDeclaration): string {
  const args = [
    quoteString(declaration.title.trim() || "Pine v6 原生策略"),
    `overlay=${declaration.overlay ? "true" : "false"}`,
    optionalNumberArg("initial_capital", declaration.initialCapital),
    optionalRawArg("currency", declaration.currency),
    optionalNumberArg("pyramiding", declaration.pyramiding),
    optionalRawArg("default_qty_type", declaration.defaultQtyType),
    optionalNumberArg("default_qty_value", declaration.defaultQtyValue),
    `calc_on_every_tick=${declaration.calcOnEveryTick === true ? "true" : "false"}`,
    `process_orders_on_close=${declaration.processOrdersOnClose === true ? "true" : "false"}`,
  ].filter((value): value is string => value !== null && value !== "");
  return `strategy(${args.join(", ")})`;
}

function renderInput(input: PineV6WorkflowInput): string {
  const name = sanitizeIdentifier(input.name, "inputValue");
  const title = quoteString(input.title.trim() || name);
  const defaultValue = input.defaultValue.trim();
  switch (input.type) {
    case "float":
      return `${name} = input.float(${defaultValue || "0.0"}, ${title})`;
    case "bool":
      return `${name} = input.bool(${defaultValue === "true" ? "true" : "false"}, ${title})`;
    case "string":
      return `${name} = input.string(${quoteString(defaultValue)}, ${title})`;
    case "source":
      return `${name} = input.source(${defaultValue || "close"}, ${title})`;
    case "time":
      return `${name} = input.time(${defaultValue || "timestamp(2026, 1, 1, 0, 0)"}, ${title})`;
    default:
      return `${name} = input.int(${defaultValue || "1"}, ${title})`;
  }
}

function renderBlocks(blocks: PineV6WorkflowBlock[], indentLevel: number): string[] {
  const lines: string[] = [];
  for (const block of blocks) {
    if (!block.enabled) {
      continue;
    }
    lines.push(...renderBlock(block, indentLevel));
  }
  return lines;
}

function renderBlock(block: PineV6WorkflowBlock, indentLevel: number): string[] {
  const indent = "    ".repeat(indentLevel);
  const params = block.params;
  switch (block.kind) {
    case "series_assign":
      return [`${indent}${sanitizeIdentifier(readString(params.name), "seriesValue")} = ${readExpression(params.expression, "close")}`];
    case "var_state":
      return [`${indent}var ${sanitizeIdentifier(readString(params.name), "stateValue")} = ${readExpression(params.initial, "na")}`];
    case "if": {
      const lines = [`${indent}if ${readExpression(params.condition, "false")}`];
      const thenLines = renderBlocks(block.thenBlocks ?? [], indentLevel + 1);
      lines.push(...(thenLines.length > 0 ? thenLines : [`${indent}    // then`]));
      const elseLines = renderBlocks(block.elseBlocks ?? [], indentLevel + 1);
      if (elseLines.length > 0) {
        lines.push(`${indent}else`);
        lines.push(...elseLines);
      }
      return lines;
    }
    case "request_security":
      return [
        `${indent}${sanitizeIdentifier(readString(params.name), "mtfValue")} = request.security(${quoteString(readString(params.symbol) || "syminfo.tickerid")}, ${quoteString(readString(params.timeframe) || "D")}, ${readExpression(params.expression, "close")})`,
      ];
    case "array_op":
      return renderArrayOperation(params, indent);
    case "strategy_entry":
      return [`${indent}strategy.entry(${renderCallArgs([
        quoteString(readString(params.id) || "Long"),
        normalizeDirection(readString(params.direction), "strategy.long"),
        optionalRawArg("qty", params.qty),
        optionalRawArg("limit", params.limit),
        optionalRawArg("stop", params.stop),
        optionalRawArg("comment", quoteMaybe(params.comment)),
        optionalRawArg("when", params.when),
      ])})`];
    case "strategy_exit":
      return [`${indent}strategy.exit(${renderCallArgs([
        quoteString(readString(params.id) || "Exit"),
        optionalRawArg("from_entry", quoteMaybe(params.from_entry) ?? quoteString("Long")),
        optionalRawArg("qty", params.qty),
        optionalRawArg("limit", params.limit),
        optionalRawArg("stop", params.stop),
        optionalRawArg("profit", params.profit),
        optionalRawArg("loss", params.loss),
        optionalRawArg("trail_points", params.trail_points),
        optionalRawArg("when", params.when),
      ])})`];
    case "strategy_order":
      return [`${indent}strategy.order(${renderCallArgs([
        quoteString(readString(params.id) || "Order"),
        normalizeDirection(readString(params.direction), "strategy.long"),
        optionalRawArg("qty", params.qty),
        optionalRawArg("limit", params.limit),
        optionalRawArg("stop", params.stop),
        optionalRawArg("when", params.when),
      ])})`];
    case "strategy_close":
      return [`${indent}strategy.close(${renderCallArgs([
        quoteString(readString(params.id) || "Long"),
        optionalRawArg("when", params.when),
        optionalRawArg("comment", quoteMaybe(params.comment)),
      ])})`];
    case "plot":
      return [`${indent}plot(${renderCallArgs([
        readExpression(params.series, "close"),
        optionalRawArg("title", quoteMaybe(params.title)),
        optionalRawArg("color", params.color),
      ])})`];
    case "alertcondition":
      return [`${indent}alertcondition(${renderCallArgs([
        readExpression(params.condition, "false"),
        optionalRawArg("title", quoteMaybe(params.title) ?? quoteString("提醒")),
        optionalRawArg("message", quoteMaybe(params.message) ?? quoteString("Pine v6 工作流提醒")),
      ])})`];
    case "log":
      return [`${indent}log.info(${quoteString(readString(params.message) || "Pine v6 工作流日志")})`];
    default:
      return [`${indent}// 暂不支持的块：${block.kind}`];
  }
}

function renderArrayOperation(params: Record<string, unknown>, indent: string): string[] {
  const name = sanitizeIdentifier(readString(params.name), "values");
  const mode = readString(params.mode) || "new_float";
  if (mode === "push") {
    return [`${indent}array.push(${name}, ${readExpression(params.value, "close")})`];
  }
  if (mode === "median") {
    return [`${indent}${sanitizeIdentifier(readString(params.output), "medianValue")} = array.median(${name})`];
  }
  return [`${indent}var ${name} = array.new_float()`];
}

function defaultParamsForBlock(kind: PineV6WorkflowBlockKind): Record<string, unknown> {
  switch (kind) {
    case "series_assign":
      return { name: "signal", expression: "close > open" };
    case "var_state":
      return { name: "armed", initial: "false" };
    case "if":
      return { condition: "close > open" };
    case "request_security":
      return { name: "dailyClose", symbol: "syminfo.tickerid", timeframe: "D", expression: "close" };
    case "array_op":
      return { name: "values", mode: "push", value: "close" };
    case "strategy_entry":
      return { id: "Long", direction: "strategy.long", qty: "" };
    case "strategy_exit":
      return { id: "Exit", from_entry: "Long", stop: "", limit: "" };
    case "strategy_order":
      return { id: "Order", direction: "strategy.long", qty: "" };
    case "strategy_close":
      return { id: "Long", when: "" };
    case "plot":
      return { series: "close", title: "Close", color: "color.blue" };
    case "alertcondition":
      return { condition: "close > open", title: "提醒", message: "Pine v6 工作流提醒" };
    case "log":
      return { message: "Pine v6 工作流" };
  }
}

function normalizeDeclaration(value: unknown, fallback: PineV6WorkflowDeclaration): PineV6WorkflowDeclaration {
  const record = isRecord(value) ? value : {};
  return {
    title: readString(record.title) || fallback.title,
    overlay: typeof record.overlay === "boolean" ? record.overlay : fallback.overlay,
    initialCapital: readNullableNumber(record.initialCapital, fallback.initialCapital ?? null),
    currency: readString(record.currency) || (fallback.currency ?? "HKD"),
    pyramiding: readNullableNumber(record.pyramiding, fallback.pyramiding ?? 0),
    defaultQtyType: readString(record.defaultQtyType) || (fallback.defaultQtyType ?? "strategy.percent_of_equity"),
    defaultQtyValue: readNullableNumber(record.defaultQtyValue, fallback.defaultQtyValue ?? 10),
    calcOnEveryTick: typeof record.calcOnEveryTick === "boolean" ? record.calcOnEveryTick : false,
    processOrdersOnClose: typeof record.processOrdersOnClose === "boolean" ? record.processOrdersOnClose : false,
  };
}

function normalizeInputs(value: unknown, fallback: PineV6WorkflowInput[]): PineV6WorkflowInput[] {
  if (!Array.isArray(value)) {
    return fallback;
  }
  return value.map((entry) => {
    const record = isRecord(entry) ? entry : {};
    const type = readString(record.type);
    return {
      id: readString(record.id) || createWorkflowId("input"),
      name: sanitizeIdentifier(readString(record.name), "inputValue"),
      title: readString(record.title) || readString(record.name) || "输入参数",
      type: type === "float" || type === "bool" || type === "string" || type === "source" || type === "time" ? type : "int",
      defaultValue: readString(record.defaultValue) || "1",
    };
  });
}

function normalizeBlocks(value: unknown, fallback: PineV6WorkflowBlock[]): PineV6WorkflowBlock[] {
  if (!Array.isArray(value)) {
    return fallback;
  }
  return value.map((entry) => {
    const record = isRecord(entry) ? entry : {};
    const kind = normalizeBlockKind(readString(record.kind));
    return {
      id: readString(record.id) || createWorkflowId("block"),
      kind,
      enabled: record.enabled !== false,
      title: readString(record.title) || kind,
      params: isRecord(record.params) ? { ...record.params } : defaultParamsForBlock(kind),
      ...(kind === "if" ? { thenBlocks: normalizeBlocks(record.thenBlocks, []) } : {}),
      ...(kind === "if" ? { elseBlocks: normalizeBlocks(record.elseBlocks, []) } : {}),
    };
  });
}

function normalizeRuntimeBindingDraft(
  value: unknown,
  fallback: PineV6WorkflowRuntimeBindingDraft,
): PineV6WorkflowRuntimeBindingDraft {
  const record = isRecord(value) ? value : {};
  const executionMode = readString(record.executionMode);
  return {
    market: readString(record.market).toUpperCase() || fallback.market,
    code: readString(record.code).toUpperCase() || fallback.code,
    interval: readString(record.interval) || fallback.interval,
    executionMode: (executionMode === "notify_only" ? "notify_only" : "live") as StrategyExecutionMode,
    useExtendedHours: record.useExtendedHours === true,
    ...(readString(record.brokerAccountKey) === "" ? {} : { brokerAccountKey: readString(record.brokerAccountKey) }),
    ...(isRecord(record.runtimeRisk)
      ? { runtimeRisk: record.runtimeRisk as unknown as StrategyRuntimeRiskSettings }
      : fallback.runtimeRisk === undefined ? {} : { runtimeRisk: fallback.runtimeRisk }),
  };
}

function normalizeBlockKind(value: string): PineV6WorkflowBlockKind {
  return PINE_V6_BLOCK_KINDS.some((item) => item.kind === value)
    ? value as PineV6WorkflowBlockKind
    : "series_assign";
}

function forEachBlock(blocks: PineV6WorkflowBlock[], visitor: (block: PineV6WorkflowBlock) => void): void {
  for (const block of blocks) {
    visitor(block);
    forEachBlock(block.thenBlocks ?? [], visitor);
    forEachBlock(block.elseBlocks ?? [], visitor);
  }
}

function renderCallArgs(values: Array<string | null>): string {
  return values.filter((value): value is string => value !== null && value !== "").join(", ");
}

function optionalNumberArg(name: string, value: unknown): string | null {
  const numeric = typeof value === "number" ? value : Number(value);
  return Number.isFinite(numeric) ? `${name}=${numeric}` : null;
}

function optionalRawArg(name: string, value: unknown): string | null {
  const text = readString(value);
  return text === "" ? null : `${name}=${text}`;
}

function quoteMaybe(value: unknown): string | null {
  const text = readString(value);
  return text === "" ? null : quoteString(text);
}

function quoteString(value: string): string {
  return `"${value.replaceAll("\\", "\\\\").replaceAll("\"", "\\\"")}"`;
}

function normalizeDirection(value: string, fallback: string): string {
  return value === "strategy.short" || value === "strategy.long" ? value : fallback;
}

function readExpression(value: unknown, fallback: string): string {
  const text = readString(value);
  return text === "" ? fallback : text;
}

function sanitizeIdentifier(value: string, fallback: string): string {
  const candidate = value.trim().replace(/[^A-Za-z0-9_]/g, "_");
  if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(candidate)) {
    return candidate;
  }
  return fallback;
}

function readString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function readPositiveInteger(value: unknown, fallback: number): number {
  const numeric = typeof value === "number" ? value : Number(value);
  return Number.isInteger(numeric) && numeric > 0 ? numeric : fallback;
}

function readNullableNumber(value: unknown, fallback: number | null): number | null {
  if (value === null || value === undefined || value === "") {
    return fallback;
  }
  const numeric = typeof value === "number" ? value : Number(value);
  return Number.isFinite(numeric) ? numeric : fallback;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
