import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";

import {
  getStrategyBlockKind,
  type StrategyBlockKind,
} from "./strategyVisualBuilderCatalog";
import {
  normalizeTechnicalIndicatorProperties,
  type TechnicalIndicatorBlockProperties,
} from "./strategyVisualBuilderIndicatorBlock";
import {
  createDefaultStrategyVisualModel,
  createDoubleMovingAverageStrategyVisualModel,
} from "./strategyVisualBuilderModels";
import {
  buildAtrIndicatorKey,
  buildBollingerIndicatorKey,
  buildCciIndicatorKey,
  buildDivergenceIndicatorKey,
  buildHookPrelude,
  buildKdjIndicatorKey,
  buildMacdIndicatorKey,
  buildMovingAverageIndicatorKey,
  buildRsiIndicatorKey,
  buildScriptRuntimeBlocks,
  buildWilliamsRIndicatorKey,
  normalizeDecimal,
  normalizeMessage,
  normalizeOrderSide,
  normalizeOrderType,
  normalizeQuantityMode,
  normalizeThreshold,
  orderSideForExchange,
  orderSideLabel,
  toConsoleLogArgument,
  toScriptMessage,
  type StrategyScriptRuntimeFlags,
} from "./strategyVisualBuilderScriptSupport";
import {
  buildStrategyFlowNodeJsDoc,
  cloneStrategyVisualModel,
} from "./strategyVisualBuilderShared";

export interface StrategyScriptContext {
  name: string;
  symbol: string;
  interval: string;
}

