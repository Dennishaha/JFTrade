import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";
import { parse } from "acorn";

import type { StrategyBlockKind } from "./strategyVisualBuilderCatalog";
import {
  parseStrategyFlowNodeJsDocComment,
  type StrategyFlowNodeJsDoc,
} from "./strategyVisualBuilderShared";

type HookKind = "onInit" | "onKLineClosed";

interface AstNode {
  type: string;
  start: number;
  end: number;
  [key: string]: unknown;
}

interface AstProgram extends AstNode {
  body: AstStatement[];
}

interface AstBlockStatement extends AstNode {
  body: AstStatement[];
}

type AstStatement = AstNode;

interface ParsedVisualNode {
  kind: Exclude<StrategyBlockKind, HookKind>;
  text: string;
  properties: Record<string, unknown>;
  children?: AstStatement[];
  flowNodeId?: string;
}

interface ParsedStatementMatch {
  nextIndex: number;
  item: ParsedVisualNode | null;
}

type ParsedConditionDescriptor = Pick<ParsedVisualNode, "kind" | "text" | "properties">;

interface ParseSequenceContext {
  hookKind: HookKind;
  parentId: string;
  baseX: number;
  baseY: number;
}

interface FlowAnnotationComment {
  start: number;
  end: number;
  annotation: StrategyFlowNodeJsDoc;
}

interface StrategyParserContext {
  script: string;
  flowAnnotations: FlowAnnotationComment[];
}

interface StrategySourceRange {
  start: number;
  end: number;
}

type ParseSequenceMode = "linear" | "siblings";

export interface StrategyScriptParseSuccess {
  ok: true;
  model: StrategyVisualModelDocument;
  codeBlockCount: number;
}

export interface StrategyScriptParseFailure {
  ok: false;
  error: string;
}

export type StrategyScriptParseResult =
  | StrategyScriptParseSuccess
  | StrategyScriptParseFailure;

const ROOT_LAYOUT = {
  global: {
    x: 180,
    y: 108,
  },
  onInit: {
    x: 180,
    y: 300,
  },
  onKLineClosed: {
    x: 180,
    y: 560,
  },
} as const;

const BLOCK_X_STEP = 260;
const BLOCK_Y_STEP = 160;

const GENERATED_RUNTIME_FUNCTIONS = new Set([
  "simpleMovingAverage",
  "calculateRSI",
  "calculateEMASequence",
  "calculateMACD",
  "calculateStandardDeviation",
  "calculateBollingerBands",
]);

