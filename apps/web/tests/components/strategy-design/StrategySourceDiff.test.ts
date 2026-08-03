// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import StrategySourceDiff from "@/components/strategy-design/StrategySourceDiff.vue";
import { provideThemeStore, type ThemeStore } from "@/composables/settings/useTheme";

const monacoMocks = vi.hoisted(() => {
  const editorDispose = vi.fn();
  const layout = vi.fn();
  const setModel = vi.fn();
  const updateOptions = vi.fn();
  const diffEditor = { dispose: editorDispose, layout, setModel, updateOptions };
  const models: Array<{
    dispose: ReturnType<typeof vi.fn>;
    getValue: ReturnType<typeof vi.fn>;
    setValue: ReturnType<typeof vi.fn>;
  }> = [];
  let languages: Array<{ id: string }> = [];
  const createDiffEditor = vi.fn(() => diffEditor);
  const createModel = vi.fn((value: string) => {
    let current = value;
    const model = {
      dispose: vi.fn(),
      getValue: vi.fn(() => current),
      setValue: vi.fn((next: string) => {
        current = next;
      }),
    };
    models.push(model);
    return model;
  });
  return {
    createDiffEditor,
    createModel,
    diffEditor,
    editorDispose,
    getLanguages: () => languages,
    layout,
    models,
    register: vi.fn(),
    reset() {
      languages = [];
      models.splice(0);
    },
    setLanguageConfiguration: vi.fn(),
    setLanguages(next: Array<{ id: string }>) {
      languages = next;
    },
    setModel,
    setModelLanguage: vi.fn(),
    setMonarchTokensProvider: vi.fn(),
    setTheme: vi.fn(),
    updateOptions,
  };
});

vi.mock("monaco-editor/editor/editor.worker?worker", () => ({
  default: class EditorWorker {},
}));

vi.mock("monaco-editor/language/typescript/ts.worker?worker", () => ({
  default: class TypeScriptWorker {},
}));

vi.mock("monaco-editor", () => ({
  editor: {
    createDiffEditor: monacoMocks.createDiffEditor,
    createModel: monacoMocks.createModel,
    setModelLanguage: monacoMocks.setModelLanguage,
    setTheme: monacoMocks.setTheme,
  },
  languages: {
    getLanguages: monacoMocks.getLanguages,
    register: monacoMocks.register,
    setLanguageConfiguration: monacoMocks.setLanguageConfiguration,
    setMonarchTokensProvider: monacoMocks.setMonarchTokensProvider,
  },
}));

beforeEach(() => {
  monacoMocks.reset();
  for (const value of Object.values(monacoMocks)) {
    if (typeof value === "function" && "mockClear" in value) {
      (value as ReturnType<typeof vi.fn>).mockClear();
    }
  }
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  document.body.innerHTML = "";
  window.localStorage.clear();
});

function mountDiff(options: { browser?: boolean; matches?: boolean; height?: number | string } = {}) {
  const listener = { current: null as ((event: MediaQueryListEvent) => void) | null };
  const removeEventListener = vi.fn();
  if (options.browser) {
    vi.stubGlobal("navigator", { userAgent: "Mozilla/5.0 Chrome/126" });
    vi.stubGlobal("matchMedia", vi.fn(() => ({
      matches: options.matches ?? false,
      media: "(max-width: 760px)",
      onchange: null,
      addEventListener: vi.fn((_type: string, next: (event: MediaQueryListEvent) => void) => {
        listener.current = next;
      }),
      removeEventListener,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    } as unknown as MediaQueryList)));
  }
  const leftSource = ref("strategy('left')");
  const rightSource = ref("strategy('right')");
  const language = ref("pine-v6");
  let themeStore: ThemeStore | null = null;
  const Host = defineComponent({
    components: { StrategySourceDiff },
    setup() {
      themeStore = provideThemeStore();
      return { height: options.height ?? 320, language, leftSource, rightSource };
    },
    template: `
      <StrategySourceDiff
        left-label="Baseline"
        right-label="Candidate"
        :left-source="leftSource"
        :right-source="rightSource"
        :language="language"
        :height="height"
      />
    `,
  });
  const wrapper = mount(Host, { attachTo: document.body });
  return { language, leftSource, listener, removeEventListener, rightSource, themeStore: () => themeStore!, wrapper };
}

