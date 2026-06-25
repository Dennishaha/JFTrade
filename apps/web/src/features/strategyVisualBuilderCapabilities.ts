import type {
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";

import {
  getStrategyBlockKind,
  normalizeStopLossBlockProperties,
  type StrategyBlockKind,
} from "./strategyVisualBuilderCatalog";
import {
  getTechnicalIndicatorDefinition,
  normalizeGetTechnicalIndicatorProperties,
  normalizeTechnicalIndicatorConditionProperties,
} from "./strategyVisualBuilderIndicatorBlock";
import type { VisualExpressionSchema } from "./strategyVisualBuilderExpressions";

export type PineBlockSupportStatus =
  | "supported"
  | "warning"
  | "unsupportedConfig";

export interface PineBlockSupportAssessment {
  status: PineBlockSupportStatus;
  label: string;
  message: string;
}

export type VisualBlockSupportRule = (
  node: StrategyVisualNodeDocument,
) => PineBlockSupportAssessment;

export interface VisualBlockControlSchema {
  controlIds: string[];
  description: string;
}

export interface VisualBlockControlDescriptor {
  id: string;
  kind: "text" | "number" | "select" | "textarea" | "reference";
  label: string;
}

export interface VisualExpressionReference {
  value: string;
  label: string;
  sourceBlockKind: StrategyBlockKind;
}

export interface VisualExpressionScope {
  references: VisualExpressionReference[];
}

export interface PineRenderRule {
  mode: "native" | "generated" | "runtimeGuard";
  description: string;
}

export interface PineParseRule {
  mode: "annotation" | "expression" | "statement";
  description: string;
}

export interface VisualBlockCapability {
  kind: StrategyBlockKind;
  label: string;
  defaultSupport: PineBlockSupportAssessment;
  controlSchema: VisualBlockControlSchema;
  expressionSchema?: VisualExpressionSchema;
  pineRenderRule: PineRenderRule;
  pineParseRule: PineParseRule;
  supportRule?: VisualBlockSupportRule;
}

const SUPPORTED: PineBlockSupportAssessment = {
  status: "supported",
  label: "可运行",
  message: "该图块会生成当前 JFTrade Pine v6 runtime 支持的闭盘策略语句。",
};

const STRATEGY_BLOCK_CAPABILITY_MAP: Record<StrategyBlockKind, VisualBlockCapability> = {
  onInit: {
    kind: "onInit",
    label: "策略启动",
    defaultSupport: {
      status: "warning",
      label: "闭盘执行",
      message: "Pine v6 没有独立初始化 hook；该入口下的语句会随脚本闭盘执行。",
    },
    controlSchema: {
      controlIds: ["blockTitle"],
      description: "生命周期入口仅允许编辑标题。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "入口内动作按闭盘顺序生成。",
    },
    pineParseRule: {
      mode: "annotation",
      description: "优先读取前端注释，缺失时按脚本顺序恢复。",
    },
  },
  onKLineClosed: {
    kind: "onKLineClosed",
    label: "K 线收盘",
    defaultSupport: SUPPORTED,
    controlSchema: {
      controlIds: ["blockTitle"],
      description: "闭盘入口仅允许编辑标题。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "闭盘入口是策略主执行路径。",
    },
    pineParseRule: {
      mode: "annotation",
      description: "优先读取前端注释，缺失时按脚本顺序恢复。",
    },
  },
  strategyInput: {
    kind: "strategyInput",
    label: "策略参数",
    defaultSupport: {
      status: "supported",
      label: "参数可运行",
      message: "生成 Pine input.* 默认参数；运行时使用默认值参与分析和回测。",
    },
    controlSchema: {
      controlIds: ["variableName", "inputType", "title", "defaultValue"],
      description: "支持 int/float/source/timeframe/time/color 的默认值输入。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "在 strategy 声明之后生成 input.* 参数声明。",
    },
    pineParseRule: {
      mode: "statement",
      description: "支持 input.int/float/source/timeframe/time/color 赋值语句反解。",
    },
  },
  derivedSeries: {
    kind: "derivedSeries",
    label: "派生序列",
    defaultSupport: {
      status: "supported",
      label: "派生可运行",
      message: "生成 history、nz、math、四则表达式或 cross 系列闭盘表达式。",
    },
    controlSchema: {
      controlIds: ["variableName", "mode", "source", "historyOffset", "fallbackValue", "mathFunction", "leftExpression", "operator", "rightExpression", "crossFunction"],
      description: "只允许无副作用标量/序列表达式，不暴露 collection 或 loop。",
    },
    expressionSchema: {
      expressionIds: ["sourceExpressionAst", "leftExpressionAst", "rightExpressionAst", "fallbackExpressionAst"],
      allowedFunctions: ["math.min", "math.max", "math.abs", "math.round", "math.floor", "math.ceil", "nz", "ta.crossover", "ta.crossunder", "ta.cross"],
      allowedOperators: ["+", "-", "*", "/"],
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成命名派生变量，供后续条件、订单或 snippet 引用。",
    },
    pineParseRule: {
      mode: "expression",
      description: "支持 source[n]、nz、math.*、简单四则表达式和 ta.cross* 反解。",
    },
  },
  mtfSeries: {
    kind: "mtfSeries",
    label: "高周期序列",
    defaultSupport: {
      status: "supported",
      label: "MTF 可运行",
      message: "生成同标的静态 timeframe request.security 一阶表达式。",
    },
    controlSchema: {
      controlIds: ["variableName", "timeframe", "expressionType", "source", "historyOffset", "indicatorExpression", "mtfField"],
      description: "仅支持 syminfo.tickerid + 静态 timeframe + source/history/指标表达式，可选择对象字段。",
    },
    expressionSchema: {
      expressionIds: ["indicatorExpressionAst"],
      allowedFunctions: ["math.min", "math.max", "math.abs", "math.round", "math.floor", "math.ceil", "nz", "ta.crossover", "ta.crossunder", "ta.cross", "ta.sma", "ta.ema", "ta.rma", "ta.wma", "ta.hma", "ta.rsi", "ta.macd", "ta.supertrend", "ta.atr"],
      allowedOperators: ["+", "-", "*", "/"],
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 request.security(syminfo.tickerid, timeframe, expression)。",
    },
    pineParseRule: {
      mode: "expression",
      description: "支持 MTF source、history 和可识别指标表达式反解。",
    },
  },
  stateVariable: {
    kind: "stateVariable",
    label: "持久状态",
    defaultSupport: {
      status: "supported",
      label: "状态可运行",
      message: "生成 var 标量状态；closed-bar runtime 下跨 K 线保留。",
    },
    controlSchema: {
      controlIds: ["variableName", "valueType", "initialValue"],
      description: "只支持 number/bool/string 标量状态。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 var name = initial。",
    },
    pineParseRule: {
      mode: "statement",
      description: "支持 var 标量声明反解。",
    },
  },
  stateUpdate: {
    kind: "stateUpdate",
    label: "更新状态",
    defaultSupport: {
      status: "supported",
      label: "状态更新可运行",
      message: "生成 name := expression 标量更新语句。",
    },
    controlSchema: {
      controlIds: ["variableName", "expression"],
      description: "只支持无 collection/loop 副作用的标量表达式。",
    },
    expressionSchema: {
      expressionIds: ["expressionAst"],
      allowedFunctions: ["math.min", "math.max", "math.abs", "math.round", "math.floor", "math.ceil", "nz", "ta.barssince", "ta.valuewhen"],
      allowedOperators: ["+", "-", "*", "/", ">", "<", ">=", "<=", "==", "!=", "and", "or"],
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 name := expression。",
    },
    pineParseRule: {
      mode: "statement",
      description: "支持 := 状态更新反解。",
    },
  },
  collectionStat: {
    kind: "collectionStat",
    label: "集合统计",
    defaultSupport: {
      status: "supported",
      label: "只读统计可运行",
      message: "仅生成 array.from(source...) 的只读统计，不开放可变 collection、loop 或 method 链。",
    },
    controlSchema: {
      controlIds: ["variableName", "statFunction", "sourceA", "sourceB", "sourceC", "percentile"],
      description: "支持 min/max/avg/sum/median/stdev/variance/percentile 的固定 source 列表统计。",
    },
    expressionSchema: {
      expressionIds: ["sourceAExpressionAst", "sourceBExpressionAst", "sourceCExpressionAst"],
      allowedFunctions: ["math.min", "math.max", "math.abs", "math.round", "math.floor", "math.ceil", "nz"],
      allowedOperators: ["+", "-", "*", "/"],
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 array.from(...).stat() 或 percentile_linear_interpolation(percentile)。",
    },
    pineParseRule: {
      mode: "expression",
      description: "支持前端生成的 array.from(...).stat() 只读统计反解。",
    },
  },
  timeFilter: {
    kind: "timeFilter",
    label: "时间过滤",
    defaultSupport: {
      status: "supported",
      label: "时间过滤可运行",
      message: "生成 hour/minute/dayofweek 的 closed-bar 安全条件。",
    },
    controlSchema: {
      controlIds: ["mode", "startHour", "startMinute", "endHour", "endMinute", "dayOfWeek"],
      description: "支持静态日内分钟窗口和 dayofweek 过滤。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 hour/minute/dayofweek 条件。",
    },
    pineParseRule: {
      mode: "expression",
      description: "支持前端生成的时间条件反解。",
    },
  },
  sessionFilter: {
    kind: "sessionFilter",
    label: "交易时段过滤",
    defaultSupport: {
      status: "supported",
      label: "交易时段可运行",
      message: "生成 session.ismarket/ispremarket/ispostmarket 的 closed-bar 安全条件。",
    },
    controlSchema: {
      controlIds: ["scope"],
      description: "仅支持 runtime 已提供的 regular/pre/post session 布尔状态。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 session.* 状态条件。",
    },
    pineParseRule: {
      mode: "expression",
      description: "支持 session.* 状态条件反解。",
    },
  },
  getTechnicalIndicator: {
    kind: "getTechnicalIndicator",
    label: "指标数据",
    defaultSupport: SUPPORTED,
    controlSchema: {
      controlIds: [
        "variableName",
        "indicatorType",
        "source",
        "period",
        "windowSize",
        "timeframe",
        "movingAverageType",
        "macdPeriods",
        "kdjPeriods",
        "multiplier",
        "adxSmoothing",
        "factor",
        "sar",
        "pivotBars",
        "offset",
        "sigma",
      ],
      description: "指标变量编辑器和 Inspector 必须覆盖同一组指标参数。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 JFTrade Pine runtime 支持的指标表达式，可按周期包装 request.security。",
    },
    pineParseRule: {
      mode: "expression",
      description: "支持前端内部函数和常见 ta.* 指标别名。",
    },
    supportRule: assessTechnicalIndicatorSupport,
  },
  technicalIndicatorCondition: {
    kind: "technicalIndicatorCondition",
    label: "指标条件判断",
    defaultSupport: SUPPORTED,
    controlSchema: {
      controlIds: [
        "indicatorType",
        "conditionMode",
        "indicatorInputs",
        "operator",
        "threshold",
        "patternType",
        "lookback",
      ],
      description: "条件控件随指标能力暴露数值或形态判断。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "基于指标变量生成数值、交叉、背离或带状条件。",
    },
    pineParseRule: {
      mode: "expression",
      description: "支持对象字段和常见条件表达式回到标准条件图块。",
    },
    supportRule: assessTechnicalIndicatorConditionSupport,
  },
  seriesCondition: {
    kind: "seriesCondition",
    label: "序列条件判断",
    defaultSupport: SUPPORTED,
    controlSchema: {
      controlIds: [
        "seriesMode",
        "seriesSource",
        "seriesOperator",
        "seriesThreshold",
        "seriesLength",
        "eventSource",
        "eventOperator",
        "eventThreshold",
        "valueSource",
        "occurrence",
      ],
      description: "支持 compare、ta.rising/ta.falling、ta.barssince、ta.valuewhen 的闭盘条件。",
    },
    expressionSchema: {
      expressionIds: ["sourceExpressionAst", "leftExpressionAst", "rightExpressionAst", "eventExpressionAst", "valueExpressionAst"],
      allowedFunctions: ["math.min", "math.max", "math.abs", "math.round", "math.floor", "math.ceil", "nz", "ta.crossover", "ta.crossunder", "ta.cross", "ta.barssince", "ta.valuewhen"],
      allowedOperators: [">", "<", ">=", "<=", "==", "!=", "and", "or", "+", "-", "*", "/"],
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成序列比较、ta.rising、ta.falling、ta.barssince 或 ta.valuewhen 条件。",
    },
    pineParseRule: {
      mode: "expression",
      description: "支持内部函数和 ta.* 别名反解。",
    },
  },
  ifCloseAbove: {
    kind: "ifCloseAbove",
    label: "收盘价高于",
    defaultSupport: SUPPORTED,
    controlSchema: {
      controlIds: ["threshold"],
      description: "仅暴露收盘价比较阈值。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 close > threshold 条件。",
    },
    pineParseRule: {
      mode: "expression",
      description: "可从 close > number 恢复。",
    },
  },
  ifCloseBelow: {
    kind: "ifCloseBelow",
    label: "收盘价低于",
    defaultSupport: SUPPORTED,
    controlSchema: {
      controlIds: ["threshold"],
      description: "仅暴露收盘价比较阈值。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 close < threshold 条件。",
    },
    pineParseRule: {
      mode: "expression",
      description: "可从 close < number 恢复。",
    },
  },
  log: {
    kind: "log",
    label: "输出日志",
    defaultSupport: SUPPORTED,
    controlSchema: {
      controlIds: ["message"],
      description: "日志图块仅编辑消息内容。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 log.info 调用。",
    },
    pineParseRule: {
      mode: "statement",
      description: "可从 log.info 恢复。",
    },
  },
  notify: {
    kind: "notify",
    label: "发送通知",
    defaultSupport: SUPPORTED,
    controlSchema: {
      controlIds: ["message"],
      description: "通知图块仅编辑消息内容。",
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 alert 调用。",
    },
    pineParseRule: {
      mode: "statement",
      description: "可从 alert 恢复。",
    },
  },
  placeOrder: {
    kind: "placeOrder",
    label: "下单",
    defaultSupport: {
      status: "supported",
      label: "订单可运行",
      message: "生成当前 runtime 支持的 strategy.entry/order/close/cancel/risk 语句。",
    },
    controlSchema: {
      controlIds: [
        "orderAction",
        "orderId",
        "side",
        "orderType",
        "entryPositionPolicy",
        "quantityMode",
        "quantityValue",
        "limitPrice",
        "stopPrice",
        "limitPriceExpressionAst",
        "stopPriceExpressionAst",
        "riskAllowedDirection",
      ],
      description: "只暴露当前 closed-bar runtime 可分析的订单动作和价格参数。",
    },
    expressionSchema: {
      expressionIds: ["limitPriceExpressionAst", "stopPriceExpressionAst"],
      allowedFunctions: ["math.min", "math.max", "math.abs", "math.round", "math.floor", "math.ceil", "nz"],
      allowedOperators: ["+", "-", "*", "/"],
    },
    pineRenderRule: {
      mode: "generated",
      description: "生成 strategy.entry、strategy.order、strategy.close、strategy.close_all、strategy.cancel、strategy.cancel_all 或 strategy.risk.allow_entry_in。",
    },
    pineParseRule: {
      mode: "statement",
      description: "支持上述 strategy.* 语句反解。",
    },
  },
  stopLoss: {
    kind: "stopLoss",
    label: "退出/风控",
    defaultSupport: SUPPORTED,
    controlSchema: {
      controlIds: [
        "mode",
        "direction",
        "timeValue",
        "timeUnit",
        "windowPolicy",
        "percentage",
        "takeProfitPercentage",
        "quantityPercentage",
        "stopPriceExpressionAst",
        "takeProfitPriceExpressionAst",
        "trailingPriceExpressionAst",
      ],
      description: "当前仅支持连续窗口 + 1 柱的闭盘退出配置。",
    },
    expressionSchema: {
      expressionIds: ["stopPriceExpressionAst", "takeProfitPriceExpressionAst", "trailingPriceExpressionAst"],
      allowedFunctions: ["math.min", "math.max", "math.abs", "math.round", "math.floor", "math.ceil", "nz"],
      allowedOperators: ["+", "-", "*", "/"],
    },
    pineRenderRule: {
      mode: "runtimeGuard",
      description: "supported 配置生成 strategy.exit；其他窗口配置生成 runtime.error 提示。",
    },
    pineParseRule: {
      mode: "statement",
      description: "支持 strategy.exit 的 stop/limit 参数回到退出图块。",
    },
    supportRule: assessStopLossSupport,
  },
};

export function getVisualBlockCapabilities(): VisualBlockCapability[] {
  return Object.values(STRATEGY_BLOCK_CAPABILITY_MAP);
}

export function getVisualBlockCapability(
  kind: StrategyBlockKind | null | undefined,
): VisualBlockCapability | null {
  if (kind === null || kind === undefined) {
    return null;
  }
  return STRATEGY_BLOCK_CAPABILITY_MAP[kind] ?? null;
}

export function assessPineBlockSupport(
  node: StrategyVisualNodeDocument | null | undefined,
): PineBlockSupportAssessment {
  const kind = getStrategyBlockKind(node);
  const capability = getVisualBlockCapability(kind);
  if (node === null || node === undefined || kind === null || capability === null) {
    return {
      status: "unsupportedConfig",
      label: "未知图块",
      message: "该图块类型无法识别，生成 Pine 时会失败并提示迁移到 Pine v6 标准图块。",
    };
  }
  return capability.supportRule?.(node) ?? capability.defaultSupport;
}

export function summarizePineBlockSupport(model: StrategyVisualModelDocument): {
  unsupportedConfigCount: number;
  warningCount: number;
} {
  const summary = {
    unsupportedConfigCount: 0,
    warningCount: 0,
  };

  for (const node of model.nodes) {
    const assessment = assessPineBlockSupport(node);
    if (assessment.status === "unsupportedConfig") {
      summary.unsupportedConfigCount += 1;
    } else if (assessment.status === "warning") {
      summary.warningCount += 1;
    }
  }

  return summary;
}

function assessTechnicalIndicatorSupport(node: StrategyVisualNodeDocument): PineBlockSupportAssessment {
  const normalized = normalizeGetTechnicalIndicatorProperties(node.properties);
  const definition = getTechnicalIndicatorDefinition(normalized.indicatorType);
  return {
    status: "supported",
    label: "指标可运行",
    message: `生成 ${definition.label} 的 Pine v6 表达式；能力：${definition.capabilityId}。`,
  };
}

function assessTechnicalIndicatorConditionSupport(
  node: StrategyVisualNodeDocument,
): PineBlockSupportAssessment {
  const normalized = normalizeTechnicalIndicatorConditionProperties(node.properties);
  const definition = getTechnicalIndicatorDefinition(normalized.indicatorType);
  return {
    status: "supported",
    label: "条件可运行",
    message: `生成 ${definition.label} 的${normalized.conditionMode === "numeric" ? "数值" : "形态"}条件；能力：${definition.capabilityId}。`,
  };
}

function assessStopLossSupport(node: StrategyVisualNodeDocument): PineBlockSupportAssessment {
  const normalized = normalizeStopLossBlockProperties(node.properties);
  if (
    normalized.windowPolicy === "continuous" &&
    normalized.timeUnit === "bar" &&
    normalized.timeValue === 1
  ) {
    return SUPPORTED;
  }
  return {
    status: "unsupportedConfig",
    label: "配置不支持",
    message: "自动退出图块当前只支持连续窗口 + 1 柱；其他时间窗口会生成 runtime.error。",
  };
}