export function buildStrategyScriptFromVisualModel(
  model: StrategyVisualModelDocument | null | undefined,
  context: StrategyScriptContext,
): string {
  const normalizedModel =
    cloneStrategyVisualModel(model) ?? createDefaultStrategyVisualModel();
  const technicalIndicators = normalizedModel.nodes
    .filter((node) => getStrategyBlockKind(node) === "technicalIndicator")
    .map((node) => normalizeTechnicalIndicatorProperties(node.properties ?? {}));

  const runtimeFlags: StrategyScriptRuntimeFlags = {
    usesMovingAverageRuntime: technicalIndicators.some(
      (indicator) => indicator.indicatorType === "movingAverage",
    ),
    usesRSIRuntime: technicalIndicators.some(
      (indicator) => indicator.indicatorType === "rsi",
    ),
    usesMACDRuntime: technicalIndicators.some(
      (indicator) => indicator.indicatorType === "macd",
    ),
    usesKDJRuntime: technicalIndicators.some(
      (indicator) => indicator.indicatorType === "kdj",
    ),
    usesATRRuntime: technicalIndicators.some(
      (indicator) => indicator.indicatorType === "atr",
    ),
    usesCCIRuntime: technicalIndicators.some(
      (indicator) => indicator.indicatorType === "cci",
    ),
    usesWilliamsRRuntime: technicalIndicators.some(
      (indicator) => indicator.indicatorType === "williamsR",
    ),
    usesBollingerRuntime: technicalIndicators.some(
      (indicator) => indicator.indicatorType === "bollinger",
    ),
    usesSimpleMovingAverageHelper: technicalIndicators.some(
      (indicator) =>
        indicator.indicatorType === "movingAverage" ||
        indicator.indicatorType === "bollinger",
    ),
    usesSeriesStateRuntime: technicalIndicators.length > 0,
    usesDivergenceRuntime: technicalIndicators.some(
      (indicator) =>
        indicator.conditionMode === "pattern" &&
        (indicator.patternType === "topDivergence" || indicator.patternType === "bottomDivergence"),
    ),
  };

  const nodeById = new Map(
    normalizedModel.nodes.map((node) => [node.id, node] as const),
  );
  const outgoingById = new Map<string, StrategyVisualEdgeDocument[]>();

  for (const edge of normalizedModel.edges) {
    const bucket = outgoingById.get(edge.sourceNodeId) ?? [];
    bucket.push(edge);
    outgoingById.set(edge.sourceNodeId, bucket);
  }

  const sortNodeIds = (nodeIds: string[]): string[] =>
    [...nodeIds].sort((leftId, rightId) => {
      const left = nodeById.get(leftId);
      const right = nodeById.get(rightId);
      if (left === undefined || right === undefined) {
        return leftId.localeCompare(rightId);
      }
      if (left.y === right.y) {
        return left.x - right.x;
      }
      return left.y - right.y;
    });

  const outgoingTargets = (nodeId: string): StrategyVisualNodeDocument[] => {
    const edges = outgoingById.get(nodeId) ?? [];
    const targetIds = edges
      .map((edge) => edge.targetNodeId)
      .filter((targetId) => nodeById.has(targetId));
    return sortNodeIds(targetIds)
      .map((targetId) => nodeById.get(targetId))
      .filter(
        (node): node is StrategyVisualNodeDocument => node !== undefined,
      );
  };

  const renderChildren = (
    nodeId: string,
    visited: Set<string>,
    depth: number,
  ): string[] => {
    const lines: string[] = [];
    for (const child of outgoingTargets(nodeId)) {
      if (visited.has(child.id)) {
        lines.push(`${indent(depth)}// Skipped cyclic block ${child.text}`);
        continue;
      }
      const nextVisited = new Set(visited);
      nextVisited.add(child.id);
      lines.push(...renderNode(child, nextVisited, depth));
    }
    return lines;
  };

  const renderNode = (
    node: StrategyVisualNodeDocument,
    visited: Set<string>,
    depth: number,
  ): string[] => {
    const kind = getStrategyBlockKind(node);
    const nodeProperties = node.properties ?? {};
    const withFlowAnnotation = (lines: string[]): string[] => {
      const annotationLines = buildStrategyFlowNodeJsDoc(node, depth);
      return annotationLines.length === 0 ? lines : [...annotationLines, ...lines];
    };

    switch (kind) {
      case "log": {
        const message = normalizeMessage(
          nodeProperties.message,
          "观察到新的策略事件",
        );
        return withFlowAnnotation([
          `${indent(depth)}console.log(${toConsoleLogArgument(message)});`,
          ...renderChildren(node.id, visited, depth),
        ]);
      }
      case "notify": {
        const message = normalizeMessage(
          nodeProperties.message,
          "策略条件命中，准备处理后续动作",
        );
        return withFlowAnnotation([
          `${indent(depth)}notify(${toScriptMessage(message)});`,
          ...renderChildren(node.id, visited, depth),
        ]);
      }
      case "placeOrder": {
        return withFlowAnnotation([
          ...renderPlaceOrderNode(nodeProperties, depth),
          ...renderChildren(node.id, visited, depth),
        ]);
      }
      case "codeBlock": {
        const code = normalizeCodeBlock(
          nodeProperties.code,
          'console.log("补充自定义逻辑");',
        );
        const codeLines = indentCodeBlock(code, depth);
        if (codeLines.length === 0) {
          return withFlowAnnotation(renderChildren(node.id, visited, depth));
        }
        return withFlowAnnotation([
          `${indent(depth)}// @jftradeCodeBlockBegin`,
          ...codeLines,
          `${indent(depth)}// @jftradeCodeBlockEnd`,
          ...renderChildren(node.id, visited, depth),
        ]);
      }
      case "technicalIndicator": {
        return withFlowAnnotation(
          renderTechnicalIndicatorNode(node, visited, depth, renderChildren),
        );
      }
      case "ifCloseAbove":
      case "ifCloseBelow": {
        const threshold = normalizeThreshold(nodeProperties.threshold, 500);
        const operator = kind === "ifCloseAbove" ? ">" : "<";
        const body = renderChildren(node.id, visited, depth + 1);
        return withFlowAnnotation([
          `${indent(depth)}if (ctx.kline.close ${operator} ${threshold}) {`,
          ...(body.length > 0
            ? body
            : [`${indent(depth + 1)}// Add action blocks after this condition.`]),
          `${indent(depth)}}`,
        ]);
      }
      default:
        return renderChildren(node.id, visited, depth);
    }
  };

  const renderHook = (kind: StrategyBlockKind, hookName: string): string[] => {
    const rootNodes = sortNodeIds(
      normalizedModel.nodes
        .filter((node) => getStrategyBlockKind(node) === kind)
        .map((node) => node.id),
    )
      .map((nodeId) => nodeById.get(nodeId))
      .filter(
        (node): node is StrategyVisualNodeDocument => node !== undefined,
      );

    const bodyLines = rootNodes.flatMap((node) =>
      renderChildren(node.id, new Set([node.id]), 1),
    );
    const hookPrelude = buildHookPrelude(kind, runtimeFlags);
    const hookContextType =
      kind === "onInit"
        ? "JFTradeInitContext"
        : "JFTradeKLineClosedContext";

    return [
      `/** @param {${hookContextType}} ctx */`,
      `function ${hookName}(ctx) {`,
      ...hookPrelude,
      ...(hookPrelude.length > 0 && bodyLines.length > 0 ? [""] : []),
      ...(bodyLines.length > 0
        ? bodyLines
        : [`${indent(1)}// Add visual blocks for this lifecycle hook.`]),
      `}`,
    ];
  };

  const globalCodeBlocks = sortNodeIds(
    normalizedModel.nodes
      .filter((node) => {
        if (getStrategyBlockKind(node) !== "codeBlock") {
          return false;
        }
        return node.properties.codeScope === "global";
      })
      .map((node) => node.id),
  )
    .map((nodeId) => nodeById.get(nodeId))
    .filter(
      (node): node is StrategyVisualNodeDocument => node !== undefined,
    )
    .flatMap((node) => renderNode(node, new Set([node.id]), 0));

  return [
    `// Generated by the Logic Flow visual builder for ${context.name || context.symbol || "QuickJS Strategy"}.`,
    `// Symbol ${context.symbol || "N/A"}, interval ${context.interval || "1m"}.`,
    `// You can keep editing below, or switch back to the visual builder and resync.`,
    "",
    ...buildScriptRuntimeBlocks(runtimeFlags),
    ...(globalCodeBlocks.length > 0 ? [...globalCodeBlocks, ""] : []),
    ...renderHook("onInit", "onInit"),
    "",
    ...renderHook("onKLineClosed", "onKLineClosed"),
  ].join("\n");
}

