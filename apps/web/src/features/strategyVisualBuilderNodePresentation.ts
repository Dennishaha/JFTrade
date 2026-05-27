export const STRATEGY_LOGIC_FLOW_CARD_NODE_TYPE = "strategy-html-card";

export const STRATEGY_LOGIC_FLOW_CONDITION_NODE_TYPE = "strategy-html-condition";

import {
  getTechnicalIndicatorDefinition,
  indicatorTypeLabel,
  isDivergencePattern,
  nextGetTechnicalIndicatorNodeText,
  nextTechnicalIndicatorConditionNodeText,
  nextTechnicalIndicatorNodeText,
  normalizeGetTechnicalIndicatorProperties,
  normalizeTechnicalIndicatorConditionProperties,
  normalizeTechnicalIndicatorProperties,
  patternTypeLabel,
  type TechnicalIndicatorConditionBlockProperties,
  type TechnicalIndicatorType,
} from "./strategyVisualBuilderIndicatorBlock";
import {
  nextStopLossNodeText,
  normalizeStopLossBlockProperties,
  stopLossDirectionLabel,
  stopLossModeLabel,
  stopLossRuleLabel,
  stopLossTimeUnitLabel,
  stopLossWindowPolicyLabel,
} from "./strategyVisualBuilderCatalog";
import {
  normalizeOrderSide,
  normalizeOrderType,
  normalizeQuantityModeForSide,
  orderSideLabel,
} from "./strategyVisualBuilderScriptSupport";

export interface StrategyVisualNodeSummaryInput {
  text?: string | { value?: string } | null | undefined;
  properties?: Record<string, unknown> | null | undefined;
}

export interface StrategyVisualNodeSummaryDetail {
  label: string;
  value: string;
}

export interface StrategyVisualNodeSummary {
  eyebrow: string;
  title: string;
  tone: "default" | "data" | "condition" | "action" | "info" | "alert" | "code";
  variant: "card" | "condition";
  details: StrategyVisualNodeSummaryDetail[];
  chips: string[];
}

export interface StrategyLogicFlowRegistry {
  register: (definition: {
    type: string;
    model: any;
    view: any;
  }) => void;
}

export interface StrategyLogicFlowCoreModule {
  HtmlNode: any;
  HtmlNodeModel: any;
}

const numberFormatter = new Intl.NumberFormat("zh-CN", {
  maximumFractionDigits: 2,
});

export function toStrategyLogicFlowDisplayNodeType(type: string | null | undefined): string {
  if (type === "rect") {
    return STRATEGY_LOGIC_FLOW_CARD_NODE_TYPE;
  }

  if (type === "diamond") {
    return STRATEGY_LOGIC_FLOW_CONDITION_NODE_TYPE;
  }

  return type ?? "rect";
}

export function fromStrategyLogicFlowDisplayNodeType(
  type: string | null | undefined,
): string {
  if (type === STRATEGY_LOGIC_FLOW_CARD_NODE_TYPE) {
    return "rect";
  }

  if (type === STRATEGY_LOGIC_FLOW_CONDITION_NODE_TYPE) {
    return "diamond";
  }

  return type ?? "rect";
}

