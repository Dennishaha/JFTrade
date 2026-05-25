import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";
import { parse } from "acorn";

import type { StrategyBlockKind } from "./strategyVisualBuilderCatalog";
import {
  nextGetTechnicalIndicatorNodeText,
  nextTechnicalIndicatorConditionNodeText,
  nextTechnicalIndicatorNodeText,
  normalizeGetTechnicalIndicatorProperties,
  normalizeTechnicalIndicatorConditionProperties,
  normalizeTechnicalIndicatorProperties,
  type GetTechnicalIndicatorBlockProperties,
  type TechnicalIndicatorConditionBlockProperties,
  type TechnicalIndicatorInputSlot,
  type TechnicalIndicatorBlockProperties,
} from "./strategyVisualBuilderIndicatorBlock";
import {
  buildStrategyVisualControlEdgeProperties,
  buildStrategyVisualDataEdgeProperties,
  type StrategyVisualEdgeBranch,
} from "./strategyVisualBuilderEdges";
import {
  readGetTechnicalIndicatorProperties,
  readTechnicalIndicatorProperties,
} from "./strategyVisualBuilderTechnicalIndicatorParsing";
import {
  parseStrategyFlowNodeJsDocComment,
  type StrategyFlowNodeJsDoc,
} from "./strategyVisualBuilderShared";

type HookKind = "onInit" | "onKLineClosed";

type ParseSequenceMode = "linear" | "siblings";

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

interface AstFunctionDeclaration extends AstNode {
  id?: { name?: string };
  body: AstBlockStatement;
}

interface AstIfStatement extends AstNode {
  test: AstNode;
  consequent: AstStatement;
  alternate?: AstStatement | null;
}

interface AstExpressionStatement extends AstNode {
  expression: AstNode;
}

type AstStatement = AstNode;

interface FlowAnnotationComment {
  start: number;
  end: number;
  annotation: StrategyFlowNodeJsDoc;
}

interface ParserComment {
  block: boolean;
  value: string;
  start: number;
  end: number;
}

interface StrategyParserContext {
  script: string;
  flowAnnotations: FlowAnnotationComment[];
  comments: ParserComment[];
  getterBindings: Map<string, {
    nodeId: string;
    properties: GetTechnicalIndicatorBlockProperties;
  }>;
}

interface ParsedIndicatorInputBinding {
  slot: TechnicalIndicatorInputSlot;
  getterNodeId: string;
}

interface ParsedVisualNode {
  kind: Exclude<StrategyBlockKind, HookKind>;
  text: string;
  properties: Record<string, unknown>;
  children?: AstStatement[];
  trueChildren?: AstStatement[];
  falseChildren?: AstStatement[];
  dataInputs?: ParsedIndicatorInputBinding[];
  flowNodeId?: string;
  keepParentForSiblings?: boolean;
}

interface ParsedStatementMatch {
  nextIndex: number;
  item: ParsedVisualNode | null;
}

interface ParseSequenceContext {
  hookKind: HookKind;
  parentId: string;
  baseX: number;
  baseY: number;
  rootEdgeProperties?: Record<string, unknown>;
}

interface StrategySourceRange {
  start: number;
  end: number;
}

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

