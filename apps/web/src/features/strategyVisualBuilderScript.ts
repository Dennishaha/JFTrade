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
  createDefaultStrategyVisualModel,
  createDoubleMovingAverageStrategyVisualModel,
} from "./strategyVisualBuilderModels";
import {
  buildHookPrelude,
  buildScriptRuntimeBlocks,
  normalizeDecimal,
  normalizeMessage,
  normalizeThreshold,
  normalizeWindowSize,
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
  const usesMovingAverageRuntime = normalizedModel.nodes.some((node) => {
    const kind = getStrategyBlockKind(node);
    return (
      kind === "movingAverageFast" ||
      kind === "movingAverageSlow" ||
      kind === "ifGoldenCross" ||
      kind === "ifDeathCross"
    );
  });
  const usesRSIRuntime = normalizedModel.nodes.some((node) => {
    const kind = getStrategyBlockKind(node);
    return kind === "rsi" || kind === "ifRsiAbove" || kind === "ifRsiBelow";
  });
  const usesMACDRuntime = normalizedModel.nodes.some((node) => {
    const kind = getStrategyBlockKind(node);
    return kind === "macd" || kind === "ifMacdBullish" || kind === "ifMacdBearish";
  });
  const usesBollingerRuntime = normalizedModel.nodes.some((node) => {
    const kind = getStrategyBlockKind(node);
    return (
      kind === "bollinger" ||
      kind === "ifCloseAboveUpperBand" ||
      kind === "ifCloseBelowLowerBand"
    );
  });
  const usesSimpleMovingAverageHelper =
    usesMovingAverageRuntime || usesBollingerRuntime;
  const usesSeriesStateRuntime =
    usesMovingAverageRuntime ||
    usesRSIRuntime ||
    usesMACDRuntime ||
    usesBollingerRuntime;
  const runtimeFlags: StrategyScriptRuntimeFlags = {
    usesMovingAverageRuntime,
    usesRSIRuntime,
    usesMACDRuntime,
    usesBollingerRuntime,
    usesSimpleMovingAverageHelper,
    usesSeriesStateRuntime,
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
      case "codeBlock": {
        const code = normalizeCodeBlock(
          nodeProperties.code,
          "console.log(\"补充自定义逻辑\");",
        );
        const codeLines = indentCodeBlock(code, depth);
        if (codeLines.length === 0) {
          return withFlowAnnotation([
            ...renderChildren(node.id, visited, depth),
          ]);
        }
        return withFlowAnnotation([
          `${indent(depth)}// @jftradeCodeBlockBegin`,
          ...codeLines,
          `${indent(depth)}// @jftradeCodeBlockEnd`,
          ...renderChildren(node.id, visited, depth),
        ]);
      }
      case "rsi": {
        const period = normalizeWindowSize(nodeProperties.period, 14);
        return withFlowAnnotation([
          `${indent(depth)}latestRsi = calculateRSI(state.closes, ${period});`,
          `${indent(depth)}if (latestRsi === null) {`,
          `${indent(depth + 1)}console.log(\`waiting for enough candles: ${'${state.closes.length}'}\`);`,
          `${indent(depth + 1)}return;`,
          `${indent(depth)}}`,
          ...renderChildren(node.id, visited, depth),
        ]);
      }
      case "macd": {
        const fastPeriod = normalizeWindowSize(nodeProperties.fastPeriod, 12);
        const slowPeriod = normalizeWindowSize(nodeProperties.slowPeriod, 26);
        const signalPeriod = normalizeWindowSize(nodeProperties.signalPeriod, 9);
        return withFlowAnnotation([
          `${indent(depth)}latestMacd = calculateMACD(state.closes, ${fastPeriod}, ${slowPeriod}, ${signalPeriod});`,
          `${indent(depth)}if (latestMacd === null) {`,
          `${indent(depth + 1)}console.log(\`waiting for enough candles: ${'${state.closes.length}'}\`);`,
          `${indent(depth + 1)}return;`,
          `${indent(depth)}}`,
          `${indent(depth)}latestMacdDiff = latestMacd.diff;`,
          `${indent(depth)}latestMacdSignal = latestMacd.signal;`,
          `${indent(depth)}latestMacdHistogram = latestMacd.histogram;`,
          ...renderChildren(node.id, visited, depth),
        ]);
      }
      case "bollinger": {
        const period = normalizeWindowSize(nodeProperties.period, 20);
        const multiplier = normalizeDecimal(nodeProperties.multiplier, 2);
        return withFlowAnnotation([
          `${indent(depth)}latestBollinger = calculateBollingerBands(state.closes, ${period}, ${multiplier});`,
          `${indent(depth)}if (latestBollinger === null) {`,
          `${indent(depth + 1)}console.log(\`waiting for enough candles: ${'${state.closes.length}'}\`);`,
          `${indent(depth + 1)}return;`,
          `${indent(depth)}}`,
          `${indent(depth)}latestBollingerMiddle = latestBollinger.middle;`,
          `${indent(depth)}latestBollingerUpper = latestBollinger.upper;`,
          `${indent(depth)}latestBollingerLower = latestBollinger.lower;`,
          ...renderChildren(node.id, visited, depth),
        ]);
      }
      case "movingAverageFast": {
        const windowSize = normalizeWindowSize(nodeProperties.windowSize, 5);
        return withFlowAnnotation([
          `${indent(depth)}fastAverage = simpleMovingAverage(state.closes, ${windowSize});`,
          ...renderChildren(node.id, visited, depth),
        ]);
      }
      case "movingAverageSlow": {
        const windowSize = normalizeWindowSize(nodeProperties.windowSize, 20);
        return withFlowAnnotation([
          `${indent(depth)}slowAverage = simpleMovingAverage(state.closes, ${windowSize});`,
          `${indent(depth)}if (fastAverage === null || slowAverage === null) {`,
          `${indent(depth + 1)}console.log(\`waiting for enough candles: ${'${state.closes.length}'}\`);`,
          `${indent(depth + 1)}return;`,
          `${indent(depth)}}`,
          `${indent(depth)}state.prevFastAverage = fastAverage;`,
          `${indent(depth)}state.prevSlowAverage = slowAverage;`,
          ...renderChildren(node.id, visited, depth),
        ]);
      }
      case "ifGoldenCross":
      case "ifDeathCross": {
        const previousOperator = kind === "ifGoldenCross" ? "<=" : ">=";
        const currentOperator = kind === "ifGoldenCross" ? ">" : "<";
        const body = renderChildren(node.id, visited, depth + 1);
        return withFlowAnnotation([
          `${indent(depth)}if (fastAverage === null || slowAverage === null || prevFastAverage === null || prevSlowAverage === null) {`,
          `${indent(depth + 1)}return;`,
          `${indent(depth)}}`,
          `${indent(depth)}if (prevFastAverage ${previousOperator} prevSlowAverage && fastAverage ${currentOperator} slowAverage) {`,
          ...(body.length > 0
            ? body
            : [`${indent(depth + 1)}// Add action blocks after this cross signal.`]),
          `${indent(depth)}}`,
        ]);
      }
      case "ifMacdBullish":
      case "ifMacdBearish": {
        const operator = kind === "ifMacdBullish" ? ">" : "<";
        const body = renderChildren(node.id, visited, depth + 1);
        return withFlowAnnotation([
          `${indent(depth)}if (latestMacdDiff === null || latestMacdSignal === null) {`,
          `${indent(depth + 1)}return;`,
          `${indent(depth)}}`,
          `${indent(depth)}if (latestMacdDiff ${operator} latestMacdSignal) {`,
          ...(body.length > 0
            ? body
            : [`${indent(depth + 1)}// Add action blocks after this MACD condition.`]),
          `${indent(depth)}}`,
        ]);
      }
      case "ifRsiAbove":
      case "ifRsiBelow": {
        const threshold = normalizeThreshold(
          nodeProperties.threshold,
          kind === "ifRsiAbove" ? 70 : 30,
        );
        const operator = kind === "ifRsiAbove" ? ">" : "<";
        const body = renderChildren(node.id, visited, depth + 1);
        return withFlowAnnotation([
          `${indent(depth)}if (latestRsi === null) {`,
          `${indent(depth + 1)}return;`,
          `${indent(depth)}}`,
          `${indent(depth)}if (latestRsi ${operator} ${threshold}) {`,
          ...(body.length > 0
            ? body
            : [`${indent(depth + 1)}// Add action blocks after this RSI condition.`]),
          `${indent(depth)}}`,
        ]);
      }
      case "ifCloseAboveUpperBand":
      case "ifCloseBelowLowerBand": {
        const boundary =
          kind === "ifCloseAboveUpperBand"
            ? "latestBollingerUpper"
            : "latestBollingerLower";
        const operator = kind === "ifCloseAboveUpperBand" ? ">" : "<";
        const body = renderChildren(node.id, visited, depth + 1);
        return withFlowAnnotation([
          `${indent(depth)}if (${boundary} === null) {`,
          `${indent(depth + 1)}return;`,
          `${indent(depth)}}`,
          `${indent(depth)}if (close ${operator} ${boundary}) {`,
          ...(body.length > 0
            ? body
            : [`${indent(depth + 1)}// Add action blocks after this Bollinger condition.`]),
          `${indent(depth)}}`,
        ]);
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
      ...(hookPrelude.length > 0 ? hookPrelude : []),
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