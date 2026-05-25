export interface MonacoExtraLibDefinition {
  filePath: string;
  content: string;
}

export interface MonacoCompletionDefinition {
  label: string;
  insertText: string;
  detail: string;
  documentation: string;
  kind?: "function" | "snippet" | "interface" | "variable";
  insertTextRule?: "plain" | "snippet";
  sortText?: string;
}

export interface MonacoHoverDefinition {
  target: string;
  signature: string;
  documentation: string;
}

const runtimeHostNotice =
  "真实 QuickJS runtime 宿主 API：在运行态可直接调用；编辑器同步提供类型声明、补全与 hover 文档。";

const generatedHelperNotice =
  "Logic Flow 模板生成的辅助函数。后续自定义因子块建议复用同一套 helper 语义与命名。";

const generatedFactorRuntimeNotice =
  "Logic Flow 模板生成时常见的运行时变量。后续自定义因子块建议沿用这些缓存字段和中间量命名。";

export const strategyEditorExtraLibs: MonacoExtraLibDefinition[] = [
  {
    filePath: "file:///jftrade/strategy-runtime.d.ts",
    content: `declare interface JFTradeStrategyBaseContext {
  id: string;
  definitionId: string;
  symbol: string;
  interval: string;
}

declare interface JFTradeInitContext extends JFTradeStrategyBaseContext {
  name: string;
}

declare interface JFTradeKLine {
  symbol: string;
  interval: string;
  startTime: string;
  endTime: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  quoteVolume: number;
  closed: boolean;
}

declare interface JFTradeMovingAverageIndicatorSnapshot {
  value: number | null;
  previous: number | null;
}

declare interface JFTradeMACDIndicatorSnapshot {
  diff: number;
  signal: number;
  histogram: number;
  previousDiff?: number;
  previousSignal?: number;
  previousHistogram?: number;
}

declare interface JFTradeKDJIndicatorSnapshot {
  k: number;
  d: number;
  j: number;
  previousK?: number;
  previousD?: number;
  previousJ?: number;
}

declare interface JFTradeBollingerIndicatorSnapshot {
  middle: number;
  upper: number;
  lower: number;
}

declare interface JFTradeIndicatorMap {
  [key: \`ma:\${string}\`]: JFTradeMovingAverageIndicatorSnapshot | null | undefined;
  [key: \`rsi:\${number}\`]: number | null | undefined;
  [key: \`macd:\${number}:\${number}:\${number}\`]: JFTradeMACDIndicatorSnapshot | null | undefined;
  [key: \`bollinger:\${number}:\${number}\`]: JFTradeBollingerIndicatorSnapshot | null | undefined;
  [key: \`kdj:\${number}:\${number}:\${number}\`]: JFTradeKDJIndicatorSnapshot | null | undefined;
  [key: \`atr:\${number}\`]: number | null | undefined;
  [key: \`cci:\${number}\`]: number | null | undefined;
  [key: \`williamsr:\${number}\`]: number | null | undefined;
  [key: \`divergence:rsi:\${number}:top:\${number}\`]: boolean | null | undefined;
  [key: \`divergence:rsi:\${number}:bottom:\${number}\`]: boolean | null | undefined;
  [key: \`divergence:macd:\${number}:\${number}:\${number}:top:\${number}\`]: boolean | null | undefined;
  [key: \`divergence:macd:\${number}:\${number}:\${number}:bottom:\${number}\`]: boolean | null | undefined;
  [key: \`divergence:kdj:\${number}:\${number}:\${number}:top:\${number}\`]: boolean | null | undefined;
  [key: \`divergence:kdj:\${number}:\${number}:\${number}:bottom:\${number}\`]: boolean | null | undefined;
}

declare interface JFTradeKLineClosedContext extends JFTradeStrategyBaseContext {
  kline: JFTradeKLine;
  indicators: JFTradeIndicatorMap;
}

declare type JFTradeStrategyContext =
  | JFTradeInitContext
  | JFTradeKLineClosedContext;

declare type JFTradeOrderSide = "BUY" | "SELL";
declare type JFTradeOrderType = "MARKET" | "LIMIT";
declare type JFTradeTimeInForce = "GTC" | "IOC" | "FOK" | "GTT";
declare type JFTradeRiskOperation = "PLACE" | "MODIFY" | "CANCEL";

declare interface JFTradePlaceOrderRequest {
  symbol?: string;
  clientOrderId?: string;
  side: JFTradeOrderSide;
  quantity: number;
  orderType?: JFTradeOrderType;
  limitPrice?: number;
  timeInForce?: JFTradeTimeInForce;
  stopPrice?: number;
  note?: string;
  reduceOnly?: boolean;
  closePosition?: boolean;
}

declare interface JFTradeOrderAck {
  accepted: boolean;
  requestId: string;
  orderId?: string;
  status: string;
  message?: string;
}

declare interface JFTradePositionSnapshot {
  symbol: string;
  quantity: number;
  availableQuantity: number;
  averageCost: number;
  marketValue: number;
  unrealizedPnL: number;
  lastPrice: number;
  direction: "LONG" | "SHORT" | "FLAT";
}

declare interface JFTradeRiskSnapshot {
  source: "quickjs-runtime";
  accountAvailable: boolean;
  accountType: string;
  executorAvailable: boolean;
  realTradingEnabled: boolean;
  riskEnabled: boolean;
  killSwitchActive: boolean;
  blockedOperations: JFTradeRiskOperation[];
  allowsCancel: boolean;
  canTrade: boolean;
  canDeposit: boolean;
  canWithdraw: boolean;
}

declare function notify(...args: unknown[]): void;
declare function onInit(ctx: JFTradeInitContext): void;
declare function onKLineClosed(ctx: JFTradeKLineClosedContext): void;
declare function placeOrder(request: JFTradePlaceOrderRequest): JFTradeOrderAck;
declare function cancelOrder(orderId: string): boolean;
declare function getPosition(symbol?: string): JFTradePositionSnapshot | null;
declare function getPositions(): JFTradePositionSnapshot[];
declare function getRiskState(): JFTradeRiskSnapshot;
declare function isOperationBlocked(operation: JFTradeRiskOperation): boolean;
declare function getAvailableCash(): number;
declare function getTotalAccountValue(): number;
`,
  },
];