export function buildDoubleMovingAverageScript(
  context: StrategyScriptContext,
): string {
  return buildStrategyScriptFromVisualModel(
    createDoubleMovingAverageStrategyVisualModel(),
    context,
  );
}

function renderTechnicalIndicatorNode(
  node: StrategyVisualNodeDocument,
  visited: Set<string>,
  depth: number,
  renderChildren: (nodeId: string, visited: Set<string>, depth: number) => string[],
): string[] {
  const properties = normalizeTechnicalIndicatorProperties(node.properties ?? {});
  const setupLines = buildTechnicalIndicatorSetupLines(properties, depth);

  if (properties.conditionMode === "none") {
    return [...setupLines, ...renderChildren(node.id, visited, depth)];
  }

  const body = renderChildren(node.id, visited, depth + 1);
  const fallbackBody = [`${indent(depth + 1)}// Add action blocks after this indicator condition.`];

  if (properties.conditionMode === "numeric") {
    const targetValue = numericTargetExpression(properties);
    return [
      ...setupLines,
      `${indent(depth)}if (${targetValue} ${properties.operator ?? ">"} ${properties.threshold ?? 0}) {`,
      ...(body.length > 0 ? body : fallbackBody),
      `${indent(depth)}}`,
    ];
  }

  return [
    ...setupLines,
    ...buildPatternConditionLines(properties, depth, body.length > 0 ? body : fallbackBody),
  ];
}