export function buildStrategyVisualModelFromScript(
  script: string,
  existingModel?: StrategyVisualModelDocument | null,
): StrategyScriptParseResult {
  try {
    const flowAnnotations: FlowAnnotationComment[] = [];
    const comments: ParserComment[] = [];
    const program = parse(script, {
      ecmaVersion: "latest",
      sourceType: "script",
      onComment(block: boolean, value: string, start: number, end: number) {
        comments.push({ block, value, start, end });
        if (!block) {
          return;
        }
        const annotation = parseStrategyFlowNodeJsDocComment(value);
        if (annotation !== null) {
          flowAnnotations.push({ start, end, annotation });
        }
      },
    }) as unknown as AstProgram;

    const parserContext: StrategyParserContext = {
      script,
      flowAnnotations,
      comments,
      getterBindings: new Map(),
    };
    const builder = createModelBuilder(existingModel);
    const hookBodies = new Map<HookKind, AstStatement[]>();
    const globalStatements: AstStatement[] = [];

    for (const statement of program.body) {
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

function createModelBuilder(existingModel?: StrategyVisualModelDocument | null) {
  return {
    nodes: [
      {
        id: "on-init-root",
        type: "circle",
        x: ROOT_LAYOUT.onInit.x,
        y: ROOT_LAYOUT.onInit.y,
        text: "策略启动",
        properties: { blockKind: "onInit" },
      },
      {
        id: "on-kline-root",
        type: "circle",
        x: ROOT_LAYOUT.onKLineClosed.x,
        y: ROOT_LAYOUT.onKLineClosed.y,
        text: "K 线收盘",
        properties: { blockKind: "onKLineClosed" },
      },
    ] as StrategyVisualNodeDocument[],
    edges: [] as StrategyVisualEdgeDocument[],
    nextId: 0,
    codeBlockCount: 0,
    usedNodeIds: new Set(["on-init-root", "on-kline-root"]),
    existingPositions: buildExistingNodePositionMap(existingModel),
  };
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

function resolvePreservedPosition(
  builder: ReturnType<typeof createModelBuilder>,
  nodeId: string,
  fallbackX: number,
  fallbackY: number,
): { x: number; y: number } {
  return builder.existingPositions.get(nodeId) ?? { x: fallbackX, y: fallbackY };
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
    const annotation = readLeadingFlowAnnotation(statements, index, parserContext);
    const nextIndex = annotation === null
      ? index + 1
      : findNextAnnotatedStatementIndex(statements, index, parserContext);
    const code = normalizeCodeSnippet(
      parserContext.script.slice(statement.start, statements[nextIndex - 1]?.end ?? statement.end),
    );
    if (code === "") {
      index = nextIndex;
      continue;
    }
    const identity = reserveParsedNodeIdentity(builder, annotation?.nodeId, "global-code");
    const position = resolvePreservedPosition(
      builder,
      identity.nodeId,
      ROOT_LAYOUT.global.x + builder.codeBlockCount * BLOCK_X_STEP - BLOCK_X_STEP,
      ROOT_LAYOUT.global.y,
    );
    builder.codeBlockCount += 1;
    builder.nodes.push({
      id: identity.nodeId,
      type: "rect",
      x: position.x,
      y: position.y,
      text: annotation?.nodeText ?? buildCodeBlockLabel(code, true),
      properties: withSourceRangeProperties(
        {
          blockKind: "codeBlock",
          code,
          codeScope: "global",
        },
        {
          start: statement.start,
          end: statements[nextIndex - 1]?.end ?? statement.end,
        },
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
    const match = parseHookStatement(statements, statementIndex, parserContext);
    statementIndex = match.nextIndex;
    if (match.item === null) {
      continue;
    }

    const parentPosition = readNodePosition(builder.nodes, currentParentId) ?? {
      x: context.baseX - BLOCK_X_STEP,
      y: context.baseY,
    };
    const keepsCurrentParent = mode === "siblings" || match.item.keepParentForSiblings === true;
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

    const identity = reserveParsedNodeIdentity(builder, match.item.flowNodeId, "visual-node");
    const position = resolvePreservedPosition(builder, identity.nodeId, nodeX, nodeY);
    builder.nodes.push({
      id: identity.nodeId,
      type: resolveNodeShape(match.item.kind),
      x: position.x,
      y: position.y,
      text: match.item.text,
      properties: { ...match.item.properties },
    });
    builder.edges.push({
      id: identity.edgeId,
      type: "polyline",
      sourceNodeId: currentParentId,
      targetNodeId: identity.nodeId,
      properties:
        currentParentId === context.parentId
          ? context.rootEdgeProperties
          : undefined,
    });

    if (match.item.kind === "getTechnicalIndicator") {
      parserContext.getterBindings.set(
        buildIndicatorGetterBaseIdentifierFromNodeId(identity.nodeId),
        {
          nodeId: identity.nodeId,
          properties: normalizeGetTechnicalIndicatorProperties(match.item.properties),
        },
      );
    }

    for (const input of match.item.dataInputs ?? []) {
      builder.edges.push({
        id: `edge-data-${input.getterNodeId}-${identity.nodeId}-${input.slot}`,
        type: "polyline",
        sourceNodeId: input.getterNodeId,
        targetNodeId: identity.nodeId,
        properties: buildStrategyVisualDataEdgeProperties(input.slot),
      });
    }

    if (match.item.kind === "codeBlock") {
      builder.codeBlockCount += 1;
    }

    if (keepsCurrentParent) {
      siblingIndex += 1;
    } else {
      currentParentId = identity.nodeId;
      siblingIndex = 0;
    }

    if ((match.item.trueChildren?.length ?? 0) > 0) {
      appendHookSequence(match.item.trueChildren ?? [], parserContext, builder, {
        hookKind: context.hookKind,
        parentId: identity.nodeId,
        baseX: position.x + BLOCK_X_STEP,
        baseY: position.y - BLOCK_Y_STEP / 2,
        rootEdgeProperties: buildStrategyVisualControlEdgeProperties("true"),
      });
    }

    if ((match.item.falseChildren?.length ?? 0) > 0) {
      appendHookSequence(match.item.falseChildren ?? [], parserContext, builder, {
        hookKind: context.hookKind,
        parentId: identity.nodeId,
        baseX: position.x + BLOCK_X_STEP,
        baseY: position.y + BLOCK_Y_STEP / 2,
        rootEdgeProperties: buildStrategyVisualControlEdgeProperties("false"),
      });
    }

    if ((match.item.children?.length ?? 0) > 0) {
      appendHookSequence(match.item.children ?? [], parserContext, builder, {
        hookKind: context.hookKind,
        parentId: identity.nodeId,
        baseX: position.x + BLOCK_X_STEP,
        baseY: position.y + BLOCK_Y_STEP,
      });
    }
  }
}

function parseHookStatement(
  statements: AstStatement[],
  index: number,
  parserContext: StrategyParserContext,
): ParsedStatementMatch {
  const statement = statements[index];
  if (statement === undefined) {
    return { nextIndex: index + 1, item: null };
  }

  const annotation = readLeadingFlowAnnotation(statements, index, parserContext);
  if (annotation?.blockKind === "getTechnicalIndicator") {
    return parseGetTechnicalIndicatorNode(statements, index, parserContext, annotation);
  }
  if (annotation?.blockKind === "technicalIndicatorCondition") {
    return parseTechnicalIndicatorConditionNode(statement, parserContext, annotation, index);
  }
  if (annotation?.blockKind === "technicalIndicator") {
    return parseTechnicalIndicatorNode(statements, index, parserContext, annotation);
  }
  if (annotation?.blockKind === "log") {
    return parseLogNode(statement, parserContext, annotation, index);
  }
  if (annotation?.blockKind === "notify") {
    return parseNotifyNode(statement, parserContext, annotation, index);
  }
  if (annotation?.blockKind === "placeOrder") {
    return parsePlaceOrderNode(statements, index, parserContext, annotation);
  }
  if (annotation?.blockKind === "codeBlock") {
    return parseCodeBlockNode(statements, index, parserContext, annotation);
  }
  if (annotation?.blockKind === "ifCloseAbove" || annotation?.blockKind === "ifCloseBelow") {
    return parseCloseConditionNode(statement, parserContext, annotation, index);
  }

  const rawLog = tryParseRawLogStatement(statement, parserContext.script);
  if (rawLog !== null) {
    return {
      nextIndex: index + 1,
      item: {
        kind: "log",
        text: "输出日志",
        properties: withSourceRangeProperties(
          {
            blockKind: "log",
            message: rawLog,
          },
          { start: statement.start, end: statement.end },
        ),
      },
    };
  }

  const rawNotify = tryParseRawNotifyStatement(statement, parserContext.script);
  if (rawNotify !== null) {
    return {
      nextIndex: index + 1,
      item: {
        kind: "notify",
        text: "发送通知",
        properties: withSourceRangeProperties(
          {
            blockKind: "notify",
            message: rawNotify,
          },
          { start: statement.start, end: statement.end },
        ),
      },
    };
  }

  if (isGeneratedHookPreludeStatement(statement, parserContext.script)) {
    return {
      nextIndex: index + 1,
      item: null,
    };
  }

  return {
    nextIndex: index + 1,
    item: buildFallbackCodeBlock(statement, parserContext),
  };
}

function parseGetTechnicalIndicatorNode(
  statements: AstStatement[],
  index: number,
  parserContext: StrategyParserContext,
  annotation: StrategyFlowNodeJsDoc,
): ParsedStatementMatch {
  const statement = statements[index];
  if (statement === undefined) {
    return { nextIndex: index + 1, item: null };
  }

  const groupEnd = findNextAnnotatedStatementIndex(statements, index, parserContext);
  const lastStatement = statements[groupEnd - 1] ?? statement;
  const slice = parserContext.script.slice(statement.start, lastStatement.end);
  const normalized = normalizeGetTechnicalIndicatorProperties(
    readGetTechnicalIndicatorProperties(slice),
  );
  const variableName = annotation.variableName;

  return {
    nextIndex: groupEnd,
    item: {
      kind: "getTechnicalIndicator",
      text: annotation.nodeText ?? nextGetTechnicalIndicatorNodeText({ ...normalized }),
      flowNodeId: annotation.nodeId,
      properties: withSourceRangeProperties(
        {
          ...normalized,
          ...(variableName === undefined || variableName === "" ? {} : { variableName }),
          blockKind: "getTechnicalIndicator",
        },
        { start: statement.start, end: lastStatement.end },
      ),
    },
  };
}

function parseTechnicalIndicatorConditionNode(
  statement: AstStatement,
  parserContext: StrategyParserContext,
  annotation: StrategyFlowNodeJsDoc,
  index: number,
): ParsedStatementMatch {
  if (statement.type !== "IfStatement") {
    return {
      nextIndex: index + 1,
      item: buildFallbackCodeBlock(statement, parserContext),
    };
  }

  const ifStatement = statement as unknown as AstIfStatement;
  const testSource = parserContext.script.slice(
    ifStatement.test.start,
    ifStatement.test.end,
  );
  const dataInputs = readConditionInputBindings(
    annotation,
    testSource,
    parserContext.getterBindings,
  );
  const normalized = normalizeTechnicalIndicatorConditionProperties(
    readTechnicalIndicatorConditionPropertiesFromSource(
      testSource,
      dataInputs,
      parserContext.getterBindings,
    ),
  );

  const item: ParsedVisualNode = {
    kind: "technicalIndicatorCondition",
    text: annotation.nodeText ?? nextTechnicalIndicatorConditionNodeText({ ...normalized }),
    flowNodeId: annotation.nodeId,
    dataInputs,
    properties: withSourceRangeProperties(
      {
        ...normalized,
        ...(dataInputs.find((input) => input.slot === "primary")?.getterNodeId === undefined
          ? {}
          : { inputPrimaryNodeId: dataInputs.find((input) => input.slot === "primary")?.getterNodeId }),
        ...(dataInputs.find((input) => input.slot === "fast")?.getterNodeId === undefined
          ? {}
          : { inputFastNodeId: dataInputs.find((input) => input.slot === "fast")?.getterNodeId }),
        ...(dataInputs.find((input) => input.slot === "slow")?.getterNodeId === undefined
          ? {}
          : { inputSlowNodeId: dataInputs.find((input) => input.slot === "slow")?.getterNodeId }),
        blockKind: "technicalIndicatorCondition",
      },
      { start: statement.start, end: statement.end },
    ),
  };
  const trueChildren = readIfChildren(statement);
  const falseChildren = readElseChildren(statement);
  if (trueChildren !== undefined) {
    item.trueChildren = trueChildren;
  }
  if (falseChildren !== undefined) {
    item.falseChildren = falseChildren;
  }

  return {
    nextIndex: index + 1,
    item,
  };
}

function parseTechnicalIndicatorNode(
  statements: AstStatement[],
  index: number,
  parserContext: StrategyParserContext,
  annotation: StrategyFlowNodeJsDoc,
): ParsedStatementMatch {
  const statement = statements[index];
  if (statement === undefined) {
    return { nextIndex: index + 1, item: null };
  }

  const groupEnd = findNextAnnotatedStatementIndex(statements, index, parserContext);
  const slice = parserContext.script.slice(statement.start, statements[groupEnd - 1]?.end ?? statement.end);
  const properties = readTechnicalIndicatorProperties(slice);
  const normalized = normalizeTechnicalIndicatorProperties(properties);
  const lastStatement = statements[groupEnd - 1] ?? statement;
  const children = readTechnicalIndicatorChildren(lastStatement);
  const item: ParsedVisualNode = {
    kind: "technicalIndicator",
    text: annotation.nodeText ?? nextTechnicalIndicatorNodeText(normalized as unknown as Record<string, unknown>),
    flowNodeId: annotation.nodeId,
    keepParentForSiblings: normalized.conditionMode !== "none",
    properties: withSourceRangeProperties(
      {
        ...(normalized as unknown as Record<string, unknown>),
        blockKind: "technicalIndicator",
      },
      { start: statement.start, end: lastStatement.end },
    ),
  };

  if (children !== undefined) {
    item.children = children;
  }

  return {
    nextIndex: groupEnd,
    item,
  };
}

function parseLogNode(
  statement: AstStatement,
  parserContext: StrategyParserContext,
  annotation: StrategyFlowNodeJsDoc,
  index: number,
): ParsedStatementMatch {
  const message = tryParseConsoleLogMessage(statement, parserContext.script) ?? "观察到新的策略事件";
  return {
    nextIndex: index + 1,
    item: {
      kind: "log",
      text: annotation.nodeText ?? "输出日志",
      flowNodeId: annotation.nodeId,
      properties: withSourceRangeProperties(
        {
          blockKind: "log",
          message,
        },
        { start: statement.start, end: statement.end },
      ),
    },
  };
}

function parseNotifyNode(
  statement: AstStatement,
  parserContext: StrategyParserContext,
  annotation: StrategyFlowNodeJsDoc,
  index: number,
): ParsedStatementMatch {
  const message = tryParseNotifyMessage(statement, parserContext.script) ?? "策略条件命中，准备处理后续动作";
  return {
    nextIndex: index + 1,
    item: {
      kind: "notify",
      text: annotation.nodeText ?? "发送通知",
      flowNodeId: annotation.nodeId,
      properties: withSourceRangeProperties(
        {
          blockKind: "notify",
          message,
        },
        { start: statement.start, end: statement.end },
      ),
    },
  };
}

function parsePlaceOrderNode(
  statements: AstStatement[],
  index: number,
  parserContext: StrategyParserContext,
  annotation: StrategyFlowNodeJsDoc,
): ParsedStatementMatch {
  let endIndex = index;
  while (endIndex < statements.length) {
    const currentStatement = statements[endIndex];
    if (currentStatement === undefined) {
      break;
    }
    const source = parserContext.script.slice(currentStatement.start, currentStatement.end);
    if (source.includes("placeOrder({")) {
      break;
    }
    endIndex += 1;
  }
  const firstStatement = statements[index];
  const lastStatement = statements[Math.min(endIndex, statements.length - 1)] ?? firstStatement;
  if (firstStatement === undefined || lastStatement === undefined) {
    return { nextIndex: index + 1, item: null };
  }
  const blockSource = parserContext.script.slice(firstStatement.start, lastStatement.end);
  const side = readPlaceOrderSide(blockSource);
  const orderType = blockSource.includes('orderType: "LIMIT"') ? "LIMIT" : "MARKET";
  const quantityMode = readPlaceOrderQuantityMode(blockSource);
  const quantityValue = readPlaceOrderQuantityValue(blockSource, quantityMode);
  const entryPositionPolicy = readPlaceOrderEntryPositionPolicy(blockSource, side);
  const limitPrice = readPlaceOrderLimitPrice(blockSource);
  return {
    nextIndex: Math.min(endIndex + 1, statements.length),
    item: {
      kind: "placeOrder",
      text: annotation.nodeText ?? "下单",
      flowNodeId: annotation.nodeId,
      properties: withSourceRangeProperties(
        {
          blockKind: "placeOrder",
          side,
          orderType,
          entryPositionPolicy,
          quantityMode,
          quantityValue,
          limitPrice,
        },
        { start: firstStatement.start, end: lastStatement.end },
      ),
    },
  };
}

function readPlaceOrderSide(
  blockSource: string,
): "BUY" | "SELL" | "SELL_SHORT" | "BUY_COVER" {
  if (blockSource.includes("卖出开空")) {
    return "SELL_SHORT";
  }
  if (blockSource.includes("买入平空")) {
    return "BUY_COVER";
  }
  if (blockSource.includes('side: "SELL"')) {
    return "SELL";
  }
  return "BUY";
}

function readPlaceOrderQuantityMode(
  blockSource: string,
): "shares" | "amount" | "accountPositionPercent" | "symbolPositionPercent" | "cashPercent" {
  if (blockSource.includes("const accountTotalValue = getTotalAccountValue();")) {
    return "accountPositionPercent";
  }
  if (blockSource.includes("const availableCash = getAvailableCash();")) {
    return "cashPercent";
  }
  if (
    blockSource.includes("const currentPositionValue = pos ? Math.abs(pos.marketValue) : 0;") ||
    blockSource.includes("const targetValue = (pos && pos.marketValue > 0 ? pos.marketValue : 0) * ")
  ) {
    return "symbolPositionPercent";
  }
  if (blockSource.includes("const maxQty = Math.floor(") && blockSource.includes("/ orderPrice);")) {
    return "amount";
  }
  return "shares";
}

function readPlaceOrderQuantityValue(
  blockSource: string,
  quantityMode: "shares" | "amount" | "accountPositionPercent" | "symbolPositionPercent" | "cashPercent",
): number {
  const patterns: Record<typeof quantityMode, RegExp> = {
    shares: /const orderQty = (-?\d+(?:\.\d+)?);/,
    amount: /const maxQty = Math\.floor\((-?\d+(?:\.\d+)?) \/ orderPrice\);/,
    accountPositionPercent: /const targetAmount = accountTotalValue \* (-?\d+(?:\.\d+)?) \/ 100;/,
    symbolPositionPercent: /const targetValue = (?:(?:currentPositionValue)|(?:\(pos && pos\.marketValue > 0 \? pos\.marketValue : 0\))) \* (-?\d+(?:\.\d+)?) \/ 100;/,
    cashPercent: /const targetAmount = availableCash \* (-?\d+(?:\.\d+)?) \/ 100;/,
  };
  const match = blockSource.match(patterns[quantityMode]);
  return match?.[1] === undefined ? 100 : Number(match[1]);
}

function readPlaceOrderEntryPositionPolicy(
  blockSource: string,
  side: "BUY" | "SELL" | "SELL_SHORT" | "BUY_COVER",
): "sameDirection" | "flatOnly" | "allow" {
  if (side !== "BUY" && side !== "SELL_SHORT") {
    return "sameDirection";
  }
  if (blockSource.includes("if (pos && pos.quantity !== 0) {")) {
    return "flatOnly";
  }
  if (side === "BUY" && blockSource.includes('if (pos && pos.direction === "LONG" && availablePositionQty > 0) {')) {
    return "sameDirection";
  }
  if (side === "SELL_SHORT" && blockSource.includes('if (pos && pos.direction === "SHORT" && availablePositionQty > 0) {')) {
    return "sameDirection";
  }
  return "allow";
}

function readPlaceOrderLimitPrice(blockSource: string): number {
  const match = blockSource.match(/limitPrice: (-?\d+(?:\.\d+)?)/);
  return match?.[1] === undefined ? 0 : Number(match[1]);
}

function parseCodeBlockNode(
  statements: AstStatement[],
  index: number,
  parserContext: StrategyParserContext,
  annotation: StrategyFlowNodeJsDoc,
): ParsedStatementMatch {
  const statement = statements[index];
  if (statement === undefined) {
    return { nextIndex: index + 1, item: null };
  }
  const beginComment = parserContext.comments.find((comment) =>
    !comment.block && comment.value.trim() === "@jftradeCodeBlockBegin" && comment.end <= statement.start,
  );
  const endComment = parserContext.comments.find((comment) =>
    !comment.block && comment.value.trim() === "@jftradeCodeBlockEnd" && comment.start >= statement.end,
  );
  const codeEnd = endComment?.start ?? statement.end;
  let nextIndex = index;
  while (nextIndex < statements.length) {
    const currentStatement = statements[nextIndex];
    if (currentStatement === undefined || currentStatement.end > codeEnd) {
      break;
    }
    nextIndex += 1;
  }
  const lastStatement = statements[Math.max(index, nextIndex - 1)] ?? statement;
  const code = normalizeCodeSnippet(parserContext.script.slice(statement.start, lastStatement.end));
  return {
    nextIndex,
    item: {
      kind: "codeBlock",
      text: annotation.nodeText ?? buildCodeBlockLabel(code, false),
      flowNodeId: annotation.nodeId,
      properties: withSourceRangeProperties(
        {
          blockKind: "codeBlock",
          code,
          codeScope: annotation.codeScope ?? "hook",
        },
        { start: statement.start, end: lastStatement.end },
      ),
    },
  };
}

function parseCloseConditionNode(
  statement: AstStatement,
  parserContext: StrategyParserContext,
  annotation: StrategyFlowNodeJsDoc,
  index: number,
): ParsedStatementMatch {
  const source = parserContext.script.slice(statement.start, statement.end);
  const thresholdMatch = source.match(/ctx\.kline\.close\s*[<>]\s*(-?\d+(?:\.\d+)?)/);
  const threshold = thresholdMatch === null ? 500 : Number(thresholdMatch[1]);
  const children = readIfChildren(statement);
  const item: ParsedVisualNode = {
    kind: annotation.blockKind === "ifCloseAbove" ? "ifCloseAbove" : "ifCloseBelow",
    text: annotation.nodeText ?? (annotation.blockKind === "ifCloseAbove" ? `收盘价 > ${threshold}` : `收盘价 < ${threshold}`),
    flowNodeId: annotation.nodeId,
    keepParentForSiblings: true,
    properties: withSourceRangeProperties(
      {
        blockKind: annotation.blockKind === "ifCloseAbove" ? "ifCloseAbove" : "ifCloseBelow",
        threshold,
      },
      { start: statement.start, end: statement.end },
    ),
  };
  if (children !== undefined) {
    item.children = children;
  }
  return {
    nextIndex: index + 1,
    item,
  };
}

function readTechnicalIndicatorChildren(statement: AstStatement): AstStatement[] | undefined {
  if (statement.type !== "IfStatement") {
    return undefined;
  }
  return readIfChildren(statement);
}

function readIfChildren(statement: AstStatement): AstStatement[] | undefined {
  if (statement.type !== "IfStatement") {
    return undefined;
  }
  const ifStatement = statement as unknown as AstIfStatement;
  if (ifStatement.consequent.type === "BlockStatement") {
    return (ifStatement.consequent as AstBlockStatement).body;
  }
  return [ifStatement.consequent];
}

function readElseChildren(statement: AstStatement): AstStatement[] | undefined {
  if (statement.type !== "IfStatement") {
    return undefined;
  }
  const ifStatement = statement as unknown as AstIfStatement;
  const alternate = ifStatement.alternate;
  if (alternate === undefined || alternate === null) {
    return undefined;
  }
  if (alternate.type === "BlockStatement") {
    return (alternate as AstBlockStatement).body;
  }
  return [alternate];
}

function readConditionInputBindings(
  annotation: StrategyFlowNodeJsDoc,
  testSource: string,
  getterBindings: StrategyParserContext["getterBindings"],
): ParsedIndicatorInputBinding[] {
  const annotated = readConditionInputBindingsFromAnnotation(annotation, getterBindings);
  if (annotated.length > 0) {
    return annotated;
  }

  const fallbackBases = readIndicatorBaseIdentifiers(testSource)
    .map((base) => getterBindings.get(base))
    .filter((binding): binding is NonNullable<typeof binding> => binding !== undefined);

  if (fallbackBases.length >= 2) {
    const fast = fallbackBases[0];
    const slow = fallbackBases[1];
    if (fast === undefined || slow === undefined) {
      return [];
    }
    return [
      { slot: "fast", getterNodeId: fast.nodeId },
      { slot: "slow", getterNodeId: slow.nodeId },
    ];
  }
  if (fallbackBases.length === 1) {
    const primary = fallbackBases[0];
    return primary === undefined
      ? []
      : [{ slot: "primary", getterNodeId: primary.nodeId }];
  }
  return [];
}

function readConditionInputBindingsFromAnnotation(
  annotation: StrategyFlowNodeJsDoc,
  getterBindings: StrategyParserContext["getterBindings"],
): ParsedIndicatorInputBinding[] {
  const bindings: ParsedIndicatorInputBinding[] = [];
  const pushIfKnown = (
    slot: TechnicalIndicatorInputSlot,
    getterNodeId: string | undefined,
  ) => {
    if (getterNodeId === undefined || getterNodeId === "") {
      return;
    }
    const hasGetter = [...getterBindings.values()].some(
      (binding) => binding.nodeId === getterNodeId,
    );
    if (hasGetter) {
      bindings.push({ slot, getterNodeId });
    }
  };

  pushIfKnown("primary", annotation.inputPrimaryNodeId);
  pushIfKnown("fast", annotation.inputFastNodeId);
  pushIfKnown("slow", annotation.inputSlowNodeId);
  return bindings;
}

function readTechnicalIndicatorConditionPropertiesFromSource(
  testSource: string,
  dataInputs: ParsedIndicatorInputBinding[],
  getterBindings: StrategyParserContext["getterBindings"],
): Record<string, unknown> {
  const primaryGetter = readGetterBindingForInput(dataInputs, getterBindings, "primary")
    ?? readGetterBindingForInput(dataInputs, getterBindings, "fast");

  const divergenceMatch = testSource.match(
    /ctx\.indicators\[(?:"|')divergence:(rsi|macd|kdj):([^"']+):(top|bottom):(\d+)(?:"|')\]\s*\?\?\s*false/,
  );
  if (divergenceMatch !== null) {
    return {
      blockKind: "technicalIndicatorCondition",
      indicatorType: primaryGetter?.properties.indicatorType ?? divergenceMatch[1] ?? "rsi",
      conditionMode: "pattern",
      patternType: divergenceMatch[3] === "top" ? "topDivergence" : "bottomDivergence",
      lookback: Number(divergenceMatch[4] ?? 5),
    };
  }

  if (testSource.includes("_upper") || testSource.includes("_lower")) {
    return {
      blockKind: "technicalIndicatorCondition",
      indicatorType: primaryGetter?.properties.indicatorType ?? "bollinger",
      conditionMode: "pattern",
      patternType: testSource.includes(" > ")
        ? "closeAboveUpperBand"
        : "closeBelowLowerBand",
    };
  }

  if (testSource.includes("_previous !== null") && testSource.includes("_value")) {
    return {
      blockKind: "technicalIndicatorCondition",
      indicatorType: "movingAverage",
      conditionMode: "pattern",
      patternType: testSource.includes(" >= ") && testSource.includes(" < ")
        ? "deathCross"
        : "goldenCross",
    };
  }

  if (testSource.includes("_previous_diff") || testSource.includes("_previous_signal")) {
    return {
      blockKind: "technicalIndicatorCondition",
      indicatorType: primaryGetter?.properties.indicatorType ?? "macd",
      conditionMode: "pattern",
      patternType: testSource.includes(" >= ") && testSource.includes(" < ")
        ? "deathCross"
        : "goldenCross",
    };
  }

  if (testSource.includes("_previous_k") || testSource.includes("_previous_d")) {
    return {
      blockKind: "technicalIndicatorCondition",
      indicatorType: primaryGetter?.properties.indicatorType ?? "kdj",
      conditionMode: "pattern",
      patternType: testSource.includes(" >= ") && testSource.includes(" < ")
        ? "deathCross"
        : "goldenCross",
    };
  }

  const numericMatch = testSource.match(
    /\bindicator_[a-zA-Z0-9_]+(?:_(?:histogram|j))?\s*([<>])\s*(-?\d+(?:\.\d+)?)/,
  );
  if (numericMatch !== null) {
    return {
      blockKind: "technicalIndicatorCondition",
      indicatorType: primaryGetter?.properties.indicatorType
        ?? (testSource.includes("_histogram")
          ? "macd"
          : testSource.includes("_j")
            ? "kdj"
            : "rsi"),
      conditionMode: "numeric",
      operator: numericMatch[1] ?? "<",
      threshold: Number(numericMatch[2] ?? 0),
    };
  }

  return {
    blockKind: "technicalIndicatorCondition",
    indicatorType: primaryGetter?.properties.indicatorType ?? "rsi",
    conditionMode: "numeric",
  };
}

function readGetterBindingForInput(
  dataInputs: ParsedIndicatorInputBinding[],
  getterBindings: StrategyParserContext["getterBindings"],
  slot: TechnicalIndicatorInputSlot,
) {
  const input = dataInputs.find((binding) => binding.slot === slot);
  if (input === undefined) {
    return undefined;
  }
  return [...getterBindings.values()].find(
    (binding) => binding.nodeId === input.getterNodeId,
  );
}

function readIndicatorBaseIdentifiers(testSource: string): string[] {
  const matches = testSource.matchAll(/\b(indicator_[a-zA-Z0-9_]+?)(?:_(?:snapshot|value|previous|diff|signal|histogram|previous_diff|previous_signal|k|d|j|previous_k|previous_d|middle|upper|lower))?\b/g);
  const result: string[] = [];
  for (const match of matches) {
    const value = match[1];
    if (value !== undefined && !result.includes(value)) {
      result.push(value);
    }
  }
  return result;
}

function tryParseRawLogStatement(statement: AstStatement, script: string): string | null {
  return tryParseConsoleLogMessage(statement, script);
}

function tryParseRawNotifyStatement(statement: AstStatement, script: string): string | null {
  return tryParseNotifyMessage(statement, script);
}

function tryParseConsoleLogMessage(statement: AstStatement, script: string): string | null {
  if (statement.type !== "ExpressionStatement") {
    return null;
  }
  const source = script.slice(statement.start, statement.end).trim();
  const match = source.match(/^console\.log\((.*)\);?$/s);
  if (match === null) {
    return null;
  }
  return normalizeExpressionMessage(match[1] ?? "");
}

function tryParseNotifyMessage(statement: AstStatement, script: string): string | null {
  if (statement.type !== "ExpressionStatement") {
    return null;
  }
  const source = script.slice(statement.start, statement.end).trim();
  const match = source.match(/^notify\((.*)\);?$/s);
  if (match === null) {
    return null;
  }
  return normalizeExpressionMessage(match[1] ?? "");
}

function normalizeExpressionMessage(expression: string): string {
  const trimmed = expression.trim();
  if (trimmed.startsWith("`") && trimmed.endsWith("`")) {
    return trimmed.slice(1, -1);
  }
  if ((trimmed.startsWith('"') && trimmed.endsWith('"')) || (trimmed.startsWith("'") && trimmed.endsWith("'"))) {
    return JSON.parse(trimmed);
  }
  return `\${${trimmed}}`;
}

function buildFallbackCodeBlock(
  statement: AstStatement,
  parserContext: StrategyParserContext,
): ParsedVisualNode {
  const code = normalizeCodeSnippet(parserContext.script.slice(statement.start, statement.end));
  return {
    kind: "codeBlock",
    text: buildCodeBlockLabel(code, false),
    properties: withSourceRangeProperties(
      {
        blockKind: "codeBlock",
        code,
        codeScope: "hook",
      },
      { start: statement.start, end: statement.end },
    ),
  };
}

function readHookDeclaration(statement: AstStatement): { kind: HookKind; body: AstBlockStatement } | null {
  if (statement.type !== "FunctionDeclaration") {
    return null;
  }
  const declaration = statement as unknown as AstFunctionDeclaration;
  const name = declaration.id?.name;
  if (name !== "onInit" && name !== "onKLineClosed") {
    return null;
  }
  return { kind: name, body: declaration.body };
}

function readLeadingFlowAnnotation(
  statements: AstStatement[],
  index: number,
  parserContext: StrategyParserContext,
): StrategyFlowNodeJsDoc | null {
  const statement = statements[index];
  if (statement === undefined) {
    return null;
  }
  const previousEnd = index > 0 ? statements[index - 1]?.end ?? 0 : 0;
  for (let annotationIndex = parserContext.flowAnnotations.length - 1; annotationIndex >= 0; annotationIndex -= 1) {
    const annotation = parserContext.flowAnnotations[annotationIndex];
    const gap = annotation === undefined ? "" : parserContext.script.slice(annotation.end, statement.start);
    if (
      annotation !== undefined
      && annotation.end <= statement.start
      && annotation.end >= previousEnd
      && stripComments(gap).trim() === ""
    ) {
      return annotation.annotation;
    }
  }
  return null;
}

function stripComments(value: string): string {
  return value
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/\/\/[^\n\r]*/g, "");
}

function findNextAnnotatedStatementIndex(
  statements: AstStatement[],
  index: number,
  parserContext: StrategyParserContext,
): number {
  for (let nextIndex = index + 1; nextIndex < statements.length; nextIndex += 1) {
    if (readLeadingFlowAnnotation(statements, nextIndex, parserContext) !== null) {
      return nextIndex;
    }
  }
  return statements.length;
}

function normalizeCodeSnippet(code: string): string {
  const lines = code.split(/\r?\n/);
  while (lines.length > 0 && lines[0]?.trim() === "") {
    lines.shift();
  }
  while (lines.length > 0 && lines[lines.length - 1]?.trim() === "") {
    lines.pop();
  }
  if (lines.length === 0) {
    return "";
  }

  const [firstLine, ...restLines] = lines;
  let minimumIndent = Number.POSITIVE_INFINITY;
  for (const line of restLines) {
    if (line.trim() === "") {
      continue;
    }
    const indent = line.match(/^\s*/)?.[0].length ?? 0;
    minimumIndent = Math.min(minimumIndent, indent);
  }
  const normalizedLines = [firstLine?.trimStart() ?? ""];
  if (!Number.isFinite(minimumIndent) || minimumIndent <= 0) {
    normalizedLines.push(...restLines);
  } else {
    normalizedLines.push(
      ...restLines.map((line) => {
        if (line.trim() === "") {
          return "";
        }
        return line.slice(Math.min(minimumIndent, line.length));
      }),
    );
  }
  return normalizedLines.join("\n").trim();
}

function isGeneratedHookPreludeStatement(statement: AstStatement, script: string): boolean {
  const source = script.slice(statement.start, statement.end).trim();
  return source === "const close = Number(ctx && ctx.kline ? ctx.kline.close : NaN);"
    || /^if \(!Number\.isFinite\(close\)\) \{\s*console\.log\("skip candle because close is not a finite number"\);\s*return;\s*\}$/s.test(source)
    || /^let\s+(?:fastAverageSnapshot|slowAverageSnapshot|fastAverage|slowAverage|prevFastAverage|prevSlowAverage|latestRsi|latestMacd|latestMacdDiff|latestMacdSignal|latestMacdHistogram|latestKdj|latestKValue|latestDValue|latestJValue|previousKValue|previousDValue|latestAtr|latestCci|latestWilliamsR|latestBollinger|latestBollingerMiddle|latestBollingerUpper|latestBollingerLower|divergenceSignal)\s*=\s*null;$/.test(source)
    || /^let\s+divergenceSignal\s*=\s*false;$/.test(source);
}

function buildCodeBlockLabel(code: string, isGlobal: boolean): string {
  const prefix = isGlobal ? "全局代码" : "代码块";
  const firstLine = code.split(/\r?\n/, 1)[0]?.trim() ?? "";
  if (firstLine === "") {
    return prefix;
  }
  return `${prefix} · ${firstLine.slice(0, 18)}`;
}

function withSourceRangeProperties(
  properties: Record<string, unknown>,
  sourceRange: StrategySourceRange,
): Record<string, unknown> {
  return {
    ...properties,
    sourceRange,
  };
}

function reserveParsedNodeIdentity(
  builder: ReturnType<typeof createModelBuilder>,
  preferredId: string | undefined,
  prefix: string,
): { nodeId: string; edgeId: string } {
  const nodeId = reserveNodeId(builder, preferredId, prefix);
  return {
    nodeId,
    edgeId: `edge-${nodeId}`,
  };
}

function reserveNodeId(
  builder: ReturnType<typeof createModelBuilder>,
  preferredId: string | undefined,
  prefix: string,
): string {
  if (preferredId !== undefined && !builder.usedNodeIds.has(preferredId)) {
    builder.usedNodeIds.add(preferredId);
    return preferredId;
  }
  while (true) {
    builder.nextId += 1;
    const candidate = `${prefix}-${builder.nextId}`;
    if (!builder.usedNodeIds.has(candidate)) {
      builder.usedNodeIds.add(candidate);
      return candidate;
    }
  }
}

function buildIndicatorGetterBaseIdentifierFromNodeId(nodeId: string): string {
  return `indicator_${sanitizeScriptIdentifier(nodeId)}`;
}

function sanitizeScriptIdentifier(value: string): string {
  const normalized = value
    .replace(/[^a-zA-Z0-9_]+/g, "_")
    .replace(/^([0-9])/, "_$1");
  return normalized === "" ? "node" : normalized;
}

function readNodePosition(
  nodes: StrategyVisualNodeDocument[],
  nodeId: string,
): { x: number; y: number } | null {
  const node = nodes.find((item) => item.id === nodeId);
  return node === undefined ? null : { x: node.x, y: node.y };
}

function resolveNodeShape(kind: ParsedVisualNode["kind"]): "rect" | "diamond" | "circle" {
  switch (kind) {
    case "technicalIndicatorCondition":
    case "ifCloseAbove":
    case "ifCloseBelow":
      return "diamond";
    default:
      return "rect";
  }
}