describe("StrategySourceDiff", () => {
  it("renders immutable source snapshots in the jsdom fallback", async () => {
    const { leftSource, rightSource, wrapper } = mountDiff({ height: "40vh" });
    expect(wrapper.get('[data-testid="strategy-source-diff-left-fallback"]').element.value).toBe("strategy('left')");
    expect(wrapper.get('[data-testid="strategy-source-diff-right-fallback"]').element.value).toBe("strategy('right')");
    expect(wrapper.text()).toContain("Baseline");
    expect(wrapper.text()).toContain("Candidate");

    leftSource.value = "strategy('left next')";
    rightSource.value = "strategy('right next')";
    await nextTick();
    expect(wrapper.get('[data-testid="strategy-source-diff-left-fallback"]').element.value).toContain("left next");
    expect(monacoMocks.createDiffEditor).not.toHaveBeenCalled();
  });

  it("initializes Monaco, synchronizes models, theme, language and responsive layout", async () => {
    const { language, leftSource, listener, removeEventListener, rightSource, themeStore, wrapper } = mountDiff({
      browser: true,
      matches: true,
      height: 360,
    });
    await vi.waitFor(() => expect(monacoMocks.createDiffEditor).toHaveBeenCalledTimes(1));

    expect(monacoMocks.register).toHaveBeenCalledWith({ id: "pine-v6" });
    expect(monacoMocks.setLanguageConfiguration).toHaveBeenCalled();
    expect(monacoMocks.setMonarchTokensProvider).toHaveBeenCalled();
    expect(monacoMocks.createDiffEditor).toHaveBeenCalledWith(
      expect.any(HTMLElement),
      expect.objectContaining({ renderSideBySide: false, theme: "vs-dark" }),
    );
    expect(monacoMocks.createModel).toHaveBeenNthCalledWith(1, "strategy('left')", "pine-v6");
    expect(monacoMocks.createModel).toHaveBeenNthCalledWith(2, "strategy('right')", "pine-v6");
    expect(monacoMocks.setModel).toHaveBeenCalledWith({
      original: monacoMocks.models[0],
      modified: monacoMocks.models[1],
    });
    expect(wrapper.get('[aria-label="策略源码差异"]').attributes("style")).toContain("height: 360px");

    leftSource.value = "strategy('left changed')";
    rightSource.value = "strategy('right changed')";
    await nextTick();
    expect(monacoMocks.models[0]?.setValue).toHaveBeenCalledWith("strategy('left changed')");
    expect(monacoMocks.models[1]?.setValue).toHaveBeenCalledWith("strategy('right changed')");

    language.value = "pine-next";
    themeStore().set("light");
    await nextTick();
    expect(monacoMocks.register).toHaveBeenCalledWith({ id: "pine-next" });
    expect(monacoMocks.setModelLanguage).toHaveBeenCalledWith(monacoMocks.models[0], "pine-next");
    expect(monacoMocks.setModelLanguage).toHaveBeenCalledWith(monacoMocks.models[1], "pine-next");
    expect(monacoMocks.setTheme).toHaveBeenCalledWith("vs");

    listener.current?.({ matches: false } as MediaQueryListEvent);
    expect(monacoMocks.updateOptions).toHaveBeenCalledWith({ renderSideBySide: false });
    expect(monacoMocks.layout).toHaveBeenCalled();

    const environment = (globalThis as typeof globalThis & {
      MonacoEnvironment?: { getWorker: (moduleId: string, label: string) => Worker };
    }).MonacoEnvironment;
    expect(environment?.getWorker("", "javascript")).toBeTruthy();
    expect(environment?.getWorker("", "typescript")).toBeTruthy();
    expect(environment?.getWorker("", "editor")).toBeTruthy();

    wrapper.unmount();
    expect(removeEventListener).toHaveBeenCalledWith("change", expect.any(Function));
    expect(monacoMocks.editorDispose).toHaveBeenCalled();
    expect(monacoMocks.models[0]?.dispose).toHaveBeenCalled();
    expect(monacoMocks.models[1]?.dispose).toHaveBeenCalled();
  });

  it("reuses registered languages and falls back when Monaco cannot mount", async () => {
    monacoMocks.setLanguages([{ id: "pine-v6" }]);
    const existing = mountDiff({ browser: true });
    await vi.waitFor(() => expect(monacoMocks.createDiffEditor).toHaveBeenCalledTimes(1));
    expect(monacoMocks.register).not.toHaveBeenCalled();
    existing.wrapper.unmount();

    monacoMocks.createDiffEditor.mockImplementationOnce(() => {
      throw new Error("editor unavailable");
    });
    const failed = mountDiff({ browser: true });
    await vi.waitFor(() => expect(failed.wrapper.find('[data-testid="strategy-source-diff-left-fallback"]').exists()).toBe(true));
    expect(monacoMocks.models.at(-2)?.dispose).toHaveBeenCalled();
    expect(monacoMocks.models.at(-1)?.dispose).toHaveBeenCalled();
    failed.wrapper.unmount();
  });

  it("aborts asynchronous initialization after an immediate unmount", async () => {
    const { wrapper } = mountDiff({ browser: true });
    wrapper.unmount();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(monacoMocks.createDiffEditor).not.toHaveBeenCalled();
  });
});
