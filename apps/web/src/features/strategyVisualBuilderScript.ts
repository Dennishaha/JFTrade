import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";

import {
  getStrategyBlockKind,
  nextStopLossNodeText,
  normalizeStopLossBlockProperties,
  type StrategyBlockKind,
} from "./strategyVisualBuilderCatalog";
import {
  isDivergencePattern,
  normalizeGetTechnicalIndicatorProperties,
  normalizeTechnicalIndicatorConditionProperties,
  normalizeTechnicalIndicatorProperties,
  type GetTechnicalIndicatorBlockProperties,
  type TechnicalIndicatorBlockProperties,
  type TechnicalIndicatorConditionBlockProperties,
} from "./strategyVisualBuilderIndicatorBlock";
import {
  isStrategyVisualControlEdge,
  isStrategyVisualDataEdge,
  readStrategyVisualEdgeBranch,
  readStrategyVisualEdgeInputSlot,
  type StrategyVisualEdgeBranch,
} from "./strategyVisualBuilderEdges";
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
  buildStopLossIndicatorKey,
  buildScriptRuntimeBlocks,
  buildWilliamsRIndicatorKey,
  entryPositionPolicyLabel,
  normalizeEntryPositionPolicy,
  normalizeDecimal,
  normalizeMessage,
  normalizeOrderSide,
  normalizeOrderType,
  normalizeQuantityModeForSide,
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
import {
  reconcileStrategyVisualModelIndicatorBindings,
} from "./strategyVisualBuilderIndicatorReferences";

interface IndicatorInputBinding {
  edge: StrategyVisualEdgeDocument;
  slot: string;
  node: StrategyVisualNodeDocument;
  properties: GetTechnicalIndicatorBlockProperties;
}

export interface StrategyScriptContext {
  name: string;
  symbol: string;
  interval: string;
}