export function buildStrategyVisualModelFromScript(
  script: string,
  existingModel?: StrategyVisualModelDocument | null,
): StrategyScriptParseResult {
  try {
    const flowAnnotations: FlowAnnotationComment[] = [];
    const program = parse(script, {
      ecmaVersion: "latest",
      sourceType: "script",
      onComment(block: boolean, value: string, start: number, end: number) {
        if (!block) {
          return;
        }

        const annotation = parseStrategyFlowNodeJsDocComment(value);
        if (annotation === null) {
          return;
        }

        flowAnnotations.push({
          start,
          end,
          annotation,
        });
      },
    }) as unknown as AstProgram;

    const parserContext: StrategyParserContext = {
      script,
      flowAnnotations,
    };
    const builder = createModelBuilder(existingModel);
    const hookBodies = new Map<HookKind, AstStatement[]>();
    const globalStatements: AstStatement[] = [];

    for (const statement of program.body) {
      if (isGeneratedRuntimeStatement(statement)) {
        continue;
      }

      const hookDeclaration = readHookDeclaration(statement);
      if (hookDeclaration !== null && !hookBodies.has(hookDeclaration.kind)) {
        hookBodies.set(hookDeclaration.kind, hookDeclaration.body.body);
        continue;
      }

      globalStatements.push(statement);
    }

    appendGlobalCodeBlocks(globalStatements, parserContext, builder);

    appendHookSequence(
      hookBodies.get("onInit") ?? [],
      parserContext,
      builder,
      {
        hookKind: "onInit",
        parentId: "on-init-root",
        baseX: ROOT_LAYOUT.onInit.x + BLOCK_X_STEP,
        baseY: ROOT_LAYOUT.onInit.y,
      },
    );

    appendHookSequence(
      hookBodies.get("onKLineClosed") ?? [],
      parserContext,
      builder,
      {
        hookKind: "onKLineClosed",
        parentId: "on-kline-root",
        baseX: ROOT_LAYOUT.onKLineClosed.x + BLOCK_X_STEP,
        baseY: ROOT_LAYOUT.onKLineClosed.y,
      },
    );

    return {
      ok: true,
      model: {
        engine: "logic-flow",
        version: 1,
        nodes: builder.nodes,
        edges: builder.edges,
      },
      codeBlockCount: builder.codeBlockCount,
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : "未知解析错误";
    return {
      ok: false,
      error: `QuickJS 脚本解析失败：${message}`,
    };
  }
}

function buildExistingNodePositionMap(
  model: StrategyVisualModelDocument | null | undefined,
): Map<string, { x: number; y: number }> {
  const map = new Map<string, { x: number; y: number }>();
  if (model === null || model === undefined) {
    return map;
  }

  for (const node of model.nodes) {
    map.set(node.id, { x: node.x, y: node.y });
  }

  return map;
}

function createModelBuilder(
  existingModel?: StrategyVisualModelDocument | null,
): {
  nodes: StrategyVisualNodeDocument[];
  edges: StrategyVisualEdgeDocument[];
  nextId: number;
  codeBlockCount: number;
  usedNodeIds: Set<string>;
  existingPositions: Map<string, { x: number; y: number }>;
} {
  return {
    nodes: [
      {
        id: "on-init-root",
        type: "circle",
        x: ROOT_LAYOUT.onInit.x,
        y: ROOT_LAYOUT.onInit.y,
        text: "策略启动",
        properties: {
          blockKind: "onInit",
        },
      },
      {
        id: "on-kline-root",
        type: "circle",
        x: ROOT_LAYOUT.onKLineClosed.x,
        y: ROOT_LAYOUT.onKLineClosed.y,
        text: "K 线收盘",
        properties: {
          blockKind: "onKLineClosed",
        },
      },
    ],
    edges: [],
    nextId: 0,
    codeBlockCount: 0,
    usedNodeIds: new Set(["on-init-root", "on-kline-root"]),
    existingPositions: buildExistingNodePositionMap(existingModel),
  };
}

function resolvePreservedPosition(
  builder: ReturnType<typeof createModelBuilder>,
  nodeId: string,
  fallbackX: number,
  fallbackY: number,
): { x: number; y: number } {
  const preserved = builder.existingPositions.get(nodeId);
  if (preserved !== undefined) {
    return preserved;
  }

  return { x: fallbackX, y: fallbackY };
}

function appendGlobalCodeBlocks(
  statements: AstStatement[],
  parserContext: StrategyParserContext,
  builder: ReturnType<typeof createModelBuilder>,
): void {
  let index = 0;

  while (index < statements.length) {
    const statement = statements[index];
    if (statement === undefined) {
      break;
    }

    const annotation = readLeadingFlowAnnotation(statement, parserContext);
    const nextIndex = annotation === null
      ? index + 1
      : findNextAnnotatedStatementIndex(statements, index, parserContext);
    const sourceRange = readStatementSourceRange(statements, index, nextIndex);
    const code = normalizeCodeSnippet(
      parserContext.script.slice(statement.start, statements[nextIndex - 1]?.end ?? statement.end),
    );
    if (code === "") {
      index = nextIndex;
      continue;
    }

    const { nodeId } = reserveParsedNodeIdentity(builder, annotation?.nodeId, "global-code");
    const fallbackX = ROOT_LAYOUT.global.x + builder.codeBlockCount * BLOCK_X_STEP - BLOCK_X_STEP;
    const fallbackY = ROOT_LAYOUT.global.y;
    const { x, y } = resolvePreservedPosition(builder, nodeId, fallbackX, fallbackY);
    builder.codeBlockCount += 1;
    builder.nodes.push({
      id: nodeId,
      type: "rect",
      x,
      y,
      text: annotation?.nodeText ?? buildCodeBlockLabel(code, true),
      properties: withSourceRangeProperties(
        applyCodeBlockAnnotationProperties(
          {
            blockKind: "codeBlock",
            code,
            codeScope: "global",
          },
          annotation,
        ),
        sourceRange,
      ),
    });
    index = nextIndex;
  }
}

function appendHookSequence(
  statements: AstStatement[],
  parserContext: StrategyParserContext,
  builder: ReturnType<typeof createModelBuilder>,
  context: ParseSequenceContext,
  mode: ParseSequenceMode = "linear",
): void {
  let currentParentId = context.parentId;
  let siblingIndex = 0;

  for (let statementIndex = 0; statementIndex < statements.length;) {
    const match = parseHookStatement(
      statements,
      statementIndex,
      parserContext,
      context.hookKind,
    );
    statementIndex = match.nextIndex;

    if (match.item === null) {
      continue;
    }

    const parentPosition = readNodePosition(builder.nodes, currentParentId) ?? {
      x: context.baseX - BLOCK_X_STEP,
      y: context.baseY,
    };
    const keepsCurrentParent = mode === "siblings" || isConditionBlockKind(match.item.kind);
    const nodeX = mode === "siblings"
      ? Math.max(context.baseX, parentPosition.x + BLOCK_X_STEP)
      : keepsCurrentParent
        ? parentPosition.x
        : Math.max(context.baseX, parentPosition.x + BLOCK_X_STEP);
    const nodeY = mode === "siblings"
      ? context.baseY + siblingIndex * BLOCK_Y_STEP
      : keepsCurrentParent
        ? parentPosition.y + siblingIndex * BLOCK_Y_STEP
        : parentPosition.y;
    const { edgeId, nodeId } = reserveParsedNodeIdentity(
      builder,
      match.item.flowNodeId,
      "visual-node",
    );
    const { x: resolvedX, y: resolvedY } = resolvePreservedPosition(
      builder,
      nodeId,
      nodeX,
      nodeY,
    );
    builder.nodes.push({
      id: nodeId,
      type: resolveNodeShape(match.item.kind),
      x: resolvedX,
      y: resolvedY,
      text: match.item.text,
      properties: { ...match.item.properties },
    });
    builder.edges.push({
      id: edgeId,
      type: "polyline",
      sourceNodeId: currentParentId,
      targetNodeId: nodeId,
    });

    if (match.item.kind === "codeBlock") {
      builder.codeBlockCount += 1;
    }

    if (keepsCurrentParent) {
      siblingIndex += 1;
    } else {
      currentParentId = nodeId;
      siblingIndex = 0;
    }

    if ((match.item.children?.length ?? 0) > 0) {
      appendHookSequence(match.item.children ?? [], parserContext, builder, {
        hookKind: context.hookKind,
        parentId: nodeId,
        baseX: resolvedX + BLOCK_X_STEP,
        baseY: resolvedY + BLOCK_Y_STEP,
      });
    }
  }
}

function parseHookStatement(
  statements: AstStatement[],
  index: number,
  parserContext: StrategyParserContext,
  hookKind: HookKind,
): ParsedStatementMatch {
  const statement = statements[index];
  const flowAnnotation = readLeadingFlowAnnotation(statement, parserContext);

  if (statement === undefined) {
    return {
      nextIndex: index + 1,
      item: null,
    };
  }

  if (isCodeBlockMarkerStatement(statement, parserContext)) {
    if (flowAnnotation?.blockKind === "codeBlock") {
      return withParsedItemSourceRange(
        createAnnotatedCodeBlockMatch(
          statements,
          index,
          parserContext,
          flowAnnotation,
          false,
        ),
        statements,
        index,
      );
    }

    return {
      nextIndex: index + 1,
      item: null,
    };
  }

  if (statement.type === "EmptyStatement") {
    return {
      nextIndex: index + 1,
      item: null,
    };
  }

  if (flowAnnotation?.blockKind === "codeBlock") {
    return withParsedItemSourceRange(
      createAnnotatedCodeBlockMatch(
        statements,
        index,
        parserContext,
        flowAnnotation,
        false,
      ),
      statements,
      index,
    );
  }

  if (isGeneratedHookPreludeStatement(statement, hookKind)) {
    return {
      nextIndex: index + 1,
      item: null,
    };
  }

  const parsedMatch =
    tryParseMovingAverageFast(statements, index) ??
    tryParseMovingAverageSlow(statements, index) ??
    tryParseRsi(statements, index) ??
    tryParseMacd(statements, index) ??
    tryParseBollinger(statements, index) ??
    tryParsePlaceOrder(statements, index, parserContext.script) ??
    tryParseNotify(statement, parserContext.script, index) ??
    tryParseLog(statement, parserContext.script, index) ??
    tryParseCondition(statements, index, hookKind) ??
    null;

  if (parsedMatch !== null) {
    return withParsedItemSourceRange({
      nextIndex: parsedMatch.nextIndex,
      item:
        parsedMatch.item === null
          ? null
          : applyFlowAnnotation(parsedMatch.item, flowAnnotation),
    }, statements, index);
  }

  if (flowAnnotation !== null) {
    const fallbackItem = createAnnotationFallbackNode(flowAnnotation);
    if (fallbackItem !== null) {
      return withParsedItemSourceRange({
        nextIndex: index + 1,
        item: fallbackItem,
      }, statements, index);
    }

    return withParsedItemSourceRange(
      createAnnotatedCodeBlockMatch(
        statements,
        index,
        parserContext,
        flowAnnotation,
        false,
      ),
      statements,
      index,
    );
  }

  return withParsedItemSourceRange({
    nextIndex: index + 1,
    item: createCodeBlockNode(readSource(parserContext.script, statement), false),
  }, statements, index);
}

function tryParseMovingAverageFast(
  statements: AstStatement[],
  index: number,
): ParsedStatementMatch | null {
  const expression = matchAssignedCall(
    statements[index],
    "fastAverage",
    "simpleMovingAverage",
  );
  if (expression === null) {
    return null;
  }

  const args = readCallArguments(expression.right);
  if (!matchesStateCloses(args[0])) {
    return null;
  }

  const windowSize = readNumericLiteral(args[1]);
  if (windowSize === null) {
    return null;
  }

  return {
    nextIndex: index + 1,
    item: {
      kind: "movingAverageFast",
      text: `快均线 ${windowSize}`,
      properties: {
        blockKind: "movingAverageFast",
        windowSize,
      },
    },
  };
}

function tryParseMovingAverageSlow(
  statements: AstStatement[],
  index: number,
): ParsedStatementMatch | null {
  const expression = matchAssignedCall(
    statements[index],
    "slowAverage",
    "simpleMovingAverage",
  );
  if (expression === null) {
    return null;
  }

  const args = readCallArguments(expression.right);
  if (!matchesStateCloses(args[0])) {
    return null;
  }

  const windowSize = readNumericLiteral(args[1]);
  if (windowSize === null) {
    return null;
  }

  let nextIndex = index + 1;
  if (matchesNullGuard(statements[nextIndex], ["fastAverage", "slowAverage"])) {
    nextIndex += 1;
  }
  if (matchesStateAssignment(statements[nextIndex], "prevFastAverage", "fastAverage")) {
    nextIndex += 1;
  }
  if (matchesStateAssignment(statements[nextIndex], "prevSlowAverage", "slowAverage")) {
    nextIndex += 1;
  }

  return {
    nextIndex,
    item: {
      kind: "movingAverageSlow",
      text: `慢均线 ${windowSize}`,
      properties: {
        blockKind: "movingAverageSlow",
        windowSize,
      },
    },
  };
}

function tryParseRsi(
  statements: AstStatement[],
  index: number,
): ParsedStatementMatch | null {
  const expression = matchAssignedCall(statements[index], "latestRsi", "calculateRSI");
  if (expression === null) {
    return null;
  }

  const args = readCallArguments(expression.right);
  if (!matchesStateCloses(args[0])) {
    return null;
  }

  const period = readNumericLiteral(args[1]);
  if (period === null) {
    return null;
  }

  let nextIndex = index + 1;
  if (matchesNullGuard(statements[nextIndex], ["latestRsi"])) {
    nextIndex += 1;
  }

  return {
    nextIndex,
    item: {
      kind: "rsi",
      text: `RSI ${period}`,
      properties: {
        blockKind: "rsi",
        period,
      },
    },
  };
}

function tryParseMacd(
  statements: AstStatement[],
  index: number,
): ParsedStatementMatch | null {
  const expression = matchAssignedCall(statements[index], "latestMacd", "calculateMACD");
  if (expression === null) {
    return null;
  }

  const args = readCallArguments(expression.right);
  if (!matchesStateCloses(args[0])) {
    return null;
  }

  const fastPeriod = readNumericLiteral(args[1]);
  const slowPeriod = readNumericLiteral(args[2]);
  const signalPeriod = readNumericLiteral(args[3]);
  if (fastPeriod === null || slowPeriod === null || signalPeriod === null) {
    return null;
  }

  let nextIndex = index + 1;
  if (matchesNullGuard(statements[nextIndex], ["latestMacd"])) {
    nextIndex += 1;
  }
  if (matchesFieldAssignment(statements[nextIndex], "latestMacdDiff", "latestMacd", "diff")) {
    nextIndex += 1;
  }
  if (matchesFieldAssignment(statements[nextIndex], "latestMacdSignal", "latestMacd", "signal")) {
    nextIndex += 1;
  }
  if (matchesFieldAssignment(statements[nextIndex], "latestMacdHistogram", "latestMacd", "histogram")) {
    nextIndex += 1;
  }

  return {
    nextIndex,
    item: {
      kind: "macd",
      text: `MACD ${fastPeriod}/${slowPeriod}/${signalPeriod}`,
      properties: {
        blockKind: "macd",
        fastPeriod,
        slowPeriod,
        signalPeriod,
      },
    },
  };
}

function tryParseBollinger(
  statements: AstStatement[],
  index: number,
): ParsedStatementMatch | null {
  const expression = matchAssignedCall(
    statements[index],
    "latestBollinger",
    "calculateBollingerBands",
  );
  if (expression === null) {
    return null;
  }

  const args = readCallArguments(expression.right);
  if (!matchesStateCloses(args[0])) {
    return null;
  }

  const period = readNumericLiteral(args[1]);
  const multiplier = readNumericLiteral(args[2]);
  if (period === null || multiplier === null) {
    return null;
  }

  let nextIndex = index + 1;
  if (matchesNullGuard(statements[nextIndex], ["latestBollinger"])) {
    nextIndex += 1;
  }
  if (matchesFieldAssignment(statements[nextIndex], "latestBollingerMiddle", "latestBollinger", "middle")) {
    nextIndex += 1;
  }
  if (matchesFieldAssignment(statements[nextIndex], "latestBollingerUpper", "latestBollinger", "upper")) {
    nextIndex += 1;
  }
  if (matchesFieldAssignment(statements[nextIndex], "latestBollingerLower", "latestBollinger", "lower")) {
    nextIndex += 1;
  }

  return {
    nextIndex,
    item: {
      kind: "bollinger",
      text: `布林带 ${period}x${multiplier}`,
      properties: {
        blockKind: "bollinger",
        period,
        multiplier,
      },
    },
  };
}

function tryParseNotify(
  statement: AstStatement,
  script: string,
  index: number,
): ParsedStatementMatch | null {
  const callExpression = matchCallStatement(statement, "notify");
  const message = callExpression === null
    ? null
    : readMessageLiteral(readCallArguments(callExpression)[0], script);
  if (message === null) {
    return null;
  }

  return {
    nextIndex: index + 1,
    item: {
      kind: "notify",
      text: "发送通知",
      properties: {
        blockKind: "notify",
        message,
      },
    },
  };
}

function tryParseLog(
  statement: AstStatement,
  script: string,
  index: number,
): ParsedStatementMatch | null {
  const callExpression = matchCallStatement(statement, "console.log");
  const message = callExpression === null
    ? null
    : readLogMessage(readCallArguments(callExpression)[0], script);
  if (message === null) {
    return null;
  }

  return {
    nextIndex: index + 1,
    item: {
      kind: "log",
      text: "输出日志",
      properties: {
        blockKind: "log",
        message,
      },
    },
  };
}

function tryParsePlaceOrder(
  statements: AstStatement[],
  index: number,
  script: string,
): ParsedStatementMatch | null {
  // Look for a sequence: qty calc + console.log(...下单...) + placeOrder({...})
  // We need to scan forward to find the placeOrder call
  let scanIndex = index;

  // Skip quantity calculation statements (variable declarations)
  while (scanIndex < statements.length) {
    const stmt = statements[scanIndex];
    if (stmt === undefined) {
      return null;
    }

    // Stop when we find placeOrder
    const placeOrderCall = matchCallStatement(stmt, "placeOrder");
    if (placeOrderCall !== null) {
      break;
    }

    // Allow through: variable declarations, if statements (for guards), console.log
    if (
      stmt.type === "VariableDeclaration" ||
      stmt.type === "ExpressionStatement" ||
      stmt.type === "IfStatement"
    ) {
      scanIndex += 1;
      continue;
    }

    return null;
  }

  if (scanIndex >= statements.length) {
    return null;
  }

  const placeOrderStmt = statements[scanIndex];
  if (placeOrderStmt === undefined) {
    return null;
  }

  const placeOrderCallExpr = matchCallStatement(placeOrderStmt, "placeOrder");
  if (placeOrderCallExpr === null) {
    return null;
  }

  // Parse the placeOrder arguments (the object literal)
  const callArgs = readCallArguments(placeOrderCallExpr);
  const orderObj = callArgs[0];

  if (orderObj?.type !== "ObjectExpression") {
    return null;
  }

  const properties = new Map<string, unknown>();
  const objProps = (orderObj as AstNode & { properties?: Array<{ key?: AstNode; value?: AstNode }> }).properties ?? [];
  for (const prop of objProps) {
    const keyName = readIdentifierName(prop.key);
    if (keyName === null || prop.value === undefined) {
      continue;
    }

    if (keyName === "side") {
      const sideValue = readSource(script, prop.value).replace(/["']/g, "");
      properties.set("orderSideRaw", sideValue === "SELL" ? "SELL" : "BUY");
    } else if (keyName === "orderType") {
      const typeValue = readSource(script, prop.value).replace(/["']/g, "");
      properties.set("orderType", typeValue === "LIMIT" ? "LIMIT" : "MARKET");
    } else if (keyName === "limitPrice") {
      const numVal = readNumericLiteral(prop.value);
      if (numVal !== null) {
        properties.set("limitPrice", numVal);
      }
    }
  }

  // Determine visual side from position guard patterns in preceding statements
  const rawSide = properties.get("orderSideRaw") ?? "BUY";
  let visualSide: string = rawSide === "SELL" ? "SELL" : "BUY";

  // Scan backwards for position direction guards to infer SELL_SHORT / BUY_COVER
  for (let backIdx = scanIndex - 1; backIdx >= index; backIdx -= 1) {
    const stmt = statements[backIdx];
    if (stmt === undefined) {
      continue;
    }

    const source = readSource(script, stmt);

    if (/跳过重复开空|已有空头持仓/.test(source)) {
      visualSide = "SELL_SHORT";
      break;
    }
    if (/无空头持仓可平|跳过买入平空/.test(source)) {
      visualSide = "BUY_COVER";
      break;
    }
    if (/跳过重复开多|已有多头持仓.*跳过/.test(source)) {
      visualSide = "BUY";
      break;
    }
    if (/无多头持仓可平/.test(source)) {
      visualSide = "SELL";
      break;
    }
  }

  properties.set("side", visualSide);

  // Try to recover quantity mode and value from preceding statements
  let quantityMode: string = "shares";
  let quantityValue: number = 100;

  // Scan backwards for quantity calculation patterns
  for (let backIdx = scanIndex - 1; backIdx >= index; backIdx -= 1) {
    const stmt = statements[backIdx];
    if (stmt === undefined) {
      continue;
    }

    const source = readSource(script, stmt);

    // Check for amount mode: maxQty = Math.floor(amount / price)
    if (/maxQty\s*=\s*Math\.floor/.test(source)) {
      quantityMode = "amount";
      const amountMatch = source.match(/Math\.floor\(\s*(\d+(?:\.\d+)?)\s*\//);
      if (amountMatch !== null && amountMatch[1] !== undefined) {
        quantityValue = Number(amountMatch[1]);
      }
    }

    // Check for positionPercent mode
    if (/targetValue/.test(source) && /pos\.marketValue/.test(source)) {
      quantityMode = "positionPercent";
      const pctMatch = source.match(/\*\s*(\d+(?:\.\d+)?)\s*\/\s*100/);
      if (pctMatch !== null && pctMatch[1] !== undefined) {
        quantityValue = Number(pctMatch[1]);
      }
    }

    // Check for cashPercent mode: targetAmount = availableCash * pct / 100
    if (/targetAmount/.test(source) && /availableCash/.test(source)) {
      quantityMode = "cashPercent";
      const pctMatch = source.match(/\*\s*(\d+(?:\.\d+)?)\s*\/\s*100/);
      if (pctMatch !== null && pctMatch[1] !== undefined) {
        quantityValue = Number(pctMatch[1]);
      }
    }

    // Check for shares mode: orderQty = NNN
    const sharesMatch = source.match(/orderQty\s*=\s*(\d+(?:\.\d+)?)\s*;?\s*$/);
    if (sharesMatch !== null && sharesMatch[1] !== undefined && quantityMode === "shares") {
      quantityValue = Number(sharesMatch[1]);
    }
  }

  properties.set("quantityMode", quantityMode);
  properties.set("quantityValue", quantityValue);

  const rawVisualSide = properties.get("side");
  const side: string = typeof rawVisualSide === "string" ? rawVisualSide : "BUY";
  const blockProperties: Record<string, unknown> = {
    blockKind: "placeOrder",
    side,
    orderType: properties.get("orderType") ?? "MARKET",
    quantityMode,
    quantityValue,
  };

  const limitPrice = properties.get("limitPrice");
  if (typeof limitPrice === "number") {
    blockProperties.limitPrice = limitPrice;
  }

  const sideLabelMap: Record<string, string> = {
    BUY: "买入开多",
    SELL: "卖出平多",
    SELL_SHORT: "卖出开空",
    BUY_COVER: "买入平空",
  };
  const sideLabel = sideLabelMap[side] ?? "买入";
  const modeLabels: Record<string, string> = {
    shares: `${quantityValue} 股`,
    amount: `${quantityValue} 元`,
    positionPercent: `仓位 ${quantityValue}%`,
    cashPercent: `现金 ${quantityValue}%`,
  };
  const modeLabel = modeLabels[quantityMode] ?? `${quantityValue} 股`;

  return {
    nextIndex: scanIndex + 1,
    item: {
      kind: "placeOrder",
      text: `下单 · ${sideLabel} · ${modeLabel}`,
      properties: blockProperties,
    },
  };
}

function readLogMessage(node: AstNode | undefined, script: string): string | null {
  if (node === undefined) {
    return null;
  }

  const literalMessage = readMessageLiteral(node, script);
  if (literalMessage !== null) {
    return literalMessage;
  }

  const source = normalizeCodeSnippet(readSource(script, node));
  if (source === "") {
    return null;
  }

  return `\${${source}}`;
}

function tryParseCondition(
  statements: AstStatement[],
  index: number,
  hookKind: HookKind,
): ParsedStatementMatch | null {
  const guardPair = readGeneratedConditionGuardPair(statements, index);
  const nextIndex = guardPair?.nextIndex ?? index + 1;
  const statement = statements[guardPair?.conditionIndex ?? index];
  if (statement?.type !== "IfStatement") {
    return null;
  }

  const ifStatement = statement as AstNode & {
    test: AstNode;
    consequent: AstNode;
    alternate?: AstNode | null;
  };

  if (ifStatement.alternate !== null && ifStatement.alternate !== undefined) {
    return null;
  }

  const bodyStatements = toBodyStatements(ifStatement.consequent);
  const parsedCondition = readConditionDescriptor(ifStatement.test);

  if (parsedCondition === null) {
    return null;
  }

  return {
    nextIndex,
    item: {
      kind: parsedCondition.kind,
      text: parsedCondition.text,
      properties: parsedCondition.properties,
      children: bodyStatements,
    },
  };
}

function readGeneratedConditionGuardPair(
  statements: AstStatement[],
  index: number,
): { conditionIndex: number; nextIndex: number } | null {
  const guard = statements[index];
  const condition = statements[index + 1];

  if (guard === undefined || condition?.type !== "IfStatement") {
    return null;
  }

  const matchesGeneratedGuard =
    matchesNullGuard(guard, ["fastAverage", "slowAverage", "prevFastAverage", "prevSlowAverage"]) ||
    matchesNullGuard(guard, ["latestRsi"]) ||
    matchesNullGuard(guard, ["latestMacdDiff", "latestMacdSignal"]) ||
    matchesNullGuard(guard, ["latestBollingerUpper"]) ||
    matchesNullGuard(guard, ["latestBollingerLower"]);

  if (!matchesGeneratedGuard) {
    return null;
  }

  const conditionTest = (condition as AstNode & { test?: AstNode }).test;
  if (conditionTest === undefined || readConditionDescriptor(conditionTest) === null) {
    return null;
  }

  return {
    conditionIndex: index + 1,
    nextIndex: index + 2,
  };
}

function readHookDeclaration(
  statement: AstStatement,
): { kind: HookKind; body: AstBlockStatement } | null {
  if (statement.type === "FunctionDeclaration") {
    const name = readIdentifierName((statement as AstNode & { id?: AstNode | null }).id);
    if (!isHookName(name)) {
      return null;
    }

    const body = (statement as AstNode & { body?: AstNode }).body;
    if (body?.type !== "BlockStatement") {
      return null;
    }

    return {
      kind: name,
      body: body as AstBlockStatement,
    };
  }

  if (statement.type !== "VariableDeclaration") {
    return null;
  }

  const declarations = ((statement as AstNode & { declarations?: unknown }).declarations ?? []) as AstNode[];
  for (const declaration of declarations) {
    const kind = readIdentifierName((declaration as AstNode & { id?: AstNode | null }).id);
    if (!isHookName(kind)) {
      continue;
    }

    const init = (declaration as AstNode & { init?: AstNode | null }).init;
    if (
      init?.type !== "FunctionExpression" &&
      init?.type !== "ArrowFunctionExpression"
    ) {
      continue;
    }

    const body = (init as AstNode & { body?: AstNode }).body;
    if (body?.type !== "BlockStatement") {
      continue;
    }

    return {
      kind,
      body: body as AstBlockStatement,
    };
  }

  return null;
}

function isHookName(value: string | null): value is HookKind {
  return value === "onInit" || value === "onKLineClosed";
}

function isGeneratedRuntimeStatement(statement: AstStatement): boolean {
  if (statement.type === "VariableDeclaration") {
    const declarations = ((statement as AstNode & { declarations?: unknown }).declarations ?? []) as AstNode[];
    return declarations.some((declaration) => {
      const name = readIdentifierName((declaration as AstNode & { id?: AstNode | null }).id);
      return name === "MAX_CACHE_SIZE" || name === "state";
    });
  }

  if (statement.type === "FunctionDeclaration") {
    const name = readIdentifierName((statement as AstNode & { id?: AstNode | null }).id);
    return name !== null && GENERATED_RUNTIME_FUNCTIONS.has(name);
  }

  return false;
}

function isCodeBlockMarkerStatement(
  statement: AstStatement,
  parserContext: StrategyParserContext,
): boolean {
  const source = readSource(parserContext.script, statement).trim();
  return /^\/\/\s*@jftradeCodeBlock(Begin|End)\s*$/.test(source);
}

function isGeneratedHookPreludeStatement(
  statement: AstStatement,
  hookKind: HookKind,
): boolean {
  if (hookKind !== "onKLineClosed") {
    return false;
  }

  return (
    matchesCloseDeclaration(statement) ||
    matchesCloseGuard(statement) ||
    matchesStateClosesPush(statement) ||
    matchesStateClosesTrim(statement) ||
    matchesVariableInitialization(statement, "fastAverage") ||
    matchesVariableInitialization(statement, "slowAverage") ||
    matchesVariableInitialization(statement, "latestRsi") ||
    matchesVariableInitialization(statement, "latestMacd") ||
    matchesVariableInitialization(statement, "latestMacdDiff") ||
    matchesVariableInitialization(statement, "latestMacdSignal") ||
    matchesVariableInitialization(statement, "latestMacdHistogram") ||
    matchesVariableInitialization(statement, "latestBollinger") ||
    matchesVariableInitialization(statement, "latestBollingerMiddle") ||
    matchesVariableInitialization(statement, "latestBollingerUpper") ||
    matchesVariableInitialization(statement, "latestBollingerLower") ||
    matchesStateRead(statement, "prevFastAverage") ||
    matchesStateRead(statement, "prevSlowAverage")
  );
}

function matchesCloseDeclaration(statement: AstStatement): boolean {
  if (statement.type !== "VariableDeclaration") {
    return false;
  }
  const declarations = ((statement as AstNode & { declarations?: unknown }).declarations ?? []) as AstNode[];
  return declarations.some((declaration) => readIdentifierName((declaration as AstNode & { id?: AstNode | null }).id) === "close");
}

function matchesCloseGuard(statement: AstStatement): boolean {
  return matchesNegatedFiniteGuard(statement, "close");
}

function matchesStateClosesPush(statement: AstStatement): boolean {
  const call = matchCallStatementNode(statement);
  if (call === null) {
    return false;
  }

  return (
    isMemberExpressionChain(call.callee as AstNode, ["state", "closes", "push"]) &&
    isIdentifier(readCallArguments(call)[0], "close")
  );
}

function matchesStateClosesTrim(statement: AstStatement): boolean {
  if (statement?.type !== "IfStatement") {
    return false;
  }

  const ifStatement = statement as AstNode & { test: AstNode; consequent: AstNode; alternate?: AstNode | null };
  if (ifStatement.alternate !== null && ifStatement.alternate !== undefined) {
    return false;
  }

  const test = ifStatement.test;
  if (
    test.type !== "BinaryExpression" ||
    ((test.operator as string) !== ">") ||
    !isMemberExpressionChain((test.left as AstNode), ["state", "closes", "length"]) ||
    !isIdentifier((test.right as AstNode), "MAX_CACHE_SIZE")
  ) {
    return false;
  }

  const body = toBodyStatements(ifStatement.consequent);
  if (body.length !== 1) {
    return false;
  }

  const [firstStatement] = body;
  if (firstStatement === undefined) {
    return false;
  }

  const call = matchCallStatementNode(firstStatement);
  return call !== null && isMemberExpressionChain(call.callee as AstNode, ["state", "closes", "shift"]);
}

function matchesVariableInitialization(
  statement: AstStatement,
  identifierName: string,
): boolean {
  if (statement?.type !== "VariableDeclaration") {
    return false;
  }

  const declarations = ((statement as AstNode & { declarations?: unknown }).declarations ?? []) as AstNode[];
  return declarations.some((declaration) => readIdentifierName((declaration as AstNode & { id?: AstNode | null }).id) === identifierName);
}

function matchesStateRead(statement: AstStatement, propertyName: string): boolean {
  if (statement?.type !== "VariableDeclaration") {
    return false;
  }

  const declarations = ((statement as AstNode & { declarations?: unknown }).declarations ?? []) as AstNode[];
  return declarations.some((declaration) => {
    const name = readIdentifierName((declaration as AstNode & { id?: AstNode | null }).id);
    const init = (declaration as AstNode & { init?: AstNode | null }).init;
    return name === propertyName && isMemberExpressionChain(init, ["state", propertyName]);
  });
}

function matchesNegatedFiniteGuard(statement: AstStatement, identifierName: string): boolean {
  if (statement?.type !== "IfStatement") {
    return false;
  }

  const ifStatement = statement as AstNode & { test: AstNode; consequent: AstNode; alternate?: AstNode | null };
  if (ifStatement.alternate !== null && ifStatement.alternate !== undefined) {
    return false;
  }

  const test = ifStatement.test;
  if (
    test.type !== "UnaryExpression" ||
    (test.operator as string) !== "!"
  ) {
    return false;
  }

  const argument = test.argument as AstNode;
  const call = matchCallExpression(argument, "Number.isFinite");
  if (call === null || !isIdentifier(readCallArguments(call)[0], identifierName)) {
    return false;
  }

  return bodyEndsWithReturn(toBodyStatements(ifStatement.consequent));
}

function matchesNullGuard(statement: AstStatement | undefined, identifiers: string[]): boolean {
  if (statement?.type !== "IfStatement") {
    return false;
  }

  const ifStatement = statement as AstNode & { test: AstNode; consequent: AstNode; alternate?: AstNode | null };
  if (ifStatement.alternate !== null && ifStatement.alternate !== undefined) {
    return false;
  }

  return matchesNullComparisonChain(ifStatement.test, identifiers) && bodyEndsWithReturn(toBodyStatements(ifStatement.consequent));
}

function matchesNullComparisonChain(node: AstNode, identifiers: string[]): boolean {
  if (identifiers.length === 0) {
    return false;
  }

  const parts = flattenLogicalExpression(node, "||");
  if (parts.length !== identifiers.length) {
    return false;
  }

  return identifiers.every((identifier, index) => {
    const part = parts[index];
    return part !== undefined && matchesNullComparison(part, identifier);
  });
}

function matchesNullComparison(node: AstNode, identifierName: string): boolean {
  return node.type === "BinaryExpression" &&
    (((node.operator as string) === "==") || ((node.operator as string) === "===")) &&
    isIdentifier((node.left as AstNode), identifierName) &&
    isNullLiteral((node.right as AstNode));
}

function bodyEndsWithReturn(statements: AstStatement[]): boolean {
  if (statements.length === 0) {
    return false;
  }

  const lastStatement = statements[statements.length - 1];
  if (lastStatement === undefined || lastStatement.type !== "ReturnStatement") {
    return false;
  }

  return statements.slice(0, -1).every((statement) => {
    const call = matchCallStatement(statement, "console.log");
    return call !== null || statement.type === "EmptyStatement";
  });
}

function matchesStateAssignment(
  statement: AstStatement | undefined,
  propertyName: string,
  valueIdentifier: string,
): boolean {
  return matchesMemberAssignment(statement, ["state", propertyName], valueIdentifier);
}

function matchesFieldAssignment(
  statement: AstStatement | undefined,
  assigneeName: string,
  objectName: string,
  fieldName: string,
): boolean {
  if (statement === undefined || statement.type !== "ExpressionStatement") {
    return false;
  }

  const expression = (statement as AstNode & { expression?: AstNode }).expression;
  if (expression?.type !== "AssignmentExpression" || (expression.operator as string) !== "=") {
    return false;
  }

  return (
    isIdentifier((expression.left as AstNode), assigneeName) &&
    isMemberExpressionChain((expression.right as AstNode), [objectName, fieldName])
  );
}

function matchesMemberAssignment(
  statement: AstStatement | undefined,
  path: string[],
  valueIdentifier: string,
): boolean {
  if (statement === undefined || statement.type !== "ExpressionStatement") {
    return false;
  }

  const expression = (statement as AstNode & { expression?: AstNode }).expression;
  if (expression?.type !== "AssignmentExpression" || (expression.operator as string) !== "=") {
    return false;
  }

  return (
    isMemberExpressionChain((expression.left as AstNode), path) &&
    isIdentifier((expression.right as AstNode), valueIdentifier)
  );
}

function readCrossCondition(
  node: AstNode,
): ParsedConditionDescriptor | null {
  const parts = flattenLogicalExpression(node, "&&");
  if (parts.length !== 2) {
    return null;
  }

  const [firstPart, secondPart] = parts;
  if (firstPart === undefined || secondPart === undefined) {
    return null;
  }

  if (
    matchesBinaryComparison(firstPart, "prevFastAverage", "<=", "prevSlowAverage") &&
    matchesBinaryComparison(secondPart, "fastAverage", ">", "slowAverage")
  ) {
    return {
      kind: "ifGoldenCross",
      text: "金叉",
      properties: {
        blockKind: "ifGoldenCross",
      },
    };
  }

  if (
    matchesBinaryComparison(firstPart, "prevFastAverage", ">=", "prevSlowAverage") &&
    matchesBinaryComparison(secondPart, "fastAverage", "<", "slowAverage")
  ) {
    return {
      kind: "ifDeathCross",
      text: "死叉",
      properties: {
        blockKind: "ifDeathCross",
      },
    };
  }

  return null;
}

function readConditionDescriptor(node: AstNode): ParsedConditionDescriptor | null {
  return (
    readCrossCondition(node) ??
    readRsiCondition(node) ??
    readMacdCondition(node) ??
    readBollingerCondition(node) ??
    readCloseThresholdCondition(node)
  );
}

function readRsiCondition(
  node: AstNode,
): ParsedConditionDescriptor | null {
  const thresholdCondition = readNumericThresholdComparison(node, "latestRsi");
  if (thresholdCondition === null) {
    return null;
  }

  if (thresholdCondition.operator === ">") {
    return {
      kind: "ifRsiAbove",
      text: `RSI > ${thresholdCondition.value}`,
      properties: {
        blockKind: "ifRsiAbove",
        threshold: thresholdCondition.value,
      },
    };
  }

  return {
    kind: "ifRsiBelow",
    text: `RSI < ${thresholdCondition.value}`,
    properties: {
      blockKind: "ifRsiBelow",
      threshold: thresholdCondition.value,
    },
  };
}

function readMacdCondition(
  node: AstNode,
): ParsedConditionDescriptor | null {
  if (matchesBinaryComparison(node, "latestMacdDiff", ">", "latestMacdSignal")) {
    return {
      kind: "ifMacdBullish",
      text: "MACD 多头",
      properties: {
        blockKind: "ifMacdBullish",
      },
    };
  }

  if (matchesBinaryComparison(node, "latestMacdDiff", "<", "latestMacdSignal")) {
    return {
      kind: "ifMacdBearish",
      text: "MACD 空头",
      properties: {
        blockKind: "ifMacdBearish",
      },
    };
  }

  return null;
}

function readBollingerCondition(
  node: AstNode,
): ParsedConditionDescriptor | null {
  if (matchesCloseComparison(node, ">", "latestBollingerUpper")) {
    return {
      kind: "ifCloseAboveUpperBand",
      text: "收盘价 > 上轨",
      properties: {
        blockKind: "ifCloseAboveUpperBand",
      },
    };
  }

  if (matchesCloseComparison(node, "<", "latestBollingerLower")) {
    return {
      kind: "ifCloseBelowLowerBand",
      text: "收盘价 < 下轨",
      properties: {
        blockKind: "ifCloseBelowLowerBand",
      },
    };
  }

  return null;
}

function readCloseThresholdCondition(
  node: AstNode,
): ParsedConditionDescriptor | null {
  const numericThreshold = readCloseNumericComparison(node);
  if (numericThreshold === null) {
    return null;
  }

  if (numericThreshold.operator === ">") {
    return {
      kind: "ifCloseAbove",
      text: `收盘价 > ${numericThreshold.value}`,
      properties: {
        blockKind: "ifCloseAbove",
        threshold: numericThreshold.value,
      },
    };
  }

  return {
    kind: "ifCloseBelow",
    text: `收盘价 < ${numericThreshold.value}`,
    properties: {
      blockKind: "ifCloseBelow",
      threshold: numericThreshold.value,
    },
  };
}

function readNumericThresholdComparison(
  node: AstNode,
  leftIdentifier: string,
): { operator: ">" | "<"; value: number } | null {
  if (node.type !== "BinaryExpression") {
    return null;
  }

  const operator = node.operator as string;
  if (operator !== ">" && operator !== "<") {
    return null;
  }

  if (!isIdentifier((node.left as AstNode), leftIdentifier)) {
    return null;
  }

  const value = readNumericLiteral((node.right as AstNode));
  if (value === null) {
    return null;
  }

  return {
    operator,
    value,
  };
}

function readCloseNumericComparison(
  node: AstNode,
): { operator: ">" | "<"; value: number } | null {
  if (node.type !== "BinaryExpression") {
    return null;
  }

  const operator = node.operator as string;
  if (operator !== ">" && operator !== "<") {
    return null;
  }

  if (!isCloseExpression((node.left as AstNode))) {
    return null;
  }

  const value = readNumericLiteral((node.right as AstNode));
  if (value === null) {
    return null;
  }

  return {
    operator,
    value,
  };
}

function matchesBinaryComparison(
  node: AstNode,
  leftIdentifier: string,
  operator: string,
  rightIdentifier: string,
): boolean {
  return node.type === "BinaryExpression" &&
    (node.operator as string) === operator &&
    isIdentifier((node.left as AstNode), leftIdentifier) &&
    isIdentifier((node.right as AstNode), rightIdentifier);
}

function matchesCloseComparison(
  node: AstNode,
  operator: string,
  rightIdentifier: string,
): boolean {
  return node.type === "BinaryExpression" &&
    (node.operator as string) === operator &&
    isCloseExpression((node.left as AstNode)) &&
    isIdentifier((node.right as AstNode), rightIdentifier);
}

function isCloseExpression(node: AstNode): boolean {
  return isIdentifier(node, "close") || isMemberExpressionChain(node, ["ctx", "kline", "close"]);
}

function matchAssignedCall(
  statement: AstStatement | undefined,
  assigneeName: string,
  calleeName: string,
): { right: AstNode } | null {
  if (statement === undefined || statement.type !== "ExpressionStatement") {
    return null;
  }

  const expression = (statement as AstNode & { expression?: AstNode }).expression;
  if (expression?.type !== "AssignmentExpression" || (expression.operator as string) !== "=") {
    return null;
  }

  if (!isIdentifier((expression.left as AstNode), assigneeName)) {
    return null;
  }

  const right = expression.right as AstNode;
  if (matchCallExpression(right, calleeName) === null) {
    return null;
  }

  return {
    right,
  };
}

function matchCallStatement(statement: AstStatement, calleeName: string): AstNode | null {
  const call = matchCallStatementNode(statement);
  return call !== null && matchCallExpression(call, calleeName) !== null ? call : null;
}

function matchCallStatementNode(statement: AstStatement): AstNode | null {
  if (statement.type !== "ExpressionStatement") {
    return null;
  }

  const expression = (statement as AstNode & { expression?: AstNode }).expression;
  return expression?.type === "CallExpression" ? expression : null;
}

function matchCallExpression(node: AstNode, calleeName: string): AstNode | null {
  if (node.type !== "CallExpression") {
    return null;
  }

  const callee = node.callee as AstNode;
  if (calleeName.includes(".")) {
    return isMemberExpressionChain(callee, calleeName.split(".")) ? node : null;
  }
  return isIdentifier(callee, calleeName) ? node : null;
}

function readCallArguments(node: AstNode | undefined): AstNode[] {
  if (node?.type !== "CallExpression") {
    return [];
  }
  return ((node as AstNode & { arguments?: unknown }).arguments ?? []) as AstNode[];
}

function readNumericLiteral(node: AstNode | undefined): number | null {
  if (node === undefined) {
    return null;
  }

  if (node.type === "Literal" && typeof node.value === "number") {
    return Number.isFinite(node.value) ? node.value : null;
  }

  if (
    node.type === "UnaryExpression" &&
    (node.operator as string) === "-"
  ) {
    const argument = node.argument as AstNode;
    const inner = readNumericLiteral(argument);
    return inner === null ? null : -inner;
  }

  return null;
}

function readMessageLiteral(node: AstNode | undefined, script: string): string | null {
  if (node === undefined) {
    return null;
  }

  if (node.type === "Literal" && typeof node.value === "string") {
    return node.value;
  }

  if (node.type === "TemplateLiteral") {
    const source = readSource(script, node).trim();
    if (source.startsWith("`") && source.endsWith("`")) {
      return source.slice(1, -1);
    }
  }

  return null;
}

function resolveNodeShape(kind: ParsedVisualNode["kind"]): string {
  switch (kind) {
    case "ifGoldenCross":
    case "ifDeathCross":
    case "ifRsiAbove":
    case "ifRsiBelow":
    case "ifMacdBullish":
    case "ifMacdBearish":
    case "ifCloseAboveUpperBand":
    case "ifCloseBelowLowerBand":
    case "ifCloseAbove":
    case "ifCloseBelow":
      return "diamond";
    default:
      return "rect";
  }
}

function isConditionBlockKind(kind: ParsedVisualNode["kind"]): boolean {
  switch (kind) {
    case "ifGoldenCross":
    case "ifDeathCross":
    case "ifRsiAbove":
    case "ifRsiBelow":
    case "ifMacdBullish":
    case "ifMacdBearish":
    case "ifCloseAboveUpperBand":
    case "ifCloseBelowLowerBand":
    case "ifCloseAbove":
    case "ifCloseBelow":
      return true;
    default:
      return false;
  }
}

function readNodePosition(
  nodes: StrategyVisualNodeDocument[],
  nodeId: string,
): { x: number; y: number } | null {
  const node = nodes.find((item) => item.id === nodeId);
  if (node === undefined) {
    return null;
  }
  return {
    x: node.x,
    y: node.y,
  };
}

function createAnnotationFallbackNode(
  annotation: StrategyFlowNodeJsDoc,
): ParsedVisualNode | null {
  const text = annotation.nodeText ?? fallbackBlockText(annotation.blockKind);
  const properties = fallbackBlockProperties(annotation.blockKind, text);

  if (properties === null) {
    return null;
  }

  return {
    kind: annotation.blockKind as Exclude<StrategyBlockKind, HookKind>,
    text,
    properties,
    flowNodeId: annotation.nodeId,
  };
}

function fallbackBlockText(blockKind: string): string {
  switch (blockKind) {
    case "movingAverageFast": return "快均线 5";
    case "movingAverageSlow": return "慢均线 20";
    case "rsi": return "RSI 14";
    case "macd": return "MACD 12/26/9";
    case "bollinger": return "布林带 20x2";
    case "ifGoldenCross": return "金叉";
    case "ifDeathCross": return "死叉";
    case "ifRsiAbove": return "RSI > 70";
    case "ifRsiBelow": return "RSI < 30";
    case "ifMacdBullish": return "MACD 多头";
    case "ifMacdBearish": return "MACD 空头";
    case "ifCloseAboveUpperBand": return "收盘价 > 上轨";
    case "ifCloseBelowLowerBand": return "收盘价 < 下轨";
    case "ifCloseAbove": return "收盘价 > 500";
    case "ifCloseBelow": return "收盘价 < 500";
    case "log": return "输出日志";
    case "notify": return "发送通知";
    default: return blockKind;
  }
}

function fallbackBlockProperties(
  blockKind: string,
  text: string,
): Record<string, unknown> | null {
  switch (blockKind) {
    case "movingAverageFast":
      return { blockKind, windowSize: 5 };
    case "movingAverageSlow":
      return { blockKind, windowSize: 20 };
    case "rsi":
      return { blockKind, period: 14 };
    case "macd":
      return { blockKind, fastPeriod: 12, slowPeriod: 26, signalPeriod: 9 };
    case "bollinger":
      return { blockKind, period: 20, multiplier: 2 };
    case "ifGoldenCross":
      return { blockKind };
    case "ifDeathCross":
      return { blockKind };
    case "ifRsiAbove":
      return { blockKind, threshold: 70 };
    case "ifRsiBelow":
      return { blockKind, threshold: 30 };
    case "ifMacdBullish":
      return { blockKind };
    case "ifMacdBearish":
      return { blockKind };
    case "ifCloseAboveUpperBand":
      return { blockKind };
    case "ifCloseBelowLowerBand":
      return { blockKind };
    case "ifCloseAbove":
      return { blockKind, threshold: 500 };
    case "ifCloseBelow":
      return { blockKind, threshold: 500 };
    case "log":
      return { blockKind, message: text };
    case "notify":
      return { blockKind, message: text };
    case "codeBlock":
      return null;
    default:
      return null;
  }
}

function createCodeBlockNode(source: string, isGlobal: boolean): ParsedVisualNode {
  const code = normalizeCodeSnippet(source);
  return {
    kind: "codeBlock",
    text: buildCodeBlockLabel(code, isGlobal),
    properties: {
      blockKind: "codeBlock",
      code,
      codeScope: isGlobal ? "global" : "hook",
    },
  };
}

function applyFlowAnnotation(
  item: ParsedVisualNode,
  annotation: StrategyFlowNodeJsDoc | null,
): ParsedVisualNode {
  if (annotation === null) {
    return item;
  }

  return {
    ...item,
    text: annotation.nodeText ?? item.text,
    properties:
      item.kind === "codeBlock"
        ? applyCodeBlockAnnotationProperties(item.properties, annotation)
        : item.properties,
    flowNodeId: annotation.nodeId,
  };
}

function applyCodeBlockAnnotationProperties(
  properties: Record<string, unknown>,
  annotation: StrategyFlowNodeJsDoc | null,
): Record<string, unknown> {
  if (annotation?.codeScope === undefined) {
    return properties;
  }

  return {
    ...properties,
    codeScope: annotation.codeScope,
  };
}

function withParsedItemSourceRange(
  match: ParsedStatementMatch,
  statements: AstStatement[],
  index: number,
): ParsedStatementMatch {
  if (match.item === null) {
    return match;
  }

  return {
    nextIndex: match.nextIndex,
    item: {
      ...match.item,
      properties: withSourceRangeProperties(
        match.item.properties,
        readStatementSourceRange(statements, index, match.nextIndex),
      ),
    },
  };
}

function withSourceRangeProperties(
  properties: Record<string, unknown>,
  sourceRange: StrategySourceRange | null,
): Record<string, unknown> {
  if (sourceRange === null) {
    return properties;
  }

  return {
    ...properties,
    sourceRange,
  };
}

function createAnnotatedCodeBlockMatch(
  statements: AstStatement[],
  index: number,
  parserContext: StrategyParserContext,
  annotation: StrategyFlowNodeJsDoc,
  isGlobal: boolean,
): ParsedStatementMatch {
  const firstStatement = statements[index];
  if (firstStatement === undefined) {
    return {
      nextIndex: index + 1,
      item: null,
    };
  }

  const nextIndex = findNextAnnotatedStatementIndex(statements, index, parserContext);
  const lastStatement = statements[nextIndex - 1] ?? firstStatement;

  return {
    nextIndex,
    item: applyFlowAnnotation(
      createCodeBlockNode(
        parserContext.script.slice(firstStatement.start, lastStatement.end),
        isGlobal,
      ),
      annotation,
    ),
  };
}

function readStatementSourceRange(
  statements: AstStatement[],
  startIndex: number,
  nextIndex: number,
): StrategySourceRange | null {
  const firstStatement = statements[startIndex];
  if (firstStatement === undefined) {
    return null;
  }

  const lastStatement = statements[nextIndex - 1] ?? firstStatement;
  return {
    start: firstStatement.start,
    end: lastStatement.end,
  };
}

function findNextAnnotatedStatementIndex(
  statements: AstStatement[],
  startIndex: number,
  parserContext: StrategyParserContext,
): number {
  let nextIndex = startIndex + 1;

  while (nextIndex < statements.length) {
    if (readLeadingFlowAnnotation(statements[nextIndex], parserContext) !== null) {
      break;
    }
    nextIndex += 1;
  }

  return nextIndex;
}

function readLeadingFlowAnnotation(
  statement: AstStatement | undefined,
  parserContext: StrategyParserContext,
): StrategyFlowNodeJsDoc | null {
  if (statement === undefined) {
    return null;
  }

  let matchedAnnotation: StrategyFlowNodeJsDoc | null = null;
  const CODE_BLOCK_MARKER_PATTERN = /^\s*\/\/\s*@jftradeCodeBlock(Begin|End)\s*$/;

  for (const comment of parserContext.flowAnnotations) {
    if (comment.end > statement.start) {
      break;
    }

    const betweenText = parserContext.script
      .slice(comment.end, statement.start)
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line !== "" && !CODE_BLOCK_MARKER_PATTERN.test(line))
      .join("");

    if (betweenText !== "") {
      continue;
    }

    matchedAnnotation = comment.annotation;
  }

  return matchedAnnotation;
}

function reserveParsedNodeIdentity(
  builder: ReturnType<typeof createModelBuilder>,
  preferredNodeId: string | undefined,
  fallbackPrefix: string,
): { edgeId: string; nodeId: string } {
  while (builder.usedNodeIds.has(`${fallbackPrefix}-${builder.nextId}`)) {
    builder.nextId += 1;
  }

  const fallbackNodeId = `${fallbackPrefix}-${builder.nextId}`;
  const nodeId =
    typeof preferredNodeId === "string" &&
    preferredNodeId !== "" &&
    !builder.usedNodeIds.has(preferredNodeId)
      ? preferredNodeId
      : fallbackNodeId;
  const edgeId = `visual-edge-${builder.nextId}`;

  builder.usedNodeIds.add(nodeId);
  builder.nextId += 1;

  return {
    edgeId,
    nodeId,
  };
}

function buildCodeBlockLabel(code: string, isGlobal: boolean): string {
  const firstLine = code
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find((line) => line !== "");

  if (firstLine === undefined) {
    return isGlobal ? "全局代码" : "代码块";
  }

  const preview = firstLine.replace(/\s+/g, " ");
  const compactPreview = preview.length > 12 ? `${preview.slice(0, 12)}…` : preview;
  return isGlobal ? `全局 · ${compactPreview}` : `代码 · ${compactPreview}`;
}

function normalizeCodeSnippet(source: string): string {
  const trimmed = source.trim();
  if (trimmed === "") {
    return "";
  }

  const lines = trimmed.split(/\r?\n/);
  const markerPattern = /^\s*\/\/\s*@jftradeCodeBlock(Begin|End)\s*$/;

  const codeLines = lines.filter((line) => !markerPattern.test(line));
  if (codeLines.length === 0) {
    return "";
  }

  const followingLineIndents = codeLines
    .slice(1)
    .filter((line) => line.trim() !== "")
    .map((line) => line.match(/^\s*/)?.[0].length ?? 0);
  const sharedFollowingIndent = followingLineIndents.length === 0
    ? 0
    : Math.min(...followingLineIndents);

  return codeLines
    .map((line, index) => {
      const normalizedLine = index === 0
        ? line.trimStart()
        : line.slice(Math.min(sharedFollowingIndent, line.length));
      return normalizedLine.trimEnd();
    })
    .join("\n");
}

function readSource(script: string, node: AstNode): string {
  return script.slice(node.start, node.end);
}

function toBodyStatements(node: AstNode): AstStatement[] {
  if (node.type === "BlockStatement") {
    return [...((((node as AstBlockStatement).body) ?? []) as AstStatement[])];
  }
  return [node];
}

function flattenLogicalExpression(node: AstNode, operator: string): AstNode[] {
  if (node.type !== "LogicalExpression" || (node.operator as string) !== operator) {
    return [node];
  }

  return [
    ...flattenLogicalExpression(node.left as AstNode, operator),
    ...flattenLogicalExpression(node.right as AstNode, operator),
  ];
}

function matchesStateCloses(node: AstNode | undefined): boolean {
  return node !== undefined && isMemberExpressionChain(node, ["state", "closes"]);
}

function isIdentifier(node: AstNode | undefined | null, name: string): boolean {
  return node?.type === "Identifier" && node.name === name;
}

function readIdentifierName(node: AstNode | undefined | null): string | null {
  return node?.type === "Identifier" && typeof node.name === "string"
    ? node.name
    : null;
}

function isNullLiteral(node: AstNode | undefined): boolean {
  return node?.type === "Literal" && node.value === null;
}

function isMemberExpressionChain(
  node: AstNode | undefined | null,
  parts: string[],
): boolean {
  if (node === undefined || node === null) {
    return false;
  }

  const actualParts: string[] = [];
  let current: AstNode | undefined | null = node;

  while (current?.type === "MemberExpression") {
    if ((current.computed as boolean) === true) {
      return false;
    }

    const propertyName = readIdentifierName((current.property as AstNode | null) ?? null);
    if (propertyName === null) {
      return false;
    }
    actualParts.unshift(propertyName);
    current = (current.object as AstNode | null) ?? null;
  }

  const baseName = readIdentifierName(current);
  if (baseName === null) {
    return false;
  }
  actualParts.unshift(baseName);

  return actualParts.length === parts.length && actualParts.every((part, index) => part === parts[index]);
}