<script setup lang="ts">
import type {
  IDisposable,
  editor as MonacoEditorNamespace,
  languages as MonacoLanguages,
} from "monaco-editor";

import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import { useTheme } from "../composables/useTheme";

interface MonacoExtraLibConfig {
  filePath: string;
  content: string;
}

interface MonacoCompletionConfig {
  label: string;
  insertText: string;
  detail: string;
  documentation: string;
  kind?: "function" | "snippet" | "interface" | "variable";
  insertTextRule?: "plain" | "snippet";
  sortText?: string;
}

interface MonacoHoverConfig {
  target: string;
  signature: string;
  documentation: string;
}

interface Props {
  modelValue: string;
  language?: string;
  height?: number | string;
  minHeight?: number | string;
  placeholder?: string;
  testId?: string;
  resizable?: boolean;
  extraLibs?: MonacoExtraLibConfig[];
  completionItems?: MonacoCompletionConfig[];
  hoverItems?: MonacoHoverConfig[];
}

const props = withDefaults(defineProps<Props>(), {
  language: "javascript",
  height: "360px",
  minHeight: "220px",
  placeholder: "",
  testId: "",
  resizable: false,
  extraLibs: () => [],
  completionItems: () => [],
  hoverItems: () => [],
});

const emit = defineEmits<{
  "update:modelValue": [value: string];
  blur: [];
  "cursor-offset": [offset: number];
}>();

type MonacoModule = typeof import("monaco-editor");

const { theme } = useTheme();
const containerRef = ref<HTMLDivElement | null>(null);
const isFallbackMode = ref(shouldUseFallback());

let monaco: MonacoModule | null = null;
let editor: MonacoEditorNamespace.IStandaloneCodeEditor | null = null;
let skipEditorChange = false;

interface MonacoOffsetRange {
  start: number;
  end: number;
}

function offsetToPosition(
  text: string,
  offset: number,
): { lineNumber: number; column: number } {
  const clampedOffset = Math.max(0, Math.min(offset, text.length));
  const preceding = text.slice(0, clampedOffset);
  const lineNumber = (preceding.match(/\n/g)?.length ?? 0) + 1;
  const lastNewlineIndex = preceding.lastIndexOf("\n");
  const column = clampedOffset - lastNewlineIndex;
  return { lineNumber, column: Math.max(1, column) };
}

function revealOffsetRange(range: MonacoOffsetRange): void {
  if (editor === null) {
    return;
  }

  const text = editor.getValue();
  const startPosition = offsetToPosition(text, range.start);
  const endPosition = offsetToPosition(text, range.end);

  editor.setSelection({
    startLineNumber: startPosition.lineNumber,
    startColumn: startPosition.column,
    endLineNumber: endPosition.lineNumber,
    endColumn: endPosition.column,
  });

  editor.revealRangeInCenter({
    startLineNumber: startPosition.lineNumber,
    startColumn: startPosition.column,
    endLineNumber: endPosition.lineNumber,
    endColumn: endPosition.column,
  });
}

defineExpose({
  revealOffsetRange,
});
let modelChangeSubscription: IDisposable | null = null;
let editorBlurSubscription: IDisposable | null = null;
let cursorPositionSubscription: IDisposable | null = null;
let completionProviderDisposable: IDisposable | null = null;
let hoverProviderDisposable: IDisposable | null = null;
let extraLibDisposables: IDisposable[] = [];
let isUnmounted = false;
let initializationGeneration = 0;

const editorHeight = computed(() =>
  typeof props.height === "number" ? `${props.height}px` : props.height,
);

const editorMinHeight = computed(() =>
  typeof props.minHeight === "number" ? `${props.minHeight}px` : props.minHeight,
);

watch(
  () => props.modelValue,
  (nextValue) => {
    if (isFallbackMode.value || editor === null) {
      return;
    }
    const model = editor.getModel();
    if (model === null || model.getValue() === nextValue) {
      return;
    }
    skipEditorChange = true;
    model.setValue(nextValue);
    skipEditorChange = false;
  },
);

watch(
  () => props.language,
  (nextLanguage) => {
    if (isFallbackMode.value || editor === null || monaco === null) {
      return;
    }
    const model = editor.getModel();
    if (model === null) {
      return;
    }
    monaco.editor.setModelLanguage(model, nextLanguage);
  },
);

watch(theme, (nextTheme) => {
  if (isFallbackMode.value || monaco === null) {
    return;
  }
  monaco.editor.setTheme(nextTheme === "light" ? "vs" : "vs-dark");
});

onMounted(() => {
  isUnmounted = false;
  if (isFallbackMode.value) {
    return;
  }
  void initializeMonaco();
});

onBeforeUnmount(() => {
  isUnmounted = true;
  initializationGeneration += 1;
  disposeMonacoInstance();
});

function canMountEditor(target: HTMLDivElement | null): target is HTMLDivElement {
  return target !== null && target.isConnected && target.parentElement !== null;
}

function shouldAbortInitialization(
  generation: number,
  target: HTMLDivElement | null,
): boolean {
  return isUnmounted || generation !== initializationGeneration || !canMountEditor(target) || target !== containerRef.value;
}

function disposePendingRegistrations(
  nextCompletionProviderDisposable: IDisposable | null,
  nextHoverProviderDisposable: IDisposable | null,
  nextExtraLibDisposables: IDisposable[],
): void {
  nextCompletionProviderDisposable?.dispose();
  nextHoverProviderDisposable?.dispose();
  for (const disposable of nextExtraLibDisposables) {
    disposable.dispose();
  }
}

function disposeMonacoInstance(): void {
  modelChangeSubscription?.dispose();
  modelChangeSubscription = null;
  editorBlurSubscription?.dispose();
  editorBlurSubscription = null;
  cursorPositionSubscription?.dispose();
  cursorPositionSubscription = null;
  completionProviderDisposable?.dispose();
  completionProviderDisposable = null;
  hoverProviderDisposable?.dispose();
  hoverProviderDisposable = null;
  for (const disposable of extraLibDisposables) {
    disposable.dispose();
  }
  extraLibDisposables = [];
  editor?.dispose();
  editor = null;
  monaco = null;
}

function resolveCompletionKind(
  completionKind: MonacoCompletionConfig["kind"],
) {
  switch (completionKind) {
    case "function":
      return monaco!.languages.CompletionItemKind.Function;
    case "interface":
      return monaco!.languages.CompletionItemKind.Interface;
    case "variable":
      return monaco!.languages.CompletionItemKind.Variable;
    case "snippet":
    default:
      return monaco!.languages.CompletionItemKind.Snippet;
  }
}


function createCompletionRange(
  model: MonacoEditorNamespace.ITextModel,
  position: { lineNumber: number; column: number },
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

function isHoverIdentifierCharacter(character: string): boolean {
  return /[A-Za-z0-9_$]/.test(character);
}

function isHoverExpressionCharacter(character: string): boolean {
  return character === "." || isHoverIdentifierCharacter(character);
}

function readHoverExpressionMatch(
  model: MonacoEditorNamespace.ITextModel,
  position: { lineNumber: number; column: number },
) {
  const lineContent = model.getLineContent(position.lineNumber);
  if (lineContent.length === 0) {
    return null;
  }

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

  if (expression === "") {
    return null;
  }

  const segments = expression.split(".").filter((segment) => segment.length > 0);
  if (segments.length === 0) {
    return null;
  }

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

function resolveHoverMatch(
  model: MonacoEditorNamespace.ITextModel,
  position: { lineNumber: number; column: number },
  hoverItems: MonacoHoverConfig[],
) {
  const expressionMatch = readHoverExpressionMatch(model, position);
  if (expressionMatch === null) {
    return null;
  }

  const candidates = [
    {
      target: expressionMatch.expression,
      range: expressionMatch.expressionRange,
    },
    {
      target: expressionMatch.segmentPath,
      range: expressionMatch.segmentPathRange,
    },
    {
      target: expressionMatch.hoveredWord,
      range: expressionMatch.hoveredWordRange,
    },
  ];

  for (const candidate of candidates) {
    const item = hoverItems.find(
      (hoverItem) => hoverItem.target === candidate.target,
    );
    if (item !== undefined) {
      return {
        item,
        range: candidate.range,
      };
    }
  }

  return null;
}

function buildContextAwareSuggestions(
  model: MonacoEditorNamespace.ITextModel,
  position: { lineNumber: number; column: number },
) {
  const linePrefix = model.getValueInRange({
    startLineNumber: position.lineNumber,
    startColumn: 1,
    endLineNumber: position.lineNumber,
    endColumn: position.column,
  });
  const range = createCompletionRange(model, position);

  if (linePrefix.endsWith("ctx.kline.")) {
    return [
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
    ].map(([label, detail, documentation]) => ({
      label,
      kind: monaco!.languages.CompletionItemKind.Property,
      insertText: label,
      detail,
      documentation: { value: documentation },
      range,
      sortText: `00-${label}`,
    }));
  }

  if (linePrefix.endsWith("ctx.")) {
    return [
      ["id", "string", "当前策略运行时 ID"],
      ["name", "string", "当前策略名称，仅 onInit 一定提供"],
      ["definitionId", "string", "当前策略定义 ID"],
      ["symbol", "string", "当前策略绑定的标的代码"],
      ["interval", "string", "当前策略绑定的周期"],
      ["kline", "JFTradeKLine", "K 线收盘上下文里的行情对象"],
    ].map(([label, detail, documentation]) => ({
      label,
      kind: monaco!.languages.CompletionItemKind.Property,
      insertText: label,
      detail,
      documentation: { value: documentation },
      range,
      sortText: `00-${label}`,
    }));
  }

  return [];
}
function shouldUseFallback(): boolean {
  if (typeof window === "undefined" || typeof document === "undefined") {
    return true;
  }
  return typeof navigator !== "undefined" && navigator.userAgent.toLowerCase().includes("jsdom");
}

function handleFallbackInput(event: Event): void {
  emit(
    "update:modelValue",
    (event.target as HTMLTextAreaElement | null)?.value ?? "",
  );
}

function handleFallbackBlur(): void {
  emit("blur");
}

function getMonacoTheme(): "vs" | "vs-dark" {
  return theme.value === "light" ? "vs" : "vs-dark";
}

function ensureJftradeDslLanguage(monacoInstance: MonacoModule): void {
  const languageId = "jftrade-dsl";
  if (!monacoInstance.languages.getLanguages().some((language) => language.id === languageId)) {
    monacoInstance.languages.register({ id: languageId });
  }

  monacoInstance.languages.setLanguageConfiguration(languageId, {
    comments: { lineComment: "#" },
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
      increaseIndentPattern: /^\s*(on\s+(?:init|kline_close)|if\s+.+|else)\s*:\s*(?:#.*)?$/,
      decreaseIndentPattern: /^\s*else\s*:\s*(?:#.*)?$/,
    },
  });

  monacoInstance.languages.setMonarchTokensProvider(languageId, {
    defaultToken: "",
    tokenPostfix: ".dsl",
    keywords: [
      "strategy",
      "version",
      "symbol",
      "interval",
      "on",
      "init",
      "kline_close",
      "let",
      "if",
      "else",
      "log",
      "notify",
      "buy",
      "sell",
      "short",
      "cover",
      "protect",
      "policy",
      "limit",
      "type",
      "window",
      "and",
      "or",
      "not",
    ],
    functions: [
      "ma",
      "rsi",
      "macd",
      "kdj",
      "bollinger",
      "atr",
      "cci",
      "williams_r",
      "williamsr",
      "cross_over",
      "cross_under",
      "divergence_top",
      "divergence_bottom",
      "abs",
    ],
    tokenizer: {
      root: [
        [/#.*$/, "comment"],
        [/"([^"\\]|\\.)*$/, "string.invalid"],
        [/"([^"\\]|\\.)*"/, "string"],
        [/'([^'\\]|\\.)*'/, "string"],
        [/\b\d+(?:\.\d+)?%?\b/, "number"],
        [/[()]/, "delimiter.parenthesis"],
        [/[<>!=]=?|[-+*/]/, "operator"],
        [/[A-Za-z_][A-Za-z0-9_]*/, {
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

async function initializeMonaco(): Promise<void> {
  const target = containerRef.value;
  if (!canMountEditor(target) || editor !== null) {
    return;
  }

  const generation = ++initializationGeneration;

  try {
    const editorWorkerModule = await import(
      "monaco-editor/esm/vs/editor/editor.worker?worker"
    );
    const typescriptWorkerModule = await import(
      "monaco-editor/esm/vs/language/typescript/ts.worker?worker"
    );
    const monacoModule = await import("monaco-editor");

    if (shouldAbortInitialization(generation, target)) {
      return;
    }

    const EditorWorker = editorWorkerModule.default;
    const TypeScriptWorker = typescriptWorkerModule.default;
    const nextMonaco = monacoModule;

    (
      globalThis as typeof globalThis & {
        MonacoEnvironment?: {
          getWorker: (_moduleId: string, label: string) => Worker;
        };
      }
    ).MonacoEnvironment = {
      getWorker: (_moduleId, label) => {
        if (label === "javascript" || label === "typescript") {
          return new TypeScriptWorker();
        }
        return new EditorWorker();
      },
    };

    monaco = nextMonaco;
  ensureJftradeDslLanguage(monaco);
    monaco.typescript.javascriptDefaults.setEagerModelSync(true);
    monaco.typescript.javascriptDefaults.setDiagnosticsOptions({
      noSemanticValidation: false,
      noSyntaxValidation: false,
    });
    monaco.typescript.javascriptDefaults.setCompilerOptions({
      allowNonTsExtensions: true,
      allowJs: true,
      checkJs: true,
      target: monaco.typescript.ScriptTarget.ES2020,
      module: monaco.typescript.ModuleKind.ESNext,
    });

    const nextExtraLibDisposables = props.extraLibs.map((extraLib) =>
      monaco!.typescript.javascriptDefaults.addExtraLib(
        extraLib.content,
        extraLib.filePath,
      ),
    );

    let nextCompletionProviderDisposable: IDisposable | null = null;
    let nextHoverProviderDisposable: IDisposable | null = null;

    if (props.completionItems.length > 0) {
      const completionItems: MonacoCompletionConfig[] = props.completionItems;
      nextCompletionProviderDisposable = monaco.languages.registerCompletionItemProvider(
        props.language,
        {
          triggerCharacters: [".", "@"],
          provideCompletionItems: (model, position) => {
            const range = createCompletionRange(model, position);
            const suggestions = [
              ...buildContextAwareSuggestions(model, position),
              ...completionItems.map((completionItem) => ({
                label: completionItem.label ?? "",
                kind: resolveCompletionKind(completionItem.kind),
                insertText: completionItem.insertText ?? "",
                detail: completionItem.detail ?? "",
                documentation: {
                  value: completionItem.documentation ?? "",
                },
                sortText: completionItem.sortText ?? completionItem.label ?? "",
                range,
                insertTextRules:
                  completionItem.insertTextRule === "snippet"
                    ? monaco!.languages.CompletionItemInsertTextRule.InsertAsSnippet
                    : undefined,
              })),
            ] as MonacoLanguages.CompletionItem[];
            return {
              suggestions,
            };
          },
        },
      );
    }

    if (props.hoverItems.length > 0) {
      const hoverItems: MonacoHoverConfig[] = props.hoverItems;
      nextHoverProviderDisposable = monaco.languages.registerHoverProvider(
        props.language,
        {
          provideHover: (model, position) => {
            const hoverMatch = resolveHoverMatch(model, position, hoverItems);
            if (hoverMatch === null) {
              return null;
            }

            return {
              range: hoverMatch.range,
              contents: [
                { value: `**${hoverMatch.item.target}**` },
                {
                  value: [
                    "```ts",
                    hoverMatch.item.signature,
                    "```",
                  ].join("\n"),
                },
                { value: hoverMatch.item.documentation },
              ],
            };
          },
        },
      );
    }

    if (shouldAbortInitialization(generation, target)) {
      disposePendingRegistrations(
        nextCompletionProviderDisposable,
        nextHoverProviderDisposable,
        nextExtraLibDisposables,
      );
      return;
    }

    editor = monaco.editor.create(target, {
      value: props.modelValue,
      language: props.language,
      theme: getMonacoTheme(),
      automaticLayout: true,
      minimap: { enabled: false },
      overviewRulerLanes: 0,
      scrollBeyondLastLine: false,
      quickSuggestions: true,
      suggestOnTriggerCharacters: true,
      wordWrap: "on",
      fontSize: 13,
      tabSize: 2,
      padding: {
        top: 16,
        bottom: 16,
      },
    });

    if (shouldAbortInitialization(generation, target)) {
      editor.dispose();
      editor = null;
      disposePendingRegistrations(
        nextCompletionProviderDisposable,
        nextHoverProviderDisposable,
        nextExtraLibDisposables,
      );
      return;
    }

    completionProviderDisposable = nextCompletionProviderDisposable;
    hoverProviderDisposable = nextHoverProviderDisposable;
    extraLibDisposables = nextExtraLibDisposables;

    modelChangeSubscription = editor.onDidChangeModelContent(() => {
      if (skipEditorChange || editor === null) {
        return;
      }
      const nextValue = editor.getValue();
      if (nextValue !== props.modelValue) {
        emit("update:modelValue", nextValue);
      }
    });
    editorBlurSubscription = editor.onDidBlurEditorText(() => {
      emit("blur");
    });
    cursorPositionSubscription = editor.onDidChangeCursorPosition((event) => {
      const model = editor?.getModel();
      if (model === null || model === undefined) {
        return;
      }
      const offset = model.getOffsetAt(event.position);
      emit("cursor-offset", offset);
    });
  } catch (error) {
    if (shouldAbortInitialization(generation, target)) {
      return;
    }
    console.error("failed to initialize Monaco editor", error);
    isFallbackMode.value = true;
  }
}
</script>

<template>
  <div
    class="monaco-code-editor-shell"
    :class="{ 'monaco-code-editor-shell--resizable': resizable }"
    :style="{ height: editorHeight, minHeight: editorMinHeight }"
  >
    <textarea
      v-if="isFallbackMode"
      :value="modelValue"
      :data-testid="testId || undefined"
      :placeholder="placeholder"
      class="monaco-code-editor-fallback"
      spellcheck="false"
      @blur="handleFallbackBlur"
      @input="handleFallbackInput"
    />
    <div
      v-else
      ref="containerRef"
      :data-testid="testId || undefined"
      class="monaco-code-editor-surface"
    />
  </div>
</template>

<style scoped>
.monaco-code-editor-shell {
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.32);
  border-radius: 1.1rem;
  background: rgb(2, 6, 23);
}

.monaco-code-editor-shell--resizable {
  resize: vertical;
}

.monaco-code-editor-surface {
  width: 100%;
  height: 100%;
}

.monaco-code-editor-fallback {
  width: 100%;
  height: 100%;
  min-width: 100%;
  min-height: 100%;
  border: 0;
  background: rgb(2, 6, 23);
  color: rgb(226, 232, 240);
  padding: 0.8rem 0.85rem;
  outline: none;
  resize: none;
  font-family: "SFMono-Regular", Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 0.75rem;
  line-height: 1.6;
}

:global([data-theme="light"]) .monaco-code-editor-shell {
  border-color: rgb(203, 213, 225);
  background: rgb(248, 250, 252);
}

:global([data-theme="light"]) .monaco-code-editor-fallback {
  background: rgb(248, 250, 252);
  color: rgb(15, 23, 42);
}
</style>