function buildTechnicalIndicatorSetupLines(
  properties: TechnicalIndicatorBlockProperties,
  depth: number,
): string[] {
  switch (properties.indicatorType) {
    case "movingAverage": {
      const fastKey = buildMovingAverageIndicatorKey(properties.fastPeriod ?? 5);
      const slowKey = buildMovingAverageIndicatorKey(properties.slowPeriod ?? 20);
      return [
        `${indent(depth)}fastAverageSnapshot = ctx.indicators[${JSON.stringify(fastKey)}] ?? null;`,
        `${indent(depth)}slowAverageSnapshot = ctx.indicators[${JSON.stringify(slowKey)}] ?? null;`,
        `${indent(depth)}fastAverage = fastAverageSnapshot ? fastAverageSnapshot.value ?? null : null;`,
        `${indent(depth)}slowAverage = slowAverageSnapshot ? slowAverageSnapshot.value ?? null : null;`,
        `${indent(depth)}prevFastAverage = fastAverageSnapshot ? fastAverageSnapshot.previous ?? null : null;`,
        `${indent(depth)}prevSlowAverage = slowAverageSnapshot ? slowAverageSnapshot.previous ?? null : null;`,
        `${indent(depth)}if (fastAverage === null || slowAverage === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator moving averages");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      ];
    }
    case "rsi": {
      const indicatorKey = buildRsiIndicatorKey(properties.period ?? 14);
      return [
        `${indent(depth)}latestRsi = ctx.indicators[${JSON.stringify(indicatorKey)}] ?? null;`,
        `${indent(depth)}if (latestRsi === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${indicatorKey}");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      ];
    }
    case "macd": {
      const indicatorKey = buildMacdIndicatorKey(
        properties.fastPeriod ?? 12,
        properties.slowPeriod ?? 26,
        properties.signalPeriod ?? 9,
      );
      return [
        `${indent(depth)}latestMacd = ctx.indicators[${JSON.stringify(indicatorKey)}] ?? null;`,
        `${indent(depth)}if (latestMacd === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${indicatorKey}");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
        `${indent(depth)}latestMacdDiff = latestMacd.diff;`,
        `${indent(depth)}latestMacdSignal = latestMacd.signal;`,
        `${indent(depth)}latestMacdHistogram = latestMacd.histogram;`,
      ];
    }
    case "kdj": {
      const indicatorKey = buildKdjIndicatorKey(
        properties.period ?? 9,
        properties.m1 ?? 3,
        properties.m2 ?? 3,
      );
      return [
        `${indent(depth)}latestKdj = ctx.indicators[${JSON.stringify(indicatorKey)}] ?? null;`,
        `${indent(depth)}if (latestKdj === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${indicatorKey}");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
        `${indent(depth)}latestKValue = latestKdj.k;`,
        `${indent(depth)}latestDValue = latestKdj.d;`,
        `${indent(depth)}latestJValue = latestKdj.j;`,
        `${indent(depth)}previousKValue = latestKdj.previousK ?? null;`,
        `${indent(depth)}previousDValue = latestKdj.previousD ?? null;`,
      ];
    }
    case "atr": {
      const indicatorKey = buildAtrIndicatorKey(properties.period ?? 14);
      return [
        `${indent(depth)}latestAtr = ctx.indicators[${JSON.stringify(indicatorKey)}] ?? null;`,
        `${indent(depth)}if (latestAtr === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${indicatorKey}");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      ];
    }
    case "cci": {
      const indicatorKey = buildCciIndicatorKey(properties.period ?? 20);
      return [
        `${indent(depth)}latestCci = ctx.indicators[${JSON.stringify(indicatorKey)}] ?? null;`,
        `${indent(depth)}if (latestCci === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${indicatorKey}");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      ];
    }
    case "williamsR": {
      const indicatorKey = buildWilliamsRIndicatorKey(properties.period ?? 14);
      return [
        `${indent(depth)}latestWilliamsR = ctx.indicators[${JSON.stringify(indicatorKey)}] ?? null;`,
        `${indent(depth)}if (latestWilliamsR === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${indicatorKey}");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      ];
    }
    case "bollinger": {
      const indicatorKey = buildBollingerIndicatorKey(
        properties.period ?? 20,
        properties.multiplier ?? 2,
      );
      return [
        `${indent(depth)}latestBollinger = ctx.indicators[${JSON.stringify(indicatorKey)}] ?? null;`,
        `${indent(depth)}if (latestBollinger === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${indicatorKey}");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
        `${indent(depth)}latestBollingerMiddle = latestBollinger.middle;`,
        `${indent(depth)}latestBollingerUpper = latestBollinger.upper;`,
        `${indent(depth)}latestBollingerLower = latestBollinger.lower;`,
      ];
    }
    default:
      return [];
  }
}

function buildPatternConditionLines(
  properties: TechnicalIndicatorBlockProperties,
  depth: number,
  body: string[],
): string[] {
  switch (properties.indicatorType) {
    case "movingAverage": {
      const previousOperator = properties.patternType === "deathCross" ? ">=" : "<=";
      const currentOperator = properties.patternType === "deathCross" ? "<" : ">";
      return [
        `${indent(depth)}if (prevFastAverage === null || prevSlowAverage === null) {`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
        `${indent(depth)}if (prevFastAverage ${previousOperator} prevSlowAverage && fastAverage ${currentOperator} slowAverage) {`,
        ...body,
        `${indent(depth)}}`,
      ];
    }
    case "macd": {
      if (properties.patternType === "topDivergence" || properties.patternType === "bottomDivergence") {
        const key = buildDivergenceIndicatorKey(
          "macd",
          [properties.fastPeriod ?? 12, properties.slowPeriod ?? 26, properties.signalPeriod ?? 9],
          properties.patternType === "topDivergence" ? "top" : "bottom",
          properties.lookback ?? 5,
        );
        return [
          `${indent(depth)}divergenceSignal = ctx.indicators[${JSON.stringify(key)}] ?? false;`,
          `${indent(depth)}if (divergenceSignal) {`,
          ...body,
          `${indent(depth)}}`,
        ];
      }
      const previousOperator = properties.patternType === "deathCross" ? ">=" : "<=";
      const currentOperator = properties.patternType === "deathCross" ? "<" : ">";
      return [
        `${indent(depth)}if (latestMacd.previousDiff === undefined || latestMacd.previousSignal === undefined) {`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
        `${indent(depth)}if (latestMacd.previousDiff ${previousOperator} latestMacd.previousSignal && latestMacdDiff ${currentOperator} latestMacdSignal) {`,
        ...body,
        `${indent(depth)}}`,
      ];
    }
    case "kdj": {
      if (properties.patternType === "topDivergence" || properties.patternType === "bottomDivergence") {
        const key = buildDivergenceIndicatorKey(
          "kdj",
          [properties.period ?? 9, properties.m1 ?? 3, properties.m2 ?? 3],
          properties.patternType === "topDivergence" ? "top" : "bottom",
          properties.lookback ?? 5,
        );
        return [
          `${indent(depth)}divergenceSignal = ctx.indicators[${JSON.stringify(key)}] ?? false;`,
          `${indent(depth)}if (divergenceSignal) {`,
          ...body,
          `${indent(depth)}}`,
        ];
      }
      const previousOperator = properties.patternType === "deathCross" ? ">=" : "<=";
      const currentOperator = properties.patternType === "deathCross" ? "<" : ">";
      return [
        `${indent(depth)}if (previousKValue === null || previousDValue === null) {`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
        `${indent(depth)}if (previousKValue ${previousOperator} previousDValue && latestKValue ${currentOperator} latestDValue) {`,
        ...body,
        `${indent(depth)}}`,
      ];
    }
    case "rsi": {
      const key = buildDivergenceIndicatorKey(
        "rsi",
        [properties.period ?? 14],
        properties.patternType === "topDivergence" ? "top" : "bottom",
        properties.lookback ?? 5,
      );
      return [
        `${indent(depth)}divergenceSignal = ctx.indicators[${JSON.stringify(key)}] ?? false;`,
        `${indent(depth)}if (divergenceSignal) {`,
        ...body,
        `${indent(depth)}}`,
      ];
    }
    case "bollinger": {
      const operator = properties.patternType === "closeAboveUpperBand" ? ">" : "<";
      const boundary = properties.patternType === "closeAboveUpperBand"
        ? "latestBollingerUpper"
        : "latestBollingerLower";
      return [
        `${indent(depth)}if (close ${operator} ${boundary}) {`,
        ...body,
        `${indent(depth)}}`,
      ];
    }
    default:
      return body;
  }
}

function numericTargetExpression(properties: TechnicalIndicatorBlockProperties): string {
  switch (properties.indicatorType) {
    case "rsi":
      return "latestRsi";
    case "macd":
      return "latestMacdHistogram";
    case "kdj":
      return "latestJValue";
    case "atr":
      return "latestAtr";
    case "cci":
      return "latestCci";
    case "williamsR":
      return "latestWilliamsR";
    default:
      return "0";
  }
}

function renderPlaceOrderNode(
  nodeProperties: Record<string, unknown>,
  depth: number,
): string[] {
  const visualSide = normalizeOrderSide(nodeProperties.side);
  const exchangeSide = orderSideForExchange(visualSide);
  const sideLabel = orderSideLabel(visualSide);
  const orderType = normalizeOrderType(nodeProperties.orderType);
  const quantityMode = normalizeQuantityMode(nodeProperties.quantityMode);
  const quantityValue = normalizeDecimal(nodeProperties.quantityValue, 100);
  const limitPrice = normalizeDecimal(nodeProperties.limitPrice, 0);
  const orderProps = [`side: "${exchangeSide}"`, `orderType: "${orderType}"`];
  if (orderType === "LIMIT" && limitPrice > 0) {
    orderProps.push(`limitPrice: ${limitPrice}`);
  }

  const lines: string[] = [];
  switch (visualSide) {
    case "BUY":
      lines.push(
        `${indent(depth)}const pos = getPosition();`,
        `${indent(depth)}if (pos && pos.direction === "LONG" && pos.quantity > 0) {`,
        `${indent(depth + 1)}console.log("已有多头持仓 " + pos.quantity + " 股，跳过重复开多");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    case "SELL":
      lines.push(
        `${indent(depth)}const pos = getPosition();`,
        `${indent(depth)}if (!pos || pos.direction !== "LONG" || pos.availableQuantity <= 0) {`,
        `${indent(depth + 1)}console.log("无多头持仓可平，跳过卖出");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    case "SELL_SHORT":
      lines.push(
        `${indent(depth)}const pos = getPosition();`,
        `${indent(depth)}if (pos && pos.direction === "SHORT" && pos.quantity > 0) {`,
        `${indent(depth + 1)}console.log("已有空头持仓 " + pos.quantity + " 股，跳过重复开空");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    case "BUY_COVER":
      lines.push(
        `${indent(depth)}const pos = getPosition();`,
        `${indent(depth)}if (!pos || pos.direction !== "SHORT" || pos.availableQuantity <= 0) {`,
        `${indent(depth + 1)}console.log("无空头持仓可平，跳过买入平空");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    default:
      break;
  }

  switch (quantityMode) {
    case "shares":
      lines.push(`${indent(depth)}const orderQty = ${quantityValue};`);
      break;
    case "amount":
      lines.push(
        `${indent(depth)}const orderPrice = ctx.kline.close;`,
        `${indent(depth)}const maxQty = Math.floor(${quantityValue} / orderPrice);`,
        `${indent(depth)}if (maxQty <= 0) {`,
        `${indent(depth + 1)}console.log("金额 ${quantityValue} 不足以购买 1 股（当前价格 " + orderPrice + "），跳过下单");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
        `${indent(depth)}const orderQty = maxQty;`,
      );
      break;
    case "positionPercent":
      lines.push(
        `${indent(depth)}const orderPrice = ctx.kline.close;`,
        `${indent(depth)}const targetValue = (pos && pos.marketValue > 0 ? pos.marketValue : 0) * ${quantityValue} / 100;`,
        `${indent(depth)}const orderQty = targetValue > 0 ? Math.floor(targetValue / orderPrice) : 0;`,
        `${indent(depth)}if (orderQty <= 0) {`,
        `${indent(depth + 1)}console.log("仓位百分比计算所得数量为 0，跳过下单");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    case "cashPercent":
      lines.push(
        `${indent(depth)}const orderPrice = ctx.kline.close;`,
        `${indent(depth)}const availableCash = getAvailableCash();`,
        `${indent(depth)}const targetAmount = availableCash * ${quantityValue} / 100;`,
        `${indent(depth)}const orderQty = targetAmount > 0 ? Math.floor(targetAmount / orderPrice) : 0;`,
        `${indent(depth)}if (orderQty <= 0) {`,
        `${indent(depth + 1)}console.log("现金百分比计算所得数量为 0（可用资金 " + availableCash + " × ${quantityValue}% ÷ 价格 " + orderPrice + "），请调整百分比或确认账户资金充足");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    default:
      lines.push(`${indent(depth)}const orderQty = ${quantityValue};`);
      break;
  }

  orderProps.push("quantity: orderQty");
  lines.push(
    `${indent(depth)}console.log(\`下单 \${orderQty} 股 ${sideLabel} (${quantityMode})\`);`,
    `${indent(depth)}placeOrder({ ${orderProps.join(", ")} });`,
  );
  return lines;
}

function indent(depth: number): string {
  return "  ".repeat(depth);
}

function normalizeCodeBlock(
  value: unknown,
  fallback: string,
): string {
  if (typeof value !== "string") {
    return fallback;
  }
  const normalized = value.trim();
  return normalized === "" ? fallback : normalized;
}

function indentCodeBlock(code: string, depth: number): string[] {
  return code
    .split(/\r?\n/)
    .map((line) => line.trimEnd())
    .filter((line, index, lines) => line !== "" || lines.length === 1 || index < lines.length - 1)
    .map((line) => `${indent(depth)}${line}`);
}