export function buildStrategyScriptFromVisualModel(
  model: StrategyVisualModelDocument | null | undefined,
  context: StrategyScriptContext,
): string {
  const normalizedModel = reconcileStrategyVisualModelIndicatorBindings(
    cloneStrategyVisualModel(model) ?? createDefaultStrategyVisualModel(),
  );
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
  const incomingById = new Map<string, StrategyVisualEdgeDocument[]>();

  for (const edge of normalizedModel.edges) {
    const outgoingBucket = outgoingById.get(edge.sourceNodeId) ?? [];
    outgoingBucket.push(edge);
    outgoingById.set(edge.sourceNodeId, outgoingBucket);

    const incomingBucket = incomingById.get(edge.targetNodeId) ?? [];
    incomingBucket.push(edge);
    incomingById.set(edge.targetNodeId, incomingBucket);
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
    const edges = (outgoingById.get(nodeId) ?? []).filter((edge) =>
      isStrategyVisualControlEdge(edge) && readStrategyVisualEdgeBranch(edge) === null,
    );
    const targetIds = edges
      .map((edge) => edge.targetNodeId)
      .filter((targetId) => nodeById.has(targetId));
    return sortNodeIds(targetIds)
      .map((targetId) => nodeById.get(targetId))
      .filter(
        (node): node is StrategyVisualNodeDocument => node !== undefined,
      );
  };

  const outgoingBranchTargets = (
    nodeId: string,
    branch: StrategyVisualEdgeBranch,
  ): StrategyVisualNodeDocument[] => {
    const edges = (outgoingById.get(nodeId) ?? []).filter((edge) => {
      if (!isStrategyVisualControlEdge(edge)) {
        return false;
      }
      const edgeBranch = readStrategyVisualEdgeBranch(edge);
      return edgeBranch === branch || (branch === "true" && edgeBranch === null);
    });
    const targetIds = edges
      .map((edge) => edge.targetNodeId)
      .filter((targetId) => nodeById.has(targetId));
    return sortNodeIds(targetIds)
      .map((targetId) => nodeById.get(targetId))
      .filter(
        (node): node is StrategyVisualNodeDocument => node !== undefined,
      );
  };

  const controlReachableNodeIds = collectControlReachableNodeIds(
    normalizedModel.nodes
      .filter((node) => {
        const kind = getStrategyBlockKind(node);
        return kind === "onInit" || kind === "onKLineClosed";
      })
      .map((node) => node.id),
    outgoingById,
    nodeById,
  );

  const incomingIndicatorInputs = (nodeId: string): IndicatorInputBinding[] => {
    const edges = (incomingById.get(nodeId) ?? []).filter((edge) =>
      isStrategyVisualDataEdge(edge),
    );
    const bindings: IndicatorInputBinding[] = [];
    for (const edge of edges) {
      const sourceNode = nodeById.get(edge.sourceNodeId);
      if (sourceNode === undefined || getStrategyBlockKind(sourceNode) !== "getTechnicalIndicator") {
        continue;
      }
      bindings.push({
        edge,
        slot: readStrategyVisualEdgeInputSlot(edge) ?? "primary",
        node: sourceNode,
        properties: normalizeGetTechnicalIndicatorProperties(sourceNode.properties ?? {}),
      });
    }
    return bindings.sort((left, right) => {
        if (left.slot === right.slot) {
          const leftNode = nodeById.get(left.node.id);
          const rightNode = nodeById.get(right.node.id);
          if (leftNode !== undefined && rightNode !== undefined) {
            if (leftNode.y === rightNode.y) {
              return leftNode.x - rightNode.x;
            }
            return leftNode.y - rightNode.y;
          }
          return left.node.id.localeCompare(right.node.id);
        }
        return left.slot.localeCompare(right.slot);
        });
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
      lines.push(`${indent(depth)}${buildFlowFunctionName(child.id)}();`);
    }
    return lines;
  };

  const renderBranchChildren = (
    nodeId: string,
    branch: StrategyVisualEdgeBranch,
    visited: Set<string>,
    depth: number,
  ): string[] => {
    const lines: string[] = [];
    for (const child of outgoingBranchTargets(nodeId, branch)) {
      if (visited.has(child.id)) {
        lines.push(`${indent(depth)}// Skipped cyclic block ${child.text}`);
        continue;
      }
      lines.push(`${indent(depth)}${buildFlowFunctionName(child.id)}();`);
    }
    return lines;
  };

  const readFunctionDependencies = (
    node: StrategyVisualNodeDocument,
  ): StrategyVisualNodeDocument[] => {
    const dependencies: StrategyVisualNodeDocument[] = [];
    const pushUnique = (candidates: StrategyVisualNodeDocument[]) => {
      for (const candidate of candidates) {
        if (!dependencies.some((item) => item.id === candidate.id)) {
          dependencies.push(candidate);
        }
      }
    };

    const kind = getStrategyBlockKind(node);
    if (kind === "technicalIndicatorCondition") {
      pushUnique(
        incomingIndicatorInputs(node.id)
          .map((input) => input.node),
      );
      pushUnique(outgoingBranchTargets(node.id, "true"));
      pushUnique(outgoingBranchTargets(node.id, "false"));
      return dependencies;
    }

    pushUnique(outgoingTargets(node.id));
    return dependencies;
  };

  const renderedFunctionIds = new Set<string>();
  const renderedFunctionNodes: StrategyVisualNodeDocument[] = [];

  const renderNodeDefinition = (
    node: StrategyVisualNodeDocument,
    visited: Set<string>,
    depth: number,
  ): string[] => {
    if (renderedFunctionIds.has(node.id)) {
      return [];
    }
    renderedFunctionIds.add(node.id);
    renderedFunctionNodes.push(node);

    const dependencyLines = readFunctionDependencies(node).flatMap((dependency) => {
      const nextVisited = new Set(visited);
      nextVisited.add(dependency.id);
      return renderNodeDefinition(dependency, nextVisited, depth);
    });

    const kind = getStrategyBlockKind(node);
    const nodeProperties = node.properties ?? {};
    const withFlowAnnotation = (
      lines: string[],
      extra: Parameters<typeof buildStrategyFlowNodeJsDoc>[2] = {},
    ): string[] => {
      const annotationLines = buildStrategyFlowNodeJsDoc(node, depth, extra);
      return annotationLines.length === 0 ? lines : [...annotationLines, ...lines];
    };

    switch (kind) {
      case "log": {
        const message = normalizeMessage(
          nodeProperties.message,
          "观察到新的策略事件",
        );
        return [
          ...dependencyLines,
          ...withFlowAnnotation([
            `${indent(depth)}const ${buildFlowFunctionName(node.id)} = () => {`,
            `${indent(depth + 1)}console.log(${toConsoleLogArgument(message)});`,
            ...renderChildren(node.id, visited, depth + 1),
            `${indent(depth)}};`,
          ]),
          "",
        ];
      }
      case "notify": {
        const message = normalizeMessage(
          nodeProperties.message,
          "策略条件命中，准备处理后续动作",
        );
        return [
          ...dependencyLines,
          ...withFlowAnnotation([
            `${indent(depth)}const ${buildFlowFunctionName(node.id)} = () => {`,
            `${indent(depth + 1)}notify(${toScriptMessage(message)});`,
            ...renderChildren(node.id, visited, depth + 1),
            `${indent(depth)}};`,
          ]),
          "",
        ];
      }
      case "placeOrder": {
        return [
          ...dependencyLines,
          ...withFlowAnnotation([
            `${indent(depth)}const ${buildFlowFunctionName(node.id)} = () => {`,
            ...renderPlaceOrderNode(nodeProperties, depth + 1),
            ...renderChildren(node.id, visited, depth + 1),
            `${indent(depth)}};`,
          ]),
          "",
        ];
      }
      case "stopLoss": {
        return [
          ...dependencyLines,
          ...withFlowAnnotation([
            `${indent(depth)}const ${buildFlowFunctionName(node.id)} = () => {`,
            ...renderStopLossNode(node.id, nodeProperties, depth + 1),
            ...renderChildren(node.id, visited, depth + 1),
            `${indent(depth)}};`,
          ]),
          "",
        ];
      }
      case "codeBlock": {
        const code = normalizeCodeBlock(
          nodeProperties.code,
          'console.log("补充自定义逻辑");',
        );
        const codeLines = indentCodeBlock(code, depth + 1);
        if (codeLines.length === 0) {
          return [
            ...dependencyLines,
            ...withFlowAnnotation([
              `${indent(depth)}const ${buildFlowFunctionName(node.id)} = () => {`,
              ...renderChildren(node.id, visited, depth + 1),
              `${indent(depth)}};`,
            ]),
            "",
          ];
        }
        return [
          ...dependencyLines,
          ...withFlowAnnotation([
            `${indent(depth)}const ${buildFlowFunctionName(node.id)} = () => {`,
            `${indent(depth + 1)}// @jftradeCodeBlockBegin`,
            ...codeLines,
            `${indent(depth + 1)}// @jftradeCodeBlockEnd`,
            ...renderChildren(node.id, visited, depth + 1),
            `${indent(depth)}};`,
          ]),
          "",
        ];
      }
      case "getTechnicalIndicator": {
        const getterProperties = normalizeGetTechnicalIndicatorProperties(node.properties ?? {});
        return [
          ...dependencyLines,
          ...withFlowAnnotation([
            `${indent(depth)}const ${buildFlowFunctionName(node.id)} = () => {`,
            ...renderGetTechnicalIndicatorNode(node, visited, depth + 1, renderChildren),
            `${indent(depth)}};`,
          ],
          getterProperties.variableName === undefined
            ? {}
            : { variableName: getterProperties.variableName }),
          "",
        ];
      }
      case "technicalIndicatorCondition": {
        const inputs = incomingIndicatorInputs(node.id);
        const flowInputTags: Parameters<typeof buildStrategyFlowNodeJsDoc>[2] = {};
        const primaryInputNodeId = readIndicatorInputBinding(inputs, "primary")?.node.id;
        const fastInputNodeId = readIndicatorInputBinding(inputs, "fast")?.node.id;
        const slowInputNodeId = readIndicatorInputBinding(inputs, "slow")?.node.id;
        if (primaryInputNodeId !== undefined) {
          flowInputTags.inputPrimaryNodeId = primaryInputNodeId;
        }
        if (fastInputNodeId !== undefined) {
          flowInputTags.inputFastNodeId = fastInputNodeId;
        }
        if (slowInputNodeId !== undefined) {
          flowInputTags.inputSlowNodeId = slowInputNodeId;
        }
        return [
          ...dependencyLines,
          ...withFlowAnnotation([
            `${indent(depth)}const ${buildFlowFunctionName(node.id)} = () => {`,
            ...renderTechnicalIndicatorConditionNode(
              node,
              visited,
              depth + 1,
              renderBranchChildren,
              () => inputs,
              (inputNodeId) => !controlReachableNodeIds.has(inputNodeId),
            ),
            `${indent(depth)}};`,
          ],
          flowInputTags),
          "",
        ];
      }
      case "technicalIndicator": {
        return [
          ...dependencyLines,
          ...withFlowAnnotation([
            `${indent(depth)}const ${buildFlowFunctionName(node.id)} = () => {`,
            ...renderTechnicalIndicatorNode(node, visited, depth + 1, renderChildren),
            `${indent(depth)}};`,
          ]),
          "",
        ];
      }
      case "ifCloseAbove":
      case "ifCloseBelow": {
        const threshold = normalizeThreshold(nodeProperties.threshold, 500);
        const operator = kind === "ifCloseAbove" ? ">" : "<";
        const body = renderChildren(node.id, visited, depth + 2);
        return [
          ...dependencyLines,
          ...withFlowAnnotation([
            `${indent(depth)}const ${buildFlowFunctionName(node.id)} = () => {`,
            `${indent(depth + 1)}if (ctx.kline.close ${operator} ${threshold}) {`,
            ...(body.length > 0
              ? body
              : [`${indent(depth + 2)}// Add action blocks after this condition.`]),
            `${indent(depth + 1)}}`,
            `${indent(depth)}};`,
          ]),
          "",
        ];
      }
      default:
        return dependencyLines;
    }
  };

  const renderHook = (kind: StrategyBlockKind, hookName: string): string[] => {
    renderedFunctionIds.clear();
    renderedFunctionNodes.length = 0;

    const rootNodes = sortNodeIds(
      normalizedModel.nodes
        .filter((node) => getStrategyBlockKind(node) === kind)
        .map((node) => node.id),
    )
      .map((nodeId) => nodeById.get(nodeId))
      .filter(
        (node): node is StrategyVisualNodeDocument => node !== undefined,
      );

    const rootEntryNodes = rootNodes.flatMap((node) => outgoingTargets(node.id));
    const functionDefinitionLines = rootEntryNodes.flatMap((node) =>
      renderNodeDefinition(node, new Set([node.id]), 1),
    );
    const sharedIndicatorStateLines = renderedFunctionNodes
      .filter((node) => getStrategyBlockKind(node) === "getTechnicalIndicator")
      .flatMap((node) =>
        buildGetTechnicalIndicatorStateDeclarationLines(
          node.id,
          normalizeGetTechnicalIndicatorProperties(node.properties ?? {}),
          1,
        ),
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
      ...(hookPrelude.length > 0 && (sharedIndicatorStateLines.length > 0 || functionDefinitionLines.length > 0 || bodyLines.length > 0) ? [""] : []),
      ...sharedIndicatorStateLines,
      ...(sharedIndicatorStateLines.length > 0 && (functionDefinitionLines.length > 0 || bodyLines.length > 0) ? [""] : []),
      ...functionDefinitionLines,
      ...(functionDefinitionLines.length > 0 && bodyLines.length > 0 ? [""] : []),
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
    .flatMap((node) => {
      const code = normalizeCodeBlock(
        node.properties.code,
        'console.log("补充自定义逻辑");',
      );
      const codeLines = indentCodeBlock(code, 0);
      if (codeLines.length === 0) {
        return [];
      }
      return [
        ...buildStrategyFlowNodeJsDoc(node, 0),
        `// @jftradeCodeBlockBegin`,
        ...codeLines,
        `// @jftradeCodeBlockEnd`,
      ];
    });

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

function renderGetTechnicalIndicatorNode(
  node: StrategyVisualNodeDocument,
  visited: Set<string>,
  depth: number,
  renderChildren: (nodeId: string, visited: Set<string>, depth: number) => string[],
): string[] {
  const properties = normalizeGetTechnicalIndicatorProperties(node.properties ?? {});
  return [
    ...buildGetTechnicalIndicatorSetupLines(node.id, properties, depth, true),
    ...renderChildren(node.id, visited, depth),
    `${indent(depth)}return true;`,
  ];
}

function buildGetTechnicalIndicatorSetupLines(
  nodeId: string,
  properties: GetTechnicalIndicatorBlockProperties,
  depth: number,
  assignToSharedState = false,
): string[] {
  const base = buildIndicatorGetterBaseIdentifier(nodeId);
  const assign = (identifier: string, expression: string) =>
    `${indent(depth)}${assignToSharedState ? `${identifier} = ${expression};` : `const ${identifier} = ${expression};`}`;

  switch (properties.indicatorType) {
    case "movingAverage": {
      const key = buildMovingAverageIndicatorKey(
        properties.windowSize ?? 20,
        properties.movingAverageType ?? "MA",
        properties.periodUnit ?? "bar",
      );
      const snapshotVar = `${base}_snapshot`;
      const valueVar = `${base}_value`;
      const previousVar = `${base}_previous`;
      return [
        assign(snapshotVar, `ctx.indicators[${JSON.stringify(key)}] ?? null`),
        `${indent(depth)}if (${snapshotVar} === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${key}");`,
        `${indent(depth + 1)}return false;`,
        `${indent(depth)}}`,
        assign(valueVar, `${snapshotVar}.value ?? null`),
        assign(previousVar, `${snapshotVar}.previous ?? null`),
      ];
    }
    case "rsi": {
      const key = buildRsiIndicatorKey(properties.period ?? 14);
      return buildScalarIndicatorSetupLines(base, key, depth, assignToSharedState);
    }
    case "macd": {
      const key = buildMacdIndicatorKey(
        properties.fastPeriod ?? 12,
        properties.slowPeriod ?? 26,
        properties.signalPeriod ?? 9,
      );
      return [
        assign(base, `ctx.indicators[${JSON.stringify(key)}] ?? null`),
        `${indent(depth)}if (${base} === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${key}");`,
        `${indent(depth + 1)}return false;`,
        `${indent(depth)}}`,
        assign(`${base}_diff`, `${base}.diff`),
        assign(`${base}_signal`, `${base}.signal`),
        assign(`${base}_histogram`, `${base}.histogram`),
        assign(`${base}_previous_diff`, `${base}.previousDiff ?? null`),
        assign(`${base}_previous_signal`, `${base}.previousSignal ?? null`),
      ];
    }
    case "kdj": {
      const key = buildKdjIndicatorKey(
        properties.period ?? 9,
        properties.m1 ?? 3,
        properties.m2 ?? 3,
      );
      return [
        assign(base, `ctx.indicators[${JSON.stringify(key)}] ?? null`),
        `${indent(depth)}if (${base} === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${key}");`,
        `${indent(depth + 1)}return false;`,
        `${indent(depth)}}`,
        assign(`${base}_k`, `${base}.k`),
        assign(`${base}_d`, `${base}.d`),
        assign(`${base}_j`, `${base}.j`),
        assign(`${base}_previous_k`, `${base}.previousK ?? null`),
        assign(`${base}_previous_d`, `${base}.previousD ?? null`),
      ];
    }
    case "atr": {
      const key = buildAtrIndicatorKey(properties.period ?? 14);
      return buildScalarIndicatorSetupLines(base, key, depth, assignToSharedState);
    }
    case "cci": {
      const key = buildCciIndicatorKey(properties.period ?? 20);
      return buildScalarIndicatorSetupLines(base, key, depth, assignToSharedState);
    }
    case "williamsR": {
      const key = buildWilliamsRIndicatorKey(properties.period ?? 14);
      return buildScalarIndicatorSetupLines(base, key, depth, assignToSharedState);
    }
    case "bollinger": {
      const key = buildBollingerIndicatorKey(
        properties.period ?? 20,
        properties.multiplier ?? 2,
      );
      return [
        assign(base, `ctx.indicators[${JSON.stringify(key)}] ?? null`),
        `${indent(depth)}if (${base} === null) {`,
        `${indent(depth + 1)}console.log("waiting for indicator ${key}");`,
        `${indent(depth + 1)}return false;`,
        `${indent(depth)}}`,
        assign(`${base}_middle`, `${base}.middle`),
        assign(`${base}_upper`, `${base}.upper`),
        assign(`${base}_lower`, `${base}.lower`),
      ];
    }
    default:
      return [];
  }
}

function buildGetTechnicalIndicatorStateDeclarationLines(
  nodeId: string,
  properties: GetTechnicalIndicatorBlockProperties,
  depth: number,
): string[] {
  const base = buildIndicatorGetterBaseIdentifier(nodeId);
  switch (properties.indicatorType) {
    case "movingAverage":
      return [
        `${indent(depth)}let ${base}_snapshot = null;`,
        `${indent(depth)}let ${base}_value = null;`,
        `${indent(depth)}let ${base}_previous = null;`,
      ];
    case "rsi":
    case "atr":
    case "cci":
    case "williamsR":
      return [
        `${indent(depth)}let ${base}_snapshot = null;`,
        `${indent(depth)}let ${base}_value = null;`,
      ];
    case "macd":
      return [
        `${indent(depth)}let ${base} = null;`,
        `${indent(depth)}let ${base}_diff = null;`,
        `${indent(depth)}let ${base}_signal = null;`,
        `${indent(depth)}let ${base}_histogram = null;`,
        `${indent(depth)}let ${base}_previous_diff = null;`,
        `${indent(depth)}let ${base}_previous_signal = null;`,
      ];
    case "kdj":
      return [
        `${indent(depth)}let ${base} = null;`,
        `${indent(depth)}let ${base}_k = null;`,
        `${indent(depth)}let ${base}_d = null;`,
        `${indent(depth)}let ${base}_j = null;`,
        `${indent(depth)}let ${base}_previous_k = null;`,
        `${indent(depth)}let ${base}_previous_d = null;`,
      ];
    case "bollinger":
      return [
        `${indent(depth)}let ${base} = null;`,
        `${indent(depth)}let ${base}_middle = null;`,
        `${indent(depth)}let ${base}_upper = null;`,
        `${indent(depth)}let ${base}_lower = null;`,
      ];
    default:
      return [];
  }
}

function buildScalarIndicatorSetupLines(
  variableName: string,
  indicatorKey: string,
  depth: number,
  assignToSharedState = false,
): string[] {
  const snapshotVar = `${variableName}_snapshot`;
  const valueVar = `${variableName}_value`;
  return [
    `${indent(depth)}${assignToSharedState ? `${snapshotVar} = ctx.indicators[${JSON.stringify(indicatorKey)}] ?? null;` : `const ${snapshotVar} = ctx.indicators[${JSON.stringify(indicatorKey)}] ?? null;`}`,
    `${indent(depth)}if (${snapshotVar} === null) {`,
    `${indent(depth + 1)}console.log("waiting for indicator ${indicatorKey}");`,
    `${indent(depth + 1)}return false;`,
    `${indent(depth)}}`,
    `${indent(depth)}${assignToSharedState ? `${valueVar} = ${snapshotVar};` : `const ${valueVar} = ${snapshotVar};`}`,
  ];
}

function renderTechnicalIndicatorConditionNode(
  node: StrategyVisualNodeDocument,
  visited: Set<string>,
  depth: number,
  renderBranchChildren: (
    nodeId: string,
    branch: StrategyVisualEdgeBranch,
    visited: Set<string>,
    depth: number,
  ) => string[],
  incomingIndicatorInputs: (nodeId: string) => IndicatorInputBinding[],
  shouldInlineIndicatorSetup: (nodeId: string) => boolean,
): string[] {
  const properties = normalizeTechnicalIndicatorConditionProperties(node.properties ?? {});
  const inputs = incomingIndicatorInputs(node.id);
  const inlineGetterCalls = inputs
    .filter((input, index, allInputs) =>
      allInputs.findIndex((candidate) => candidate.node.id === input.node.id) === index,
    )
    .filter((input) => shouldInlineIndicatorSetup(input.node.id))
    .flatMap((input) => [
      `${indent(depth)}if (!${buildFlowFunctionName(input.node.id)}()) {`,
      `${indent(depth + 1)}return;`,
      `${indent(depth)}}`,
    ]);
  const conditionExpression = buildTechnicalIndicatorConditionExpression(properties, inputs);
  const trueBody = renderBranchChildren(node.id, "true", visited, depth + 1);
  const falseBody = renderBranchChildren(node.id, "false", visited, depth + 1);

  if (conditionExpression === null) {
    return [
      ...inlineGetterCalls,
      `${indent(depth)}// Missing indicator inputs for ${node.text || node.id}.`,
    ];
  }

  return [
    ...inlineGetterCalls,
    `${indent(depth)}if (${conditionExpression}) {`,
    ...(trueBody.length > 0
      ? trueBody
      : [`${indent(depth + 1)}// Add action blocks for the true branch.`]),
    `${indent(depth)}} else {`,
    ...(falseBody.length > 0
      ? falseBody
      : [`${indent(depth + 1)}// Add action blocks for the false branch.`]),
    `${indent(depth)}}`,
  ];
}

function buildTechnicalIndicatorConditionExpression(
  properties: TechnicalIndicatorConditionBlockProperties,
  inputs: IndicatorInputBinding[],
): string | null {
  const primary = readIndicatorInputBinding(inputs, "primary") ?? inputs[0];
  if (properties.conditionMode === "numeric") {
    if (primary === undefined) {
      return null;
    }
    const targetValue = numericInputTargetExpression(primary.node.id, primary.properties.indicatorType);
    if (targetValue === null) {
      return null;
    }
    return `Number.isFinite(${targetValue}) && ${targetValue} ${properties.operator ?? "<"} ${properties.threshold ?? 0}`;
  }

  switch (properties.indicatorType) {
    case "movingAverage": {
      const fast = readIndicatorInputBinding(inputs, "fast") ?? inputs[0];
      const slow = readIndicatorInputBinding(inputs, "slow") ?? inputs[1];
      if (fast === undefined || slow === undefined) {
        return null;
      }
      const fastBase = buildIndicatorGetterBaseIdentifier(fast.node.id);
      const slowBase = buildIndicatorGetterBaseIdentifier(slow.node.id);
      const previousOperator = properties.patternType === "deathCross" ? ">=" : "<=";
      const currentOperator = properties.patternType === "deathCross" ? "<" : ">";
      return `${fastBase}_previous !== null && ${slowBase}_previous !== null && ${fastBase}_value !== null && ${slowBase}_value !== null && ${fastBase}_previous ${previousOperator} ${slowBase}_previous && ${fastBase}_value ${currentOperator} ${slowBase}_value`;
    }
    case "rsi": {
      if (primary === undefined || !isDivergencePattern(properties.patternType)) {
        return null;
      }
      return `ctx.indicators[${JSON.stringify(buildDivergenceIndicatorKey("rsi", [primary.properties.period ?? 14], properties.patternType === "topDivergence" ? "top" : "bottom", properties.lookback ?? 5))}] ?? false`;
    }
    case "macd": {
      if (primary === undefined) {
        return null;
      }
      if (isDivergencePattern(properties.patternType)) {
        return `ctx.indicators[${JSON.stringify(buildDivergenceIndicatorKey("macd", [primary.properties.fastPeriod ?? 12, primary.properties.slowPeriod ?? 26, primary.properties.signalPeriod ?? 9], properties.patternType === "topDivergence" ? "top" : "bottom", properties.lookback ?? 5))}] ?? false`;
      }
      const base = buildIndicatorGetterBaseIdentifier(primary.node.id);
      const previousOperator = properties.patternType === "deathCross" ? ">=" : "<=";
      const currentOperator = properties.patternType === "deathCross" ? "<" : ">";
      return `${base}_previous_diff !== null && ${base}_previous_signal !== null && ${base}_previous_diff ${previousOperator} ${base}_previous_signal && ${base}_diff ${currentOperator} ${base}_signal`;
    }
    case "kdj": {
      if (primary === undefined) {
        return null;
      }
      if (isDivergencePattern(properties.patternType)) {
        return `ctx.indicators[${JSON.stringify(buildDivergenceIndicatorKey("kdj", [primary.properties.period ?? 9, primary.properties.m1 ?? 3, primary.properties.m2 ?? 3], properties.patternType === "topDivergence" ? "top" : "bottom", properties.lookback ?? 5))}] ?? false`;
      }
      const base = buildIndicatorGetterBaseIdentifier(primary.node.id);
      const previousOperator = properties.patternType === "deathCross" ? ">=" : "<=";
      const currentOperator = properties.patternType === "deathCross" ? "<" : ">";
      return `${base}_previous_k !== null && ${base}_previous_d !== null && ${base}_previous_k ${previousOperator} ${base}_previous_d && ${base}_k ${currentOperator} ${base}_d`;
    }
    case "bollinger": {
      if (primary === undefined) {
        return null;
      }
      const base = buildIndicatorGetterBaseIdentifier(primary.node.id);
      if (properties.patternType === "closeAboveUpperBand") {
        return `ctx.kline.close > ${base}_upper`;
      }
      return `ctx.kline.close < ${base}_lower`;
    }
    default:
      return null;
  }
}

function readIndicatorInputBinding(
  inputs: IndicatorInputBinding[],
  slot: string,
) {
  return inputs.find((input) => input.slot === slot);
}

function numericInputTargetExpression(
  nodeId: string,
  indicatorType: GetTechnicalIndicatorBlockProperties["indicatorType"],
): string | null {
  const base = buildIndicatorGetterBaseIdentifier(nodeId);
  switch (indicatorType) {
    case "rsi":
    case "atr":
    case "cci":
    case "williamsR":
      return `${base}_value`;
    case "macd":
      return `${base}_histogram`;
    case "kdj":
      return `${base}_j`;
    default:
      return null;
  }
}

function buildIndicatorGetterBaseIdentifier(nodeId: string): string {
  return `indicator_${sanitizeScriptIdentifier(nodeId)}`;
}

function buildFlowFunctionName(nodeId: string): string {
  return `flow_${sanitizeScriptIdentifier(nodeId)}`;
}

function sanitizeScriptIdentifier(value: string): string {
  const normalized = value.replace(/[^a-zA-Z0-9_]+/g, "_").replace(/^([0-9])/, "_$1");
  return normalized === "" ? "node" : normalized;
}

function collectControlReachableNodeIds(
  rootNodeIds: string[],
  outgoingById: Map<string, StrategyVisualEdgeDocument[]>,
  nodeById: Map<string, StrategyVisualNodeDocument>,
): Set<string> {
  const visited = new Set<string>();
  const pending = [...rootNodeIds];

  while (pending.length > 0) {
    const nodeId = pending.shift();
    if (nodeId === undefined || visited.has(nodeId) || !nodeById.has(nodeId)) {
      continue;
    }

    visited.add(nodeId);

    for (const edge of outgoingById.get(nodeId) ?? []) {
      if (!isStrategyVisualControlEdge(edge)) {
        continue;
      }
      if (!visited.has(edge.targetNodeId)) {
        pending.push(edge.targetNodeId);
      }
    }
  }

  return visited;
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
      const fastKey = buildMovingAverageIndicatorKey(
        properties.fastPeriod ?? 5,
        properties.movingAverageType ?? "MA",
      );
      const slowKey = buildMovingAverageIndicatorKey(
        properties.slowPeriod ?? 20,
        properties.movingAverageType ?? "MA",
      );
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
  const entryPositionPolicy = normalizeEntryPositionPolicy(nodeProperties.entryPositionPolicy);
  const exchangeSide = orderSideForExchange(visualSide);
  const sideLabel = orderSideLabel(visualSide);
  const orderType = normalizeOrderType(nodeProperties.orderType);
  const quantityMode = normalizeQuantityModeForSide(nodeProperties.quantityMode, visualSide);
  const quantityValue = normalizeDecimal(nodeProperties.quantityValue, 100);
  const limitPrice = normalizeDecimal(nodeProperties.limitPrice, 0);
  const orderProps = [`side: "${exchangeSide}"`, `orderType: "${orderType}"`];
  const orderPriceExpression = orderType === "LIMIT" && limitPrice > 0 ? String(limitPrice) : "ctx.kline.close";
  if (orderType === "LIMIT" && limitPrice > 0) {
    orderProps.push(`limitPrice: ${limitPrice}`);
  }

  const lines: string[] = [
    `${indent(depth)}const pos = getPosition();`,
    `${indent(depth)}const availablePositionQty = pos ? Math.floor(Math.abs(pos.availableQuantity) > 0 ? Math.abs(pos.availableQuantity) : Math.abs(pos.quantity)) : 0;`,
  ];
  switch (visualSide) {
    case "BUY":
      lines.push(...buildEntryPositionGuardLines(visualSide, entryPositionPolicy, depth));
      break;
    case "SELL":
      lines.push(
        `${indent(depth)}if (!pos || pos.direction !== "LONG" || availablePositionQty <= 0) {`,
        `${indent(depth + 1)}console.log("无多头持仓可平，跳过卖出");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    case "SELL_SHORT":
      lines.push(...buildEntryPositionGuardLines(visualSide, entryPositionPolicy, depth));
      break;
    case "BUY_COVER":
      lines.push(
        `${indent(depth)}if (!pos || pos.direction !== "SHORT" || availablePositionQty <= 0) {`,
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
        `${indent(depth)}const orderPrice = ${orderPriceExpression};`,
        `${indent(depth)}const maxQty = Math.floor(${quantityValue} / orderPrice);`,
        `${indent(depth)}if (maxQty <= 0) {`,
        `${indent(depth + 1)}console.log("金额 ${quantityValue} 不足以购买 1 股（当前价格 " + orderPrice + "），跳过下单");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
        `${indent(depth)}const orderQty = maxQty;`,
      );
      break;
    case "accountPositionPercent":
      lines.push(
        `${indent(depth)}const orderPrice = ${orderPriceExpression};`,
        `${indent(depth)}const accountTotalValue = getTotalAccountValue();`,
        `${indent(depth)}const targetAmount = accountTotalValue * ${quantityValue} / 100;`,
        `${indent(depth)}const rawOrderQty = targetAmount > 0 ? Math.floor(targetAmount / orderPrice) : 0;`,
        `${indent(depth)}const orderQty = rawOrderQty > 0 ? Math.min(rawOrderQty, availablePositionQty || rawOrderQty) : (${visualSide === "SELL" || visualSide === "BUY_COVER"} && availablePositionQty > 0 ? 1 : 0);`,
        `${indent(depth)}if (orderQty <= 0) {`,
        `${indent(depth + 1)}console.log("账户仓位百分比计算所得数量为 0（账户总资产 " + accountTotalValue + " × ${quantityValue}% ÷ 价格 " + orderPrice + "），请调整百分比或确认账户资产快照可用");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    case "symbolPositionPercent":
      lines.push(
        `${indent(depth)}const orderPrice = ${orderPriceExpression};`,
        `${indent(depth)}const currentPositionValue = pos ? Math.abs(pos.marketValue) : 0;`,
        `${indent(depth)}const targetValue = currentPositionValue * ${quantityValue} / 100;`,
        `${indent(depth)}const rawOrderQty = targetValue > 0 ? Math.floor(targetValue / orderPrice) : 0;`,
        `${indent(depth)}const orderQty = rawOrderQty > 0 ? Math.min(rawOrderQty, availablePositionQty || rawOrderQty) : (${visualSide === "SELL" || visualSide === "BUY_COVER"} && availablePositionQty > 0 ? 1 : 0);`,
        `${indent(depth)}if (orderQty <= 0) {`,
        `${indent(depth + 1)}console.log("当前标的仓位百分比计算所得数量为 0，跳过下单");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    case "cashPercent":
      lines.push(
        `${indent(depth)}const orderPrice = ${orderPriceExpression};`,
        `${indent(depth)}const availableCash = getAvailableCash();`,
        `${indent(depth)}const targetAmount = availableCash * ${quantityValue} / 100;`,
        `${indent(depth)}const orderQty = targetAmount > 0 ? Math.floor(targetAmount / orderPrice) : 0;`,
        `${indent(depth)}if (orderQty <= 0) {`,
        `${indent(depth + 1)}console.log("现金百分比计算所得数量为 0（可用资金 " + availableCash + " × ${quantityValue}% ÷ 价格 " + orderPrice + "），请调整百分比或确认账户资金充足");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    case "marginBuyingPowerPercent":
      lines.push(
        `${indent(depth)}const orderPrice = ${orderPriceExpression};`,
        `${indent(depth)}const marginBuyingPower = getMarginBuyingPower();`,
        `${indent(depth)}const targetAmount = marginBuyingPower * ${quantityValue} / 100;`,
        `${indent(depth)}const orderQty = targetAmount > 0 ? Math.floor(targetAmount / orderPrice) : 0;`,
        `${indent(depth)}if (orderQty <= 0) {`,
        `${indent(depth + 1)}console.log("融资可用百分比计算所得数量为 0（融资可用 " + marginBuyingPower + " × ${quantityValue}% ÷ 价格 " + orderPrice + "），请调整百分比或确认保证金账户购买力可用");`,
        `${indent(depth + 1)}return;`,
        `${indent(depth)}}`,
      );
      break;
    case "shortSellingPowerPercent":
      lines.push(
        `${indent(depth)}const orderPrice = ${orderPriceExpression};`,
        `${indent(depth)}const shortSellingPower = getShortSellingPower();`,
        `${indent(depth)}const targetAmount = shortSellingPower * ${quantityValue} / 100;`,
        `${indent(depth)}const orderQty = targetAmount > 0 ? Math.floor(targetAmount / orderPrice) : 0;`,
        `${indent(depth)}if (orderQty <= 0) {`,
        `${indent(depth + 1)}console.log("融券可用百分比计算所得数量为 0（融券可用 " + shortSellingPower + " × ${quantityValue}% ÷ 价格 " + orderPrice + "），请调整百分比或确认保证金账户融券能力可用");`,
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

function renderStopLossNode(
  nodeId: string,
  nodeProperties: Record<string, unknown>,
  depth: number,
): string[] {
  const properties = normalizeStopLossBlockProperties(nodeProperties);
  const base = `risk_${sanitizeScriptIdentifier(nodeId)}`;
  const snapshotName = `${base}_snapshot`;
  const positionName = `${base}_position`;
  const quantityName = `${base}_qty`;
  const shouldExitLongName = `${base}_exit_long`;
  const shouldExitShortName = `${base}_exit_short`;
  const indicatorKey = buildStopLossIndicatorKey(
    properties.direction ?? "auto",
    properties.timeValue ?? 1,
    properties.timeUnit ?? "day",
    properties.percentage ?? 2,
    properties.mode ?? "stopLoss",
    properties.windowPolicy ?? "continuous",
  );
  const label = nextStopLossNodeText(properties as unknown as Record<string, unknown>);
  const triggerMetric = properties.mode === "trailingStop"
    ? `Math.abs(${snapshotName}.triggerPercent ?? ${snapshotName}.longDrawdownPercent ?? ${snapshotName}.shortReboundPercent ?? 0)`
    : `Math.abs(${snapshotName}.triggerPercent ?? ${snapshotName}.changePercent ?? 0)`;

  return [
    `${indent(depth)}const ${snapshotName} = ctx.indicators[${JSON.stringify(indicatorKey)}] ?? null;`,
    `${indent(depth)}if (${snapshotName} === null) {`,
    `${indent(depth + 1)}console.log("waiting for indicator ${indicatorKey}");`,
    `${indent(depth + 1)}return;`,
    `${indent(depth)}}`,
    `${indent(depth)}const ${positionName} = getPosition();`,
    `${indent(depth)}const ${quantityName} = ${positionName} ? Math.floor(Math.abs(${positionName}.availableQuantity) > 0 ? Math.abs(${positionName}.availableQuantity) : Math.abs(${positionName}.quantity)) : 0;`,
    `${indent(depth)}const ${shouldExitLongName} = ${properties.direction !== "short"} && !!${positionName} && ${positionName}.direction === "LONG" && ${quantityName} > 0 && ${snapshotName}.longTriggered === true;`,
    `${indent(depth)}const ${shouldExitShortName} = ${properties.direction !== "long"} && !!${positionName} && ${positionName}.direction === "SHORT" && ${quantityName} > 0 && ${snapshotName}.shortTriggered === true;`,
    `${indent(depth)}if (${shouldExitLongName}) {`,
    `${indent(depth + 1)}console.log("${label}触发，幅度 " + ${triggerMetric} + "% ，执行卖出平多");`,
    `${indent(depth + 1)}placeOrder({ side: "SELL", orderType: "MARKET", quantity: ${quantityName} });`,
    `${indent(depth + 1)}return;`,
    `${indent(depth)}}`,
    `${indent(depth)}if (${shouldExitShortName}) {`,
    `${indent(depth + 1)}console.log("${label}触发，幅度 " + ${triggerMetric} + "% ，执行买入平空");`,
    `${indent(depth + 1)}placeOrder({ side: "BUY", orderType: "MARKET", quantity: ${quantityName} });`,
    `${indent(depth + 1)}return;`,
    `${indent(depth)}}`,
  ];
}

function buildEntryPositionGuardLines(
  visualSide: "BUY" | "SELL_SHORT",
  entryPositionPolicy: ReturnType<typeof normalizeEntryPositionPolicy>,
  depth: number,
): string[] {
  const isLongEntry = visualSide === "BUY";
  const sameDirection = isLongEntry ? "LONG" : "SHORT";
  const holdingLabel = isLongEntry ? "多头" : "空头";
  const actionLabel = isLongEntry ? "开多" : "开空";

  if (entryPositionPolicy === "allow") {
    return [];
  }

  if (entryPositionPolicy === "flatOnly") {
    return [
      `${indent(depth)}if (pos && pos.quantity !== 0) {`,
      `${indent(depth + 1)}console.log("当前已有持仓（方向 " + pos.direction + "，数量 " + pos.quantity + "），按${entryPositionPolicyLabel("flatOnly")}策略跳过${actionLabel}");`,
      `${indent(depth + 1)}return;`,
      `${indent(depth)}}`,
    ];
  }

  return [
    `${indent(depth)}if (pos && pos.direction === "${sameDirection}" && availablePositionQty > 0) {`,
    `${indent(depth + 1)}console.log("已有${holdingLabel}持仓 " + pos.quantity + " 股，按${entryPositionPolicyLabel("sameDirection")}策略跳过${actionLabel}");`,
    `${indent(depth + 1)}return;`,
    `${indent(depth)}}`,
  ];
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