export function buildStrategyVisualNodeSummary(
  input: StrategyVisualNodeSummaryInput,
  variantOverride?: "card" | "condition",
): StrategyVisualNodeSummary {
  const properties = isRecord(input.properties) ? input.properties : {};
  const blockKind = readString(properties.blockKind);

  switch (blockKind) {
    case "getTechnicalIndicator": {
      const normalized = normalizeGetTechnicalIndicatorProperties(properties);
      return {
        eyebrow: "技术指标",
        title: resolveNodeTitle(input.text, nextGetTechnicalIndicatorNodeText(properties)),
        tone: "data",
        variant: variantOverride ?? "card",
        details: [
          ...(normalized.variableName === undefined
            ? []
            : [{ label: "变量", value: normalized.variableName }]),
          { label: "输出", value: indicatorOutputText(normalized.indicatorType) },
          { label: "用途", value: "供后续条件或动作复用" },
        ],
        chips: ["数据"],
      };
    }
    case "technicalIndicatorCondition": {
      const normalized = normalizeTechnicalIndicatorConditionProperties(properties);
      return {
        eyebrow: "指标条件判断",
        title: resolveNodeTitle(input.text, nextTechnicalIndicatorConditionNodeText(properties)),
        tone: "condition",
        variant: variantOverride ?? "condition",
        details: buildIndicatorConditionDetails(normalized),
        chips: ["True", "False"],
      };
    }
    case "technicalIndicator": {
      const normalized = normalizeTechnicalIndicatorProperties(properties);
      return {
        eyebrow: "技术指标（兼容）",
        title: resolveNodeTitle(input.text, nextTechnicalIndicatorNodeText(properties)),
        tone: "condition",
        variant: variantOverride ?? "card",
        details: buildLegacyIndicatorDetails(normalized),
        chips: ["兼容"],
      };
    }
    case "placeOrder":
      return {
        eyebrow: "交易动作",
        title: resolveNodeTitle(input.text, "下单"),
        tone: "action",
        variant: variantOverride ?? "card",
        details: buildPlaceOrderDetails(properties),
        chips: [],
      };
    case "stopLoss": {
      const normalized = normalizeStopLossBlockProperties(properties);
      return {
        eyebrow: "风控动作",
        title: resolveNodeTitle(input.text, nextStopLossNodeText(properties)),
        tone: "alert",
        variant: variantOverride ?? "card",
        details: [
          { label: "模式", value: stopLossModeLabel(normalized.mode ?? "stopLoss") },
          { label: "方向", value: stopLossDirectionLabel(normalized.direction ?? "auto") },
          { label: "窗口", value: `${formatNumber(normalized.timeValue ?? 1)} ${stopLossTimeUnitLabel(normalized.timeUnit ?? "day")}` },
          { label: "窗口模式", value: stopLossWindowPolicyLabel(normalized.windowPolicy ?? "continuous") },
          { label: "规则", value: stopLossRuleLabel(normalized) },
        ],
        chips: ["平仓"],
      };
    }
    case "log":
      return {
        eyebrow: "运行日志",
        title: resolveNodeTitle(input.text, "输出日志"),
        tone: "info",
        variant: variantOverride ?? "card",
        details: [
          { label: "内容", value: previewText(properties.message, 60, "观察到新的策略事件") },
        ],
        chips: [],
      };
    case "notify":
      return {
        eyebrow: "策略通知",
        title: resolveNodeTitle(input.text, "发送通知"),
        tone: "alert",
        variant: variantOverride ?? "card",
        details: [
          { label: "内容", value: previewText(properties.message, 60, "策略条件命中，准备处理后续动作") },
        ],
        chips: [],
      };
    case "codeBlock":
      return {
        eyebrow: "自定义代码",
        title: resolveNodeTitle(input.text, "代码块"),
        tone: "code",
        variant: variantOverride ?? "card",
        details: [
          { label: "片段", value: previewCodeText(properties.code) },
        ],
        chips: [],
      };
    case "ifCloseAbove":
      return {
        eyebrow: "价格条件",
        title: resolveNodeTitle(input.text, "收盘价 > 阈值"),
        tone: "condition",
        variant: variantOverride ?? "condition",
        details: [
          { label: "规则", value: `close > ${formatNumber(readNumber(properties.threshold, 520))}` },
        ],
        chips: ["True", "False"],
      };
    case "ifCloseBelow":
      return {
        eyebrow: "价格条件",
        title: resolveNodeTitle(input.text, "收盘价 < 阈值"),
        tone: "condition",
        variant: variantOverride ?? "condition",
        details: [
          { label: "规则", value: `close < ${formatNumber(readNumber(properties.threshold, 480))}` },
        ],
        chips: ["True", "False"],
      };
    case "onInit":
      return buildGenericSummary("触发器", input, "策略启动", variantOverride ?? "card", "default");
    case "onKLineClosed":
      return buildGenericSummary("触发器", input, "K 线收盘", variantOverride ?? "card", "default");
    default:
      return buildGenericSummary("图块", input, "未命名图块", variantOverride ?? "card", "default");
  }
}

export function renderStrategyVisualNodeSummary(
  rootEl: SVGForeignObjectElement,
  input: StrategyVisualNodeSummaryInput,
  options: {
    selected?: boolean;
    variant?: "card" | "condition";
  } = {},
): void {
  const summary = buildStrategyVisualNodeSummary(input, options.variant);
  const shell = document.createElement("section");

  shell.className = [
    "strategy-lf-node",
    `strategy-lf-node--${summary.variant}`,
    `strategy-lf-node--tone-${summary.tone}`,
    options.selected ? "strategy-lf-node--selected" : "",
  ].filter(Boolean).join(" ");
  shell.setAttribute("xmlns", "http://www.w3.org/1999/xhtml");
  shell.style.pointerEvents = "none";

  const header = document.createElement("div");
  header.className = "strategy-lf-node__header";

  const eyebrow = document.createElement("div");
  eyebrow.className = "strategy-lf-node__eyebrow";
  eyebrow.textContent = summary.eyebrow;
  header.appendChild(eyebrow);

  const title = document.createElement("div");
  title.className = "strategy-lf-node__title";
  title.textContent = summary.title;
  header.appendChild(title);

  shell.appendChild(header);

  if (summary.details.length > 0) {
    const details = document.createElement("div");
    details.className = "strategy-lf-node__details";

    for (const detail of summary.details.slice(0, 3)) {
      const row = document.createElement("div");
      row.className = "strategy-lf-node__detail";

      const label = document.createElement("span");
      label.className = "strategy-lf-node__detail-label";
      label.textContent = detail.label;

      const value = document.createElement("span");
      value.className = "strategy-lf-node__detail-value";
      value.textContent = detail.value;

      row.append(label, value);
      details.appendChild(row);
    }

    shell.appendChild(details);
  }

  if (summary.chips.length > 0) {
    const chips = document.createElement("div");
    chips.className = "strategy-lf-node__chips";

    for (const chipText of summary.chips) {
      const chip = document.createElement("span");
      chip.className = "strategy-lf-node__chip";
      chip.textContent = chipText;
      chips.appendChild(chip);
    }

    shell.appendChild(chips);
  }

  rootEl.innerHTML = "";
  rootEl.appendChild(shell);
}

