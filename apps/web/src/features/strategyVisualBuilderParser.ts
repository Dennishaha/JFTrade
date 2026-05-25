import type {
  StrategyVisualEdgeDocument,
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";
import { parse } from "acorn";

import type { StrategyBlockKind } from "./strategyVisualBuilderCatalog";
import {
  nextTechnicalIndicatorNodeText,
  normalizeTechnicalIndicatorProperties,
  type TechnicalIndicatorBlockProperties,
} from "./strategyVisualBuilderIndicatorBlock";
import { readTechnicalIndicatorProperties } from "./strategyVisualBuilderTechnicalIndicatorParsing";
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
}

interface ParsedVisualNode {
  kind: Exclude<StrategyBlockKind, HookKind>;
  text: string;
  properties: Record<string, unknown>;
  children?: AstStatement[];
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
    });

    if (match.item.kind === "codeBlock") {
      builder.codeBlockCount += 1;
    }

    if (keepsCurrentParent) {
      siblingIndex += 1;
    } else {
      currentParentId = identity.nodeId;
      siblingIndex = 0;
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
  const side = blockSource.includes('side: "SELL"') ? "SELL" : "BUY";
  const orderType = blockSource.includes('orderType: "LIMIT"') ? "LIMIT" : "MARKET";
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
          quantityMode: "shares",
          quantityValue: 100,
        },
        { start: firstStatement.start, end: lastStatement.end },
      ),
    },
  };
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

function readNodePosition(
  nodes: StrategyVisualNodeDocument[],
  nodeId: string,
): { x: number; y: number } | null {
  const node = nodes.find((item) => item.id === nodeId);
  return node === undefined ? null : { x: node.x, y: node.y };
}

function resolveNodeShape(kind: ParsedVisualNode["kind"]): "rect" | "diamond" | "circle" {
  switch (kind) {
    case "ifCloseAbove":
    case "ifCloseBelow":
      return "diamond";
    default:
      return "rect";
  }
}