export const strategyEditorCompletions: MonacoCompletionDefinition[] = [
  {
    label: "onInit",
    detail: "QuickJS lifecycle hook",
    documentation: "插入带类型注释的 onInit(ctx) 策略启动钩子。",
    kind: "snippet",
    insertTextRule: "snippet",
    sortText: "01",
    insertText: [
      "/** @param {JFTradeInitContext} ctx */",
      "function onInit(ctx) {",
      "  console.log(\"strategy started\", ctx.symbol, ctx.interval);",
      "  $0",
      "}",
    ].join("\n"),
  },
  {
    label: "onKLineClosed",
    detail: "QuickJS market data hook",
    documentation: "插入带类型注释的 onKLineClosed(ctx) K 线收盘钩子。",
    kind: "snippet",
    insertTextRule: "snippet",
    sortText: "02",
    insertText: [
      "/** @param {JFTradeKLineClosedContext} ctx */",
      "function onKLineClosed(ctx) {",
      "  const close = ctx.kline.close;",
      "  console.log(\"close\", close);",
      "  $0",
      "}",
    ].join("\n"),
  },
  {
    label: "notify",
    detail: "QuickJS host API",
    documentation: "向运行日志和通知通道写一条消息。",
    kind: "function",
    insertTextRule: "snippet",
    sortText: "03",
    insertText: "notify(${1:message});",
  },
  {
    label: "JFTradeKLineClosedContext",
    detail: "QuickJS runtime type",
    documentation: "K 线收盘事件的上下文类型，包含 ctx.kline.open/high/low/close 等字段。",
    kind: "interface",
    insertTextRule: "plain",
    sortText: "04",
    insertText: "JFTradeKLineClosedContext",
  },
  {
    label: "JFTradeInitContext",
    detail: "QuickJS runtime type",
    documentation: "策略启动事件的上下文类型，包含策略基础字段和 name。",
    kind: "interface",
    insertTextRule: "plain",
    sortText: "05",
    insertText: "JFTradeInitContext",
  },
  {
    label: "placeOrder",
    detail: "QuickJS host API",
    documentation: `${runtimeHostNotice} 插入一段带风控保护的同步下单示例。`,
    kind: "snippet",
    insertTextRule: "snippet",
    sortText: "06",
    insertText: [
      "const riskState = getRiskState();",
      "if (isOperationBlocked(\"PLACE\")) {",
      "  notify(\"place blocked\", JSON.stringify(riskState));",
      "  return;",
      "}",
      "const orderResult = placeOrder({",
      "  symbol: ctx.symbol,",
      "  side: \"BUY\",",
      "  quantity: 1,",
      "  orderType: \"LIMIT\",",
      "  limitPrice: ctx.kline.close,",
      "});",
      "console.log(orderResult.status, orderResult.orderId);",
      "$0",
    ].join("\n"),
  },
  {
    label: "cancelOrder",
    detail: "QuickJS host API",
    documentation: `${runtimeHostNotice} 撤销当前策略运行态已经记录过的订单。`,
    kind: "function",
    insertTextRule: "snippet",
    sortText: "07",
    insertText: "cancelOrder(${1:orderId})",
  },
  {
    label: "getPosition",
    detail: "QuickJS host API",
    documentation: `${runtimeHostNotice} 查询单个标的的仓位快照，symbol 可省略为当前策略标的。`,
    kind: "function",
    insertTextRule: "snippet",
    sortText: "08",
    insertText: "getPosition(${1:ctx.symbol})",
  },
  {
    label: "getPositions",
    detail: "QuickJS host API",
    documentation: `${runtimeHostNotice} 查询当前策略可见的全部仓位快照。`,
    kind: "function",
    insertTextRule: "snippet",
    sortText: "09",
    insertText: "getPositions()",
  },
  {
    label: "getRiskState",
    detail: "QuickJS host API",
    documentation: `${runtimeHostNotice} 查询当前 runtime 的会话能力、交易可用性和阻断操作列表。`,
    kind: "function",
    insertTextRule: "snippet",
    sortText: "10",
    insertText: [
      "const riskState = getRiskState();",
      "if (riskState.realTradingEnabled) {",
      "  console.log(\"runtime trading ready\", riskState.accountType);",
      "}",
      "$0",
    ].join("\n"),
  },
  {
    label: "getAvailableCash",
    detail: "QuickJS host API",
    documentation: `${runtimeHostNotice} 查询当前策略标的报价币种下可用于下单的资金。`,
    kind: "function",
    insertTextRule: "snippet",
    sortText: "11",
    insertText: "getAvailableCash()",
  },
  {
    label: "getTotalAccountValue",
    detail: "QuickJS host API",
    documentation: `${runtimeHostNotice} 查询账户总资产，优先使用 runtime 已归一化的总权益。`,
    kind: "function",
    insertTextRule: "snippet",
    sortText: "12",
    insertText: "getTotalAccountValue()",
  },
  {
    label: "JFTradePlaceOrderRequest",
    detail: "QuickJS runtime type",
    documentation: `${runtimeHostNotice} 下单请求结构。`,
    kind: "interface",
    insertTextRule: "plain",
    sortText: "13",
    insertText: "JFTradePlaceOrderRequest",
  },
  {
    label: "JFTradeRiskSnapshot",
    detail: "QuickJS runtime type",
    documentation: `${runtimeHostNotice} runtime 层风险与会话能力快照。`,
    kind: "interface",
    insertTextRule: "plain",
    sortText: "14",
    insertText: "JFTradeRiskSnapshot",
  },
];

const coreHoverItems: MonacoHoverDefinition[] = [
  {
    target: "ctx",
    signature: "const ctx: JFTradeInitContext | JFTradeKLineClosedContext",
    documentation: "当前生命周期上下文对象。onInit(ctx) 会提供 name，onKLineClosed(ctx) 会额外提供 ctx.kline。",
  },
  {
    target: "ctx.id",
    signature: "ctx.id: string",
    documentation: "当前策略运行实例 ID。",
  },
  {
    target: "ctx.name",
    signature: "ctx.name: string",
    documentation: "当前策略名称，仅在 onInit(ctx) 中保证提供。",
  },
  {
    target: "ctx.definitionId",
    signature: "ctx.definitionId: string",
    documentation: "当前策略定义 ID，可用于把日志和策略草稿关联起来。",
  },
  {
    target: "ctx.symbol",
    signature: "ctx.symbol: string",
    documentation: "当前策略绑定的交易标的代码。",
  },
  {
    target: "ctx.interval",
    signature: "ctx.interval: string",
    documentation: "当前策略绑定的 K 线周期，例如 1m、5m、1h。",
  },
  {
    target: "ctx.kline",
    signature: "ctx.kline: JFTradeKLine",
    documentation: "K 线收盘事件里的行情对象，包含 OHLC、成交量和时间区间。",
  },
  {
    target: "ctx.indicators",
    signature: "ctx.indicators: JFTradeIndicatorMap",
    documentation: "由 Go runtime 在调用 QuickJS 前预计算并注入的指标结果集合，键名如 ma:5、rsi:14、macd:12:26:9、kdj:9:3:3、atr:14、cci:20、williamsr:14、bollinger:20:2。",
  },
  {
    target: "ctx.kline.open",
    signature: "ctx.kline.open: number",
    documentation: "当前 K 线开盘价。",
  },
  {
    target: "ctx.kline.high",
    signature: "ctx.kline.high: number",
    documentation: "当前 K 线最高价。",
  },
  {
    target: "ctx.kline.low",
    signature: "ctx.kline.low: number",
    documentation: "当前 K 线最低价。",
  },
  {
    target: "ctx.kline.close",
    signature: "ctx.kline.close: number",
    documentation: "当前 K 线收盘价，均线、RSI、MACD 和布林带模板都默认基于这个字段计算。",
  },
  {
    target: "ctx.kline.volume",
    signature: "ctx.kline.volume: number",
    documentation: "当前 K 线成交量。",
  },
  {
    target: "ctx.kline.quoteVolume",
    signature: "ctx.kline.quoteVolume: number",
    documentation: "当前 K 线成交额。",
  },
  {
    target: "ctx.kline.startTime",
    signature: "ctx.kline.startTime: string",
    documentation: "当前 K 线开始时间，ISO 8601 字符串。",
  },
  {
    target: "ctx.kline.endTime",
    signature: "ctx.kline.endTime: string",
    documentation: "当前 K 线结束时间，ISO 8601 字符串。",
  },
  {
    target: "ctx.kline.closed",
    signature: "ctx.kline.closed: boolean",
    documentation: "当前 K 线是否已收盘。onKLineClosed(ctx) 中通常恒为 true。",
  },
  {
    target: "notify",
    signature: "function notify(...args: unknown[]): void",
    documentation: "向运行日志和通知通道写一条消息。",
  },
  {
    target: "onInit",
    signature: "function onInit(ctx: JFTradeInitContext): void",
    documentation: "策略启动钩子。适合做状态初始化、记录策略启动日志。",
  },
  {
    target: "onKLineClosed",
    signature: "function onKLineClosed(ctx: JFTradeKLineClosedContext): void",
    documentation: "K 线收盘钩子。策略视觉模板生成的主逻辑默认都写在这里。",
  },
  {
    target: "JFTradeInitContext",
    signature: "type JFTradeInitContext = JFTradeStrategyBaseContext & { name: string }",
    documentation: "策略启动钩子的上下文类型。",
  },
  {
    target: "JFTradeKLineClosedContext",
    signature: "type JFTradeKLineClosedContext = JFTradeStrategyBaseContext & { kline: JFTradeKLine }",
    documentation: "K 线收盘钩子的上下文类型。",
  },
];