export function registerStrategyLogicFlowNodes(
  logicFlow: StrategyLogicFlowRegistry,
  logicFlowCore: StrategyLogicFlowCoreModule,
): void {
  const { HtmlNode, HtmlNodeModel } = logicFlowCore;

  class StrategyHtmlCardNodeModel extends HtmlNodeModel {
    setAttributes(): void {
      super.setAttributes();
      this.width = 224;
      this.height = 136;
    }

    getNodeStyle(): Record<string, unknown> {
      const style = super.getNodeStyle();
      return {
        ...style,
        fill: "transparent",
        stroke: "transparent",
        strokeWidth: 0,
      };
    }
  }

  class StrategyHtmlConditionNodeModel extends HtmlNodeModel {
    setAttributes(): void {
      super.setAttributes();
      this.width = 232;
      this.height = 144;
    }

    getNodeStyle(): Record<string, unknown> {
      const style = super.getNodeStyle();
      return {
        ...style,
        fill: "transparent",
        stroke: "transparent",
        strokeWidth: 0,
      };
    }
  }

  class StrategyHtmlCardNodeView extends HtmlNode {
    shouldUpdate(): boolean {
      const nextSignature = JSON.stringify({
        text: readLogicFlowText(this.props.model?.text),
        properties: this.props.model?.properties,
        selected: this.props.model?.isSelected,
      });

      if (this.preProperties !== nextSignature) {
        this.preProperties = nextSignature;
        return true;
      }

      return false;
    }

    setHtml(rootEl: SVGForeignObjectElement): void {
      renderStrategyVisualNodeSummary(rootEl, {
        text: readLogicFlowText(this.props.model?.text),
        properties: this.props.model?.properties,
      }, {
        selected: Boolean(this.props.model?.isSelected),
        variant: "card",
      });
    }
  }

  class StrategyHtmlConditionNodeView extends HtmlNode {
    shouldUpdate(): boolean {
      const nextSignature = JSON.stringify({
        text: readLogicFlowText(this.props.model?.text),
        properties: this.props.model?.properties,
        selected: this.props.model?.isSelected,
      });

      if (this.preProperties !== nextSignature) {
        this.preProperties = nextSignature;
        return true;
      }

      return false;
    }

    setHtml(rootEl: SVGForeignObjectElement): void {
      renderStrategyVisualNodeSummary(rootEl, {
        text: readLogicFlowText(this.props.model?.text),
        properties: this.props.model?.properties,
      }, {
        selected: Boolean(this.props.model?.isSelected),
        variant: "condition",
      });
    }
  }

  logicFlow.register({
    type: STRATEGY_LOGIC_FLOW_CARD_NODE_TYPE,
    model: StrategyHtmlCardNodeModel,
    view: StrategyHtmlCardNodeView,
  });
  logicFlow.register({
    type: STRATEGY_LOGIC_FLOW_CONDITION_NODE_TYPE,
    model: StrategyHtmlConditionNodeModel,
    view: StrategyHtmlConditionNodeView,
  });
}

function buildIndicatorConditionDetails(
  properties: TechnicalIndicatorConditionBlockProperties,
): StrategyVisualNodeSummaryDetail[] {
  const details: StrategyVisualNodeSummaryDetail[] = [
    {
      label: "来源",
      value: indicatorTypeLabel(properties.indicatorType),
    },
  ];

  if (properties.conditionMode === "numeric") {
    details.push({
      label: "判断",
      value: `${properties.operator ?? "<"} ${formatNumber(properties.threshold ?? 0)}`,
    });
    return details;
  }

  details.push({
    label: "形态",
    value: patternTypeLabel(properties.patternType ?? "goldenCross"),
  });

  if (isDivergencePattern(properties.patternType)) {
    details.push({
      label: "回看",
      value: `${properties.lookback ?? 5} 根`,
    });
  }

  return details;
}

