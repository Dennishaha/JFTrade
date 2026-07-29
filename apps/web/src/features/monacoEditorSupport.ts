import type { editor as MonacoEditorNamespace } from "monaco-editor";

export interface MonacoExtraLibConfig {
  filePath: string;
  content: string;
}

export interface MonacoCompletionConfig {
  label: string;
  insertText: string;
  detail: string;
  documentation: string;
  kind?: "function" | "snippet" | "interface" | "variable";
  insertTextRule?: "plain" | "snippet";
  sortText?: string;
}

export interface MonacoHoverConfig {
  target: string;
  signature: string;
  documentation: string;
}

export interface MonacoDiagnosticMarkerConfig {
  severity: "error" | "warning" | "info";
  message: string;
  line: number;
  column: number;
  endLine: number;
  endColumn: number;
}

export interface MonacoOffsetRange {
  start: number;
  end: number;
}

type MonacoModule = typeof import("monaco-editor");
type MonacoPosition = { lineNumber: number; column: number };

export function offsetToPosition(
  text: string,
  offset: number,
): MonacoPosition {
  const clampedOffset = Math.max(0, Math.min(offset, text.length));
  const preceding = text.slice(0, clampedOffset);
  const lineNumber = (preceding.match(/\n/g)?.length ?? 0) + 1;
  const lastNewlineIndex = preceding.lastIndexOf("\n");
  const column = clampedOffset - lastNewlineIndex;
  return { lineNumber, column: Math.max(1, column) };
}

export function createCompletionRange(
  model: MonacoEditorNamespace.ITextModel,
  position: MonacoPosition,
) {
  const word = model.getWordUntilPosition(position);
  return {
    startLineNumber: position.lineNumber,
    endLineNumber: position.lineNumber,
    startColumn: word.startColumn,
    endColumn: word.endColumn,
  };
}

function createLineRange(
  lineNumber: number,
  startColumn: number,
  endColumn: number,
) {
  return {
    startLineNumber: lineNumber,
    endLineNumber: lineNumber,
    startColumn,
    endColumn,
  };
}

function isHoverExpressionCharacter(character: string): boolean {
  return character === "." || /[A-Za-z0-9_$]/.test(character);
}

function readHoverExpressionMatch(
  model: MonacoEditorNamespace.ITextModel,
  position: MonacoPosition,
) {
  const lineContent = model.getLineContent(position.lineNumber);
  if (lineContent.length === 0) return null;

  let anchorIndex = Math.min(
    Math.max(position.column - 1, 0),
    lineContent.length - 1,
  );
  if (!isHoverExpressionCharacter(lineContent[anchorIndex] ?? "")) {
    if (
      anchorIndex > 0 &&
      isHoverExpressionCharacter(lineContent[anchorIndex - 1] ?? "")
    ) {
      anchorIndex -= 1;
    } else {
      return null;
    }
  }

  let startIndex = anchorIndex;
  while (
    startIndex > 0 &&
    isHoverExpressionCharacter(lineContent[startIndex - 1] ?? "")
  ) {
    startIndex -= 1;
  }
  let endIndex = anchorIndex + 1;
  while (
    endIndex < lineContent.length &&
    isHoverExpressionCharacter(lineContent[endIndex] ?? "")
  ) {
    endIndex += 1;
  }

  const expression = lineContent
    .slice(startIndex, endIndex)
    .replace(/^\.+|\.+$/g, "");
  const segments = expression.split(".").filter(Boolean);
  if (segments.length === 0) return null;

  const relativeIndex = anchorIndex - startIndex;
  let cursor = 0;
  for (let index = 0; index < segments.length; index += 1) {
    const segment = segments[index]!;
    const segmentStart = cursor;
    const segmentEnd = cursor + segment.length - 1;
    if (relativeIndex >= segmentStart && relativeIndex <= segmentEnd) {
      const segmentPath = segments.slice(0, index + 1).join(".");
      return {
        expression,
        expressionRange: createLineRange(
          position.lineNumber,
          startIndex + 1,
          startIndex + expression.length + 1,
        ),
        segmentPath,
        segmentPathRange: createLineRange(
          position.lineNumber,
          startIndex + 1,
          startIndex + segmentPath.length + 1,
        ),
        hoveredWord: segment,
        hoveredWordRange: createLineRange(
          position.lineNumber,
          startIndex + segmentStart + 1,
          startIndex + segmentEnd + 2,
        ),
      };
    }
    cursor = segmentEnd + 2;
  }
  return null;
}

export function resolveHoverMatch(
  model: MonacoEditorNamespace.ITextModel,
  position: MonacoPosition,
  hoverItems: MonacoHoverConfig[],
) {
  const match = readHoverExpressionMatch(model, position);
  if (match === null) return null;

  const candidates = [
    { target: match.expression, range: match.expressionRange },
    { target: match.segmentPath, range: match.segmentPathRange },
    { target: match.hoveredWord, range: match.hoveredWordRange },
  ];
  for (const candidate of candidates) {
    const item = hoverItems.find(({ target }) => target === candidate.target);
    if (item !== undefined) return { item, range: candidate.range };
  }
  return null;
}

export function buildContextAwareSuggestions(
  monaco: MonacoModule,
  model: MonacoEditorNamespace.ITextModel,
  position: MonacoPosition,
) {
  const linePrefix = model.getValueInRange({
    startLineNumber: position.lineNumber,
    startColumn: 1,
    endLineNumber: position.lineNumber,
    endColumn: position.column,
  });
  const range = createCompletionRange(model, position);
  const properties = linePrefix.endsWith("ctx.kline.")
    ? [
        ["open", "number", "当前 K 线开盘价"],
        ["high", "number", "当前 K 线最高价"],
        ["low", "number", "当前 K 线最低价"],
        ["close", "number", "当前 K 线收盘价"],
        ["volume", "number", "当前 K 线成交量"],
        ["quoteVolume", "number", "当前 K 线成交额"],
        ["interval", "string", "当前 K 线周期"],
        ["symbol", "string", "当前 K 线标的代码"],
        ["startTime", "string", "当前 K 线开始时间"],
        ["endTime", "string", "当前 K 线结束时间"],
        ["closed", "boolean", "当前 K 线是否已收盘"],
      ]
    : linePrefix.endsWith("ctx.")
      ? [
          ["id", "string", "当前策略运行时 ID"],
          ["name", "string", "当前策略名称，仅 onInit 一定提供"],
          ["definitionId", "string", "当前策略定义 ID"],
          ["symbol", "string", "当前策略绑定的标的代码"],
          ["interval", "string", "当前策略绑定的周期"],
          ["kline", "JFTradeKLine", "K 线收盘上下文里的行情对象"],
        ]
      : [];

  return properties.map(([label, detail, documentation]) => ({
    label,
    kind: monaco.languages.CompletionItemKind.Property,
    insertText: label,
    detail,
    documentation: { value: documentation },
    range,
    sortText: `00-${label}`,
  }));
}

export function ensurePineV6Language(monaco: MonacoModule): void {
  const languageId = "pine-v6";
  if (!monaco.languages.getLanguages().some(({ id }) => id === languageId)) {
    monaco.languages.register({ id: languageId });
  }
  monaco.languages.setLanguageConfiguration(languageId, {
    comments: { lineComment: "//" },
    brackets: [["(", ")"]],
    autoClosingPairs: [
      { open: '"', close: '"' },
      { open: "'", close: "'" },
      { open: "(", close: ")" },
    ],
    surroundingPairs: [
      { open: '"', close: '"' },
      { open: "'", close: "'" },
      { open: "(", close: ")" },
    ],
    indentationRules: {
      increaseIndentPattern: /^\s*(if\s+.+|else)\s*(?:\/\/.*)?$/,
      decreaseIndentPattern: /^\s*else\s*(?:\/\/.*)?$/,
    },
  });
  monaco.languages.setMonarchTokensProvider(languageId, {
    defaultToken: "",
    tokenPostfix: ".pine",
    keywords: [
      "strategy", "indicator", "library", "var", "varip", "const", "type",
      "method", "if", "else", "switch", "for", "to", "by", "while",
      "break", "continue", "and", "or", "not", "true", "false", "na",
    ],
    functions: [
      "ta.ema", "ta.sma", "ta.rsi", "ta.macd", "ta.crossover", "ta.crossunder",
      "ta.cross", "ta.atr", "ta.cci", "ta.bb", "ta.supertrend", "ta.stoch",
      "ta.vwap", "strategy.entry", "strategy.order", "strategy.close",
      "strategy.close_all", "strategy.exit", "strategy.cancel",
      "strategy.cancel_all", "log.info", "alert", "alertcondition", "plot",
      "plotshape", "plotchar", "plotarrow", "plotbar", "plotcandle", "hline",
      "fill", "bgcolor", "barcolor", "label.new", "line.new", "box.new",
      "table.new", "table.cell", "math.abs", "math.min", "math.max",
      "array.new_float", "array.from", "map.new", "matrix.new", "nz",
      "request.security",
    ],
    tokenizer: {
      root: [
        [/\/\/.*$/, "comment"],
        [/"([^"\\]|\\.)*$/, "string.invalid"],
        [/"([^"\\]|\\.)*"/, "string"],
        [/'([^'\\]|\\.)*'/, "string"],
        [/\b\d+(?:\.\d+)?%?\b/, "number"],
        [/[()[\]]/, "delimiter.parenthesis"],
        [/[?:]/, "operator"],
        [/[<>!:]=?|[-+*/]/, "operator"],
        [/[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?/, {
          cases: {
            "@keywords": "keyword",
            "@functions": "type.identifier",
            "@default": "identifier",
          },
        }],
      ],
    },
  });
}

export function shouldUseMonacoFallback(): boolean {
  if (typeof window === "undefined" || typeof document === "undefined") {
    return true;
  }
  return typeof navigator !== "undefined" &&
    navigator.userAgent.toLowerCase().includes("jsdom");
}
