<script setup lang="ts">
import type { editor as MonacoEditorNamespace } from "monaco-editor";

import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import { useTheme } from "../composables/useTheme";

const props = withDefaults(defineProps<{
  leftLabel: string;
  rightLabel: string;
  leftSource: string;
  rightSource: string;
  language?: string;
  height?: number | string;
}>(), {
  language: "pine-v6",
  height: 520,
});

type MonacoModule = typeof import("monaco-editor");

const { theme } = useTheme();
const hostRef = ref<HTMLDivElement | null>(null);
const fallbackMode = ref(shouldUseFallback());

let monaco: MonacoModule | null = null;
let diffEditor: MonacoEditorNamespace.IStandaloneDiffEditor | null = null;
let originalModel: MonacoEditorNamespace.ITextModel | null = null;
let modifiedModel: MonacoEditorNamespace.ITextModel | null = null;
let initializationGeneration = 0;
let unmounted = false;
let narrowViewportQuery: MediaQueryList | null = null;

const editorHeight = computed(() =>
  typeof props.height === "number" ? `${props.height}px` : props.height,
);

function shouldUseFallback(): boolean {
  if (typeof window === "undefined" || typeof document === "undefined") {
    return true;
  }
  return typeof navigator !== "undefined" && navigator.userAgent.toLowerCase().includes("jsdom");
}

function narrowViewport(): boolean {
  return narrowViewportQuery?.matches ?? false;
}

function updateDiffLayoutForViewport(): void {
  diffEditor?.updateOptions({
    renderSideBySide: !narrowViewport(),
  });
  diffEditor?.layout();
}

function ensureLanguage(monacoInstance: MonacoModule, language: string): void {
  if (monacoInstance.languages.getLanguages().some((entry) => entry.id === language)) {
    return;
  }
  monacoInstance.languages.register({ id: language });
  monacoInstance.languages.setLanguageConfiguration(language, {
    comments: { lineComment: "//" },
    brackets: [["(", ")"]],
  });
  monacoInstance.languages.setMonarchTokensProvider(language, {
    tokenizer: {
      root: [
        [/\/\/.*$/, "comment"],
        [/"(?:[^"\\]|\\.)*"/, "string"],
        [/\b(?:strategy|indicator|if|else|for|while|var|const|true|false)\b/, "keyword"],
        [/\b\d+(?:\.\d+)?\b/, "number"],
      ],
    },
  });
}

function disposeEditor(): void {
  diffEditor?.dispose();
  diffEditor = null;
  originalModel?.dispose();
  originalModel = null;
  modifiedModel?.dispose();
  modifiedModel = null;
  monaco = null;
}

function shouldAbort(generation: number, host: HTMLDivElement | null): boolean {
  return unmounted || generation !== initializationGeneration || host == null || !host.isConnected || host !== hostRef.value;
}

async function mountEditor(): Promise<void> {
  const generation = ++initializationGeneration;
  const host = hostRef.value;
  if (host == null || !host.isConnected) {
    return;
  }
  try {
    const editorWorkerModule = await import(
      "monaco-editor/editor/editor.worker?worker"
    );
    const typescriptWorkerModule = await import(
      "monaco-editor/language/typescript/ts.worker?worker"
    );
    const module = await import("monaco-editor");
    if (shouldAbort(generation, host)) {
      return;
    }
    const EditorWorker = editorWorkerModule.default;
    const TypeScriptWorker = typescriptWorkerModule.default;
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
    monaco = module;
    ensureLanguage(module, props.language);
    originalModel = module.editor.createModel(props.leftSource, props.language);
    modifiedModel = module.editor.createModel(props.rightSource, props.language);
    diffEditor = module.editor.createDiffEditor(host, {
      automaticLayout: true,
      readOnly: true,
      domReadOnly: true,
      originalEditable: false,
      enableSplitViewResizing: true,
      renderSideBySide: !narrowViewport(),
      renderOverviewRuler: false,
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      wordWrap: "on",
      lineNumbers: "on",
      fontSize: 13,
      theme: theme.value === "light" ? "vs" : "vs-dark",
    });
    diffEditor.setModel({ original: originalModel, modified: modifiedModel });
  } catch {
    disposeEditor();
    fallbackMode.value = true;
  }
}

function syncModels(): void {
  if (originalModel != null && originalModel.getValue() !== props.leftSource) {
    originalModel.setValue(props.leftSource);
  }
  if (modifiedModel != null && modifiedModel.getValue() !== props.rightSource) {
    modifiedModel.setValue(props.rightSource);
  }
}

watch(
  () => [props.leftSource, props.rightSource] as const,
  () => syncModels(),
);

watch(
  () => props.language,
  (nextLanguage) => {
    if (monaco == null || originalModel == null || modifiedModel == null) {
      return;
    }
    ensureLanguage(monaco, nextLanguage);
    monaco.editor.setModelLanguage(originalModel, nextLanguage);
    monaco.editor.setModelLanguage(modifiedModel, nextLanguage);
  },
);

watch(theme, (nextTheme) => {
  if (monaco != null) {
    monaco.editor.setTheme(nextTheme === "light" ? "vs" : "vs-dark");
  }
});

onMounted(() => {
  unmounted = false;
  if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
    narrowViewportQuery = window.matchMedia("(max-width: 760px)");
    narrowViewportQuery.addEventListener("change", updateDiffLayoutForViewport);
  }
  if (!fallbackMode.value) {
    void mountEditor();
  }
});

onBeforeUnmount(() => {
  unmounted = true;
  initializationGeneration += 1;
  narrowViewportQuery?.removeEventListener("change", updateDiffLayoutForViewport);
  narrowViewportQuery = null;
  disposeEditor();
});
</script>

<template>
  <section class="strategy-source-diff" data-testid="strategy-source-diff">
    <div v-if="fallbackMode" class="strategy-source-diff__fallback">
      <label>
        <span>{{ leftLabel }}</span>
        <textarea
          :value="leftSource"
          aria-label="基线策略源码"
          data-testid="strategy-source-diff-left-fallback"
          readonly
        />
      </label>
      <label>
        <span>{{ rightLabel }}</span>
        <textarea
          :value="rightSource"
          aria-label="候选策略源码"
          data-testid="strategy-source-diff-right-fallback"
          readonly
        />
      </label>
    </div>
    <div
      v-else
      ref="hostRef"
      class="strategy-source-diff__editor"
      :style="{ height: editorHeight }"
      aria-label="策略源码差异"
    />
  </section>
</template>

<style scoped>
.strategy-source-diff {
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 0.5rem;
  overflow: hidden;
  background: var(--tv-bg-surface);
}

.strategy-source-diff__editor {
  min-width: 0;
  min-height: 16rem;
}

.strategy-source-diff__fallback {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  background: var(--tv-border);
}

.strategy-source-diff__fallback label {
  display: grid;
  gap: 0.45rem;
  min-width: 0;
  background: var(--tv-bg-surface);
  padding: 0.75rem;
  color: var(--tv-text-muted);
  font-size: 0.75rem;
  font-weight: 700;
}

.strategy-source-diff__fallback textarea {
  min-width: 0;
  min-height: 30rem;
  resize: vertical;
  border: 1px solid var(--tv-border);
  border-radius: 0.35rem;
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
  padding: 0.65rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.78rem;
  line-height: 1.5;
  white-space: pre;
}

@media (max-width: 760px) {
  .strategy-source-diff__fallback {
    grid-template-columns: minmax(0, 1fr);
  }

  .strategy-source-diff__fallback textarea {
    min-height: 18rem;
  }
}
</style>