function buildLegacyIndicatorDetails(
  properties: ReturnType<typeof normalizeTechnicalIndicatorProperties>,
): StrategyVisualNodeSummaryDetail[] {
  const details: StrategyVisualNodeSummaryDetail[] = [
    {
      label: "来源",
      value: indicatorTypeLabel(properties.indicatorType),
    },
  ];

  if (properties.conditionMode === "numeric") {
    details.push({
      label: "判断",
      value: `${properties.operator ?? "<"} ${formatNumber(properties.threshold ?? 0)}`,
    });
  } else if (properties.conditionMode === "pattern") {
    details.push({
      label: "形态",
      value: patternTypeLabel(properties.patternType ?? "goldenCross"),
    });
  }

  return details;
}

function buildPlaceOrderDetails(
  properties: Record<string, unknown>,
): StrategyVisualNodeSummaryDetail[] {
  const side = normalizeOrderSide(properties.side);
  const orderType = normalizeOrderType(properties.orderType);
  const quantityMode = normalizeQuantityModeForSide(properties.quantityMode, side);
  const quantityValue = readNumber(properties.quantityValue, 100);
  const limitPrice = readNumber(properties.limitPrice, 0);

  return [
    {
      label: "方向",
      value: orderSideLabel(side),
    },
    {
      label: "委托",
      value: orderType === "LIMIT" && limitPrice > 0
        ? `限价 ${formatNumber(limitPrice)}`
        : "市价",
    },
    {
      label: "数量",
      value: formatQuantity(quantityMode, quantityValue),
    },
  ];
}

function buildGenericSummary(
  eyebrow: string,
  input: StrategyVisualNodeSummaryInput,
  fallbackTitle: string,
  variant: "card" | "condition",
  tone: StrategyVisualNodeSummary["tone"],
): StrategyVisualNodeSummary {
  return {
    eyebrow,
    title: resolveNodeTitle(input.text, fallbackTitle),
    tone,
    variant,
    details: [],
    chips: [],
  };
}

function indicatorOutputText(indicatorType: TechnicalIndicatorType): string {
  switch (indicatorType) {
    case "movingAverage":
      return "快线 / 慢线";
    case "macd":
      return "DIFF / DEA / 柱状图";
    case "kdj":
      return "K / D / J";
    case "bollinger":
      return "上轨 / 中轨 / 下轨";
    default:
      return getTechnicalIndicatorDefinition(indicatorType).numericTargetLabel ?? "指标值";
  }
}

function resolveNodeTitle(
  rawText: StrategyVisualNodeSummaryInput["text"],
  fallback: string,
): string {
  const normalized = previewText(rawText, 56, "");
  return normalized === "" ? fallback : normalized;
}

function previewCodeText(value: unknown): string {
  if (typeof value !== "string") {
    return "console.log(\"补充自定义逻辑\");";
  }

  const firstLine = value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find((line) => line !== "");

  return previewText(firstLine, 60, "console.log(\"补充自定义逻辑\");");
}

function previewText(value: unknown, maxLength: number, fallback: string): string {
  const normalized = normalizeSummaryText(value);
  if (normalized === "") {
    return fallback;
  }

  if (normalized.length <= maxLength) {
    return normalized;
  }

  return `${normalized.slice(0, Math.max(0, maxLength - 1))}…`;
}

function normalizeSummaryText(value: unknown): string {
  if (typeof value === "string") {
    return value.replace(/\s+/g, " ").trim();
  }

  if (isRecord(value) && typeof value.value === "string") {
    return value.value.replace(/\s+/g, " ").trim();
  }

  return "";
}

function readLogicFlowText(value: unknown): string | { value?: string } | undefined {
  if (typeof value === "string") {
    return value;
  }

  if (isRecord(value) && typeof value.value === "string") {
    return { value: value.value };
  }

  return undefined;
}

function readString(value: unknown): string | null {
  return typeof value === "string" ? value : null;
}

function readNumber(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }

  if (typeof value === "string") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) {
      return parsed;
    }
  }

  return fallback;
}

function formatNumber(value: number): string {
  return numberFormatter.format(value);
}

function formatQuantity(
  quantityMode: ReturnType<typeof normalizeQuantityModeForSide>,
  quantityValue: number,
): string {
  switch (quantityMode) {
    case "amount":
      return `金额 ${formatNumber(quantityValue)}`;
    case "accountPositionPercent":
      return `${formatNumber(quantityValue)}% 账户仓位`;
    case "symbolPositionPercent":
      return `${formatNumber(quantityValue)}% 当前标的仓位`;
    case "cashPercent":
      return `${formatNumber(quantityValue)}% 可用现金`;
    case "marginBuyingPowerPercent":
      return `${formatNumber(quantityValue)}% 融资可用`;
    case "shortSellingPowerPercent":
      return `${formatNumber(quantityValue)}% 融券可用`;
    case "shares":
    default:
      return `${formatNumber(quantityValue)} 股`;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}