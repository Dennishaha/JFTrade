<script setup lang="ts">
import type {
  IDisposable,
  editor as MonacoEditorNamespace,
  languages as MonacoLanguages,
} from "monaco-editor";

import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import { useTheme } from "@/composables/settings/useTheme";
import {
  loadMonacoTypeScriptSupport,
  type MonacoTypeScriptSupport,
} from "@/monacoTypescriptSupport";
import {
  buildContextAwareSuggestions,
  createCompletionRange,
  ensurePineV6Language,
  offsetToPosition,
  resolveHoverMatch,
  shouldUseMonacoFallback,
} from "@/features/monacoEditorSupport";
import type {
  MonacoCompletionConfig,
  MonacoDiagnosticMarkerConfig,
  MonacoExtraLibConfig,
  MonacoHoverConfig,
  MonacoOffsetRange,
} from "@/features/monacoEditorSupport";

interface Props {
  modelValue: string;
  language?: string;
  height?: number | string;
  minHeight?: number | string;
  fontSize?: number;
  placeholder?: string;
  testId?: string;
  resizable?: boolean;
  readOnly?: boolean;
  extraLibs?: MonacoExtraLibConfig[];
  completionItems?: MonacoCompletionConfig[];
  hoverItems?: MonacoHoverConfig[];
  diagnosticMarkers?: MonacoDiagnosticMarkerConfig[];
}

const props = withDefaults(defineProps<Props>(), {
  language: "javascript",
  height: "360px",
  minHeight: "220px",
  fontSize: 13,
  placeholder: "",
  testId: "",
  resizable: false,
  readOnly: false,
  extraLibs: () => [],
  completionItems: () => [],
  hoverItems: () => [],
  diagnosticMarkers: () => [],
});

const emit = defineEmits<{
  "update:modelValue": [value: string];
  blur: [];
  "cursor-offset": [offset: number];
}>();

type MonacoModule = typeof import("monaco-editor");

const { theme } = useTheme();
const containerRef = ref<HTMLDivElement | null>(null);
const fallbackTextareaRef = ref<HTMLTextAreaElement | null>(null);
const isFallbackMode = ref(shouldUseMonacoFallback());

let monaco: MonacoModule | null = null;
let typescriptSupport: MonacoTypeScriptSupport | null = null;
let editor: MonacoEditorNamespace.IStandaloneCodeEditor | null = null;
let skipEditorChange = false;

function revealOffsetRange(range: MonacoOffsetRange): void {
  if (isFallbackMode.value) {
    const textarea = fallbackTextareaRef.value;
    if (textarea === null) {
      return;
    }
    const start = Math.max(0, Math.min(range.start, textarea.value.length));
    const end = Math.max(start, Math.min(range.end, textarea.value.length));
    textarea.focus();
    textarea.setSelectionRange(start, end);
    const lineHeight = Number.parseFloat(getComputedStyle(textarea).lineHeight || "16");
    const preceding = textarea.value.slice(0, start);
    const lineIndex = preceding.match(/\n/g)?.length ?? 0;
    textarea.scrollTop = Math.max(0, lineIndex * lineHeight - textarea.clientHeight / 2);
    return;
  }
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
    void updateModelLanguage(nextLanguage);
  },
);

watch(theme, (nextTheme) => {
  if (isFallbackMode.value || monaco === null) {
    return;
  }
  monaco.editor.setTheme(nextTheme === "light" ? "vs" : "vs-dark");
});

watch(
  () => props.readOnly,
  (nextReadOnly) => {
    if (isFallbackMode.value || editor === null) {
      return;
    }

    editor.updateOptions({
      readOnly: nextReadOnly,
      domReadOnly: nextReadOnly,
      contextmenu: !nextReadOnly,
      cursorStyle: nextReadOnly ? "line-thin" : "line",
      occurrencesHighlight: nextReadOnly ? "off" : "singleFile",
    });
  },
);

watch(
  () => props.fontSize,
  (nextFontSize) => {
    editor?.updateOptions({ fontSize: nextFontSize });
  },
);

watch(
  () => props.diagnosticMarkers,
  () => {
    applyDiagnosticMarkers();
  },
  { deep: true },
);

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

function resolveMarkerSeverity(severity: MonacoDiagnosticMarkerConfig["severity"]) {
  switch (severity) {
    case "error":
      return monaco!.MarkerSeverity.Error;
    case "warning":
      return monaco!.MarkerSeverity.Warning;
    case "info":
    default:
      return monaco!.MarkerSeverity.Info;
  }
}

function applyDiagnosticMarkers(): void {
  if (isFallbackMode.value || monaco === null || editor === null) {
    return;
  }
  const model = editor.getModel();
  if (model === null) {
    return;
  }
  monaco.editor.setModelMarkers(
    model,
    "jftrade-pine",
    props.diagnosticMarkers.map((marker) => ({
      severity: resolveMarkerSeverity(marker.severity),
      message: marker.message,
      startLineNumber: Math.max(1, marker.line),
      startColumn: Math.max(1, marker.column),
      endLineNumber: Math.max(1, marker.endLine || marker.line),
      endColumn: Math.max(1, marker.endColumn || marker.column + 1),
    })),
  );
}


function handleFallbackInput(event: Event): void {
  if (props.readOnly) {
    return;
  }
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

function needsTypeScriptSupport(): boolean {
  return (
    props.language === "javascript" ||
    props.language === "typescript" ||
    props.extraLibs.length > 0
  );
}

function configureEditorWorker(EditorWorker: new () => Worker): void {
  (
    globalThis as typeof globalThis & {
      MonacoEnvironment?: {
        getWorker: (_moduleId: string, label: string) => Worker;
      };
    }
  ).MonacoEnvironment = {
    getWorker: () => new EditorWorker(),
  };
}

function configureTypeScriptSupport(
  nextTypeScriptSupport: MonacoTypeScriptSupport,
): void {
  nextTypeScriptSupport.javascriptDefaults.setEagerModelSync(true);
  nextTypeScriptSupport.javascriptDefaults.setDiagnosticsOptions({
    noSemanticValidation: false,
    noSyntaxValidation: false,
  });
  nextTypeScriptSupport.javascriptDefaults.setCompilerOptions({
    allowNonTsExtensions: true,
    allowJs: true,
    checkJs: true,
    target: nextTypeScriptSupport.ScriptTarget.ES2020,
    module: nextTypeScriptSupport.ModuleKind.ESNext,
  });
}

async function ensureTypeScriptSupport(): Promise<MonacoTypeScriptSupport> {
  if (typescriptSupport !== null) {
    return typescriptSupport;
  }
  const nextTypeScriptSupport = await loadMonacoTypeScriptSupport();
  if (isUnmounted) {
    return nextTypeScriptSupport;
  }
  configureTypeScriptSupport(nextTypeScriptSupport);
  typescriptSupport = nextTypeScriptSupport;
  return nextTypeScriptSupport;
}

async function updateModelLanguage(nextLanguage: string): Promise<void> {
  if (isFallbackMode.value || editor === null || monaco === null) {
    return;
  }
  if (
    (nextLanguage === "javascript" || nextLanguage === "typescript") &&
    typescriptSupport === null
  ) {
    await ensureTypeScriptSupport();
  }
  if (isUnmounted || editor === null || monaco === null) {
    return;
  }
  const model = editor.getModel();
  if (model === null) {
    return;
  }
  monaco.editor.setModelLanguage(model, nextLanguage);
}

async function initializeMonaco(): Promise<void> {
  const target = containerRef.value;
  if (!canMountEditor(target) || editor !== null) {
    return;
  }

  const generation = ++initializationGeneration;

  try {
    const [editorWorkerModule, monacoModule, nextTypeScriptSupport] =
      await Promise.all([
        import("monaco-editor/editor/editor.worker?worker"),
        import("monaco-editor"),
        needsTypeScriptSupport()
          ? loadMonacoTypeScriptSupport()
          : Promise.resolve(null),
      ]);

    if (shouldAbortInitialization(generation, target)) {
      return;
    }

    const EditorWorker = editorWorkerModule.default;
    const nextMonaco = monacoModule;

    configureEditorWorker(EditorWorker);
    if (nextTypeScriptSupport !== null) {
      configureTypeScriptSupport(nextTypeScriptSupport);
      typescriptSupport = nextTypeScriptSupport;
      // Monaco 0.56 的 TypeScript workerManager 会在首次使用时通过
      // ts.worker.js 自己创建 worker；这里仍保留统一的 editor worker
      // 环境，避免 standaloneWebWorkerService 丢失 editor worker 工厂。
    }

    monaco = nextMonaco;
    ensurePineV6Language(monaco);

    const nextExtraLibDisposables =
      nextTypeScriptSupport?.javascriptDefaults === undefined
        ? []
        : props.extraLibs.map((extraLib) =>
            nextTypeScriptSupport.javascriptDefaults.addExtraLib(
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
              ...buildContextAwareSuggestions(monaco!, model, position),
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
      readOnly: props.readOnly,
      domReadOnly: props.readOnly,
      contextmenu: !props.readOnly,
      cursorStyle: props.readOnly ? "line-thin" : "line",
      occurrencesHighlight: props.readOnly ? "off" : "singleFile",
      minimap: { enabled: false },
      overviewRulerLanes: 0,
      scrollBeyondLastLine: false,
      quickSuggestions: true,
      suggestOnTriggerCharacters: true,
      wordWrap: "on",
      fontSize: props.fontSize,
      tabSize: 2,
      padding: {
        top: 16,
        bottom: 16,
      },
    });
    applyDiagnosticMarkers();

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
      ref="fallbackTextareaRef"
      :value="modelValue"
      :data-testid="testId || undefined"
      :placeholder="placeholder"
      :readonly="readOnly"
      :style="{ fontSize: `${fontSize}px` }"
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