const runtimeHostHoverItems: MonacoHoverDefinition[] = [
  {
    target: "placeOrder",
    signature: "function placeOrder(request: JFTradePlaceOrderRequest): JFTradeOrderAck",
    documentation: `${runtimeHostNotice} 当前桥接完整支持 MARKET / LIMIT 两种订单类型；symbol 可省略为当前策略标的。`,
  },
  {
    target: "cancelOrder",
    signature: "function cancelOrder(orderId: string): boolean",
    documentation: `${runtimeHostNotice} 仅能撤销当前 QuickJS runtime 已下单并缓存过的订单。`,
  },
  {
    target: "getPosition",
    signature: "function getPosition(symbol?: string): JFTradePositionSnapshot | null",
    documentation: `${runtimeHostNotice} 查询单个标的的仓位快照；省略 symbol 时回退到当前策略标的。`,
  },
  {
    target: "getPositions",
    signature: "function getPositions(): JFTradePositionSnapshot[]",
    documentation: `${runtimeHostNotice} 返回当前策略标的与 session 已知 positions 的并集快照。`,
  },
  {
    target: "getRiskState",
    signature: "function getRiskState(): JFTradeRiskSnapshot",
    documentation: `${runtimeHostNotice} 返回 runtime 本地的会话能力快照，不是控制面 real-trade-risk 接口的直通映射。`,
  },
  {
    target: "isOperationBlocked",
    signature: "function isOperationBlocked(operation: JFTradeRiskOperation): boolean",
    documentation: `${runtimeHostNotice} 基于 blockedOperations / allowsCancel 判断 PLACE、MODIFY、CANCEL 是否应被拦截。`,
  },
  {
    target: "getAvailableCash",
    signature: "function getAvailableCash(): number",
    documentation: `${runtimeHostNotice} 返回当前策略标的报价币种下可用的下单资金，不再复用账户总资产口径。`,
  },
  {
    target: "getTotalAccountValue",
    signature: "function getTotalAccountValue(): number",
    documentation: `${runtimeHostNotice} 返回账户总资产，优先使用 runtime 已归一化的 TotalAccountValue。`,
  },
  {
    target: "JFTradePlaceOrderRequest",
    signature: "interface JFTradePlaceOrderRequest",
    documentation: `${runtimeHostNotice} 下单请求结构，包含 side / quantity / orderType / limitPrice / timeInForce 等字段。`,
  },
  {
    target: "JFTradeOrderAck",
    signature: "interface JFTradeOrderAck",
    documentation: `${runtimeHostNotice} 下单成功后的确认结构，包含 accepted / requestId / orderId / status / message。`,
  },
  {
    target: "JFTradeRiskSnapshot",
    signature: "interface JFTradeRiskSnapshot",
    documentation: `${runtimeHostNotice} runtime 层会话能力快照，反映 executor、账户能力与阻断操作列表。`,
  },
  {
    target: "JFTradePositionSnapshot",
    signature: "interface JFTradePositionSnapshot",
    documentation: `${runtimeHostNotice} 持仓快照结构，包含 quantity / availableQuantity / averageCost / marketValue / unrealizedPnL / lastPrice。`,
  },
];

const generatedHelperHoverItems: MonacoHoverDefinition[] = [
  {
    target: "simpleMovingAverage",
    signature: "function simpleMovingAverage(values: number[], windowSize: number): number | null",
    documentation: `${generatedHelperNotice} 按窗口长度计算最近一段收盘价的简单移动平均。`,
  },
  {
    target: "calculateRSI",
    signature: "function calculateRSI(values: number[], period: number): number | null",
    documentation: `${generatedHelperNotice} 基于最近 period 段价格变化计算 RSI。`,
  },
  {
    target: "calculateEMASequence",
    signature: "function calculateEMASequence(values: number[], period: number): number[]",
    documentation: `${generatedHelperNotice} 生成 EMA 序列，MACD helper 会复用它。`,
  },
  {
    target: "calculateMACD",
    signature: "function calculateMACD(values: number[], fastPeriod: number, slowPeriod: number, signalPeriod: number): { diff: number; signal: number; histogram: number } | null",
    documentation: `${generatedHelperNotice} 返回 diff、signal 和 histogram 三个字段，供 MACD 条件块和日志块直接消费。`,
  },
  {
    target: "calculateStandardDeviation",
    signature: "function calculateStandardDeviation(values: number[], average: number): number",
    documentation: `${generatedHelperNotice} 计算一段价格窗口相对平均值的标准差。`,
  },
  {
    target: "calculateBollingerBands",
    signature: "function calculateBollingerBands(values: number[], period: number, multiplier: number): { middle: number; upper: number; lower: number } | null",
    documentation: `${generatedHelperNotice} 返回布林带中轨、上轨、下轨，供布林带条件块与日志块复用。`,
  },
];

const generatedRuntimeVariableHoverItems: MonacoHoverDefinition[] = [
  {
    target: "MAX_CACHE_SIZE",
    signature: "const MAX_CACHE_SIZE: number",
    documentation: `${generatedFactorRuntimeNotice} 可视化模板默认缓存最近 96 根 K 线收盘价。`,
  },
  {
    target: "state",
    signature: "const state: { closes: number[]; prevFastAverage?: number | null; prevSlowAverage?: number | null }",
    documentation: `${generatedFactorRuntimeNotice} 模板生成时挂在脚本顶层的轻量状态容器。`,
  },
  {
    target: "state.closes",
    signature: "state.closes: number[]",
    documentation: `${generatedFactorRuntimeNotice} 最近一段收盘价缓存，RSI / MA / MACD / Bollinger helper 都会读取它。`,
  },
  {
    target: "state.prevFastAverage",
    signature: "state.prevFastAverage: number | null",
    documentation: `${generatedFactorRuntimeNotice} 上一根 K 线计算出的快均线值，用于金叉 / 死叉判定。`,
  },
  {
    target: "state.prevSlowAverage",
    signature: "state.prevSlowAverage: number | null",
    documentation: `${generatedFactorRuntimeNotice} 上一根 K 线计算出的慢均线值，用于金叉 / 死叉判定。`,
  },
  {
    target: "close",
    signature: "const close: number",
    documentation: `${generatedFactorRuntimeNotice} 模板默认把 ctx.kline.close 提前提取为局部变量，供条件块和日志块复用。`,
  },
  {
    target: "fastAverage",
    signature: "let fastAverage: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前快均线值，由 movingAverageFast 块生成。`,
  },
  {
    target: "slowAverage",
    signature: "let slowAverage: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前慢均线值，由 movingAverageSlow 块生成。`,
  },
  {
    target: "prevFastAverage",
    signature: "const prevFastAverage: number | null",
    documentation: `${generatedFactorRuntimeNotice} 从 state.prevFastAverage 读取出的上一根快均线值。`,
  },
  {
    target: "prevSlowAverage",
    signature: "const prevSlowAverage: number | null",
    documentation: `${generatedFactorRuntimeNotice} 从 state.prevSlowAverage 读取出的上一根慢均线值。`,
  },
  {
    target: "latestRsi",
    signature: "let latestRsi: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 RSI 计算结果，由 rsi 块生成，ifRsiAbove / ifRsiBelow 会直接消费。`,
  },
  {
    target: "latestMacd",
    signature: "let latestMacd: JFTradeMACDIndicatorSnapshot | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 MACD 整体结果对象，由 macd 块生成。`,
  },
  {
    target: "latestMacdDiff",
    signature: "let latestMacdDiff: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 MACD diff 值。`,
  },
  {
    target: "latestMacdSignal",
    signature: "let latestMacdSignal: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 MACD signal 值。`,
  },
  {
    target: "latestMacdHistogram",
    signature: "let latestMacdHistogram: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 MACD histogram 值。`,
  },
  {
    target: "latestKdj",
    signature: "let latestKdj: { k: number; d: number; j: number; previousK?: number; previousD?: number; previousJ?: number } | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 KDJ 结果对象，由 kdj 块生成。`,
  },
  {
    target: "latestKValue",
    signature: "let latestKValue: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 KDJ 的 K 值。`,
  },
  {
    target: "latestDValue",
    signature: "let latestDValue: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 KDJ 的 D 值。`,
  },
  {
    target: "latestJValue",
    signature: "let latestJValue: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 KDJ 的 J 值。`,
  },
  {
    target: "previousKValue",
    signature: "let previousKValue: number | null",
    documentation: `${generatedFactorRuntimeNotice} 上一根 K 线的 K 值，用于 KDJ 金叉/死叉判定。`,
  },
  {
    target: "previousDValue",
    signature: "let previousDValue: number | null",
    documentation: `${generatedFactorRuntimeNotice} 上一根 K 线的 D 值，用于 KDJ 金叉/死叉判定。`,
  },
  {
    target: "latestAtr",
    signature: "let latestAtr: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 ATR 值，由 atr 块生成。`,
  },
  {
    target: "latestCci",
    signature: "let latestCci: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 CCI 值，由 cci 块生成。`,
  },
  {
    target: "latestWilliamsR",
    signature: "let latestWilliamsR: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前 Williams %R 值，由 williamsR 块生成。`,
  },
  {
    target: "latestBollinger",
    signature: "let latestBollinger: { middle: number; upper: number; lower: number } | null",
    documentation: `${generatedFactorRuntimeNotice} 当前布林带结果对象，由 bollinger 块生成。`,
  },
  {
    target: "latestBollingerMiddle",
    signature: "let latestBollingerMiddle: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前布林带中轨值。`,
  },
  {
    target: "latestBollingerUpper",
    signature: "let latestBollingerUpper: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前布林带上轨值。`,
  },
  {
    target: "latestBollingerLower",
    signature: "let latestBollingerLower: number | null",
    documentation: `${generatedFactorRuntimeNotice} 当前布林带下轨值。`,
  },
];

export const strategyEditorHoverItems: MonacoHoverDefinition[] = [
  ...coreHoverItems,
  ...runtimeHostHoverItems,
  ...generatedHelperHoverItems,
  ...generatedRuntimeVariableHoverItems,
];