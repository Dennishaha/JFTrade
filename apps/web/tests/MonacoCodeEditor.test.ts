// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, nextTick, ref } from "vue";

import MonacoCodeEditor from "../src/components/MonacoCodeEditor.vue";
import { provideThemeStore, type ThemeStore } from "../src/composables/useTheme";

const monacoMocks = vi.hoisted(() => {
  const dispose = vi.fn();
  const extraLibDispose = vi.fn();
  const completionDispose = vi.fn();
  const hoverDispose = vi.fn();
  const modelChangeDispose = vi.fn();
  const blurDispose = vi.fn();
  const cursorDispose = vi.fn();
  const model = {
    getLineContent: vi.fn((line: number) => (line === 1 ? "ctx.kline.close" : "")),
    getOffsetAt: vi.fn(() => 17),
    getValue: vi.fn(() => "const initial = true;"),
    getValueInRange: vi.fn(() => "ctx."),
    getWordUntilPosition: vi.fn(() => ({ startColumn: 1, endColumn: 5 })),
    setValue: vi.fn(),
  };
  let currentValue = "const initial = true;";
  let modelChangeHandler: (() => void) | null = null;
  let blurHandler: (() => void) | null = null;
  let cursorHandler: ((event: { position: { lineNumber: number; column: number } }) => void) | null = null;
  const editorInstance = {
    dispose,
    getModel: vi.fn(() => model),
    getValue: vi.fn(() => currentValue),
    onDidBlurEditorText: vi.fn((handler: () => void) => {
      blurHandler = handler;
      return { dispose: blurDispose };
    }),
    onDidChangeCursorPosition: vi.fn(
      (handler: (event: { position: { lineNumber: number; column: number } }) => void) => {
        cursorHandler = handler;
        return { dispose: cursorDispose };
      },
    ),
    onDidChangeModelContent: vi.fn((handler: () => void) => {
      modelChangeHandler = handler;
      return { dispose: modelChangeDispose };
    }),
    revealRangeInCenter: vi.fn(),
    setSelection: vi.fn(),
    updateOptions: vi.fn(),
  };
  const create = vi.fn(() => editorInstance);
  const setModelLanguage = vi.fn();
  const setModelMarkers = vi.fn();
  const setTheme = vi.fn();
  const register = vi.fn();
  const setLanguageConfiguration = vi.fn();
  const setMonarchTokensProvider = vi.fn();
  let registeredLanguages: Array<{ id: string }> = [];
  let completionProvider: {
    provideCompletionItems: (
      model: typeof monacoMocks.model,
      position: { lineNumber: number; column: number },
    ) => { suggestions: Array<Record<string, unknown>> };
  } | null = null;
  let hoverProvider: {
    provideHover: (
      model: typeof monacoMocks.model,
      position: { lineNumber: number; column: number },
    ) => Record<string, unknown> | null;
  } | null = null;
  const registerCompletionItemProvider = vi.fn((_language: string, provider: typeof completionProvider) => {
    completionProvider = provider;
    return { dispose: completionDispose };
  });
  const registerHoverProvider = vi.fn((_language: string, provider: typeof hoverProvider) => {
    hoverProvider = provider;
    return { dispose: hoverDispose };
  });
  const addExtraLib = vi.fn(() => ({ dispose: extraLibDispose }));
  const setCompilerOptions = vi.fn();
  const setDiagnosticsOptions = vi.fn();
  const setEagerModelSync = vi.fn();

  return {
    addExtraLib,
    blurDispose,
    completionDispose,
    create,
    cursorDispose,
    dispose,
    editorInstance,
    extraLibDispose,
    getCompletionProvider: () => completionProvider,
    getHoverProvider: () => hoverProvider,
    getLanguages: () => registeredLanguages,
    hoverDispose,
    model,
    modelChangeDispose,
    register,
    registerCompletionItemProvider,
    registerHoverProvider,
    resetRuntime() {
      currentValue = "const initial = true;";
      modelChangeHandler = null;
      blurHandler = null;
      cursorHandler = null;
      completionProvider = null;
      hoverProvider = null;
      registeredLanguages = [];
      model.getLineContent.mockImplementation((line: number) =>
        line === 1 ? "ctx.kline.close" : "",
      );
      model.getValue.mockReturnValue("const initial = true;");
      model.getValueInRange.mockReturnValue("ctx.");
    },
    setCompilerOptions,
    setDiagnosticsOptions,
    setEagerModelSync,
    setLanguageConfiguration,
    setLanguages(value: Array<{ id: string }>) {
      registeredLanguages = value;
    },
    setModelLanguage,
    setModelMarkers,
    setMonarchTokensProvider,
    setTheme,
    triggerBlur() {
      blurHandler?.();
    },
    triggerCursor() {
      cursorHandler?.({ position: { lineNumber: 2, column: 4 } });
    },
    triggerModelChange(value: string) {
      currentValue = value;
      modelChangeHandler?.();
    },
  };
});

vi.mock("monaco-editor/esm/vs/editor/editor.worker?worker", () => ({
  default: class EditorWorker {},
}));

vi.mock("monaco-editor/esm/vs/language/typescript/ts.worker?worker", () => ({
  default: class TypeScriptWorker {},
}));

vi.mock("monaco-editor", () => ({
  MarkerSeverity: { Error: 8, Warning: 4, Info: 2 },
  editor: {
    create: monacoMocks.create,
    setModelLanguage: monacoMocks.setModelLanguage,
    setModelMarkers: monacoMocks.setModelMarkers,
    setTheme: monacoMocks.setTheme,
  },
  languages: {
    CompletionItemInsertTextRule: { InsertAsSnippet: 4 },
    CompletionItemKind: {
      Function: 1,
      Interface: 2,
      Property: 3,
      Snippet: 4,
      Variable: 5,
    },
    getLanguages: monacoMocks.getLanguages,
    register: monacoMocks.register,
    registerCompletionItemProvider: monacoMocks.registerCompletionItemProvider,
    registerHoverProvider: monacoMocks.registerHoverProvider,
    setLanguageConfiguration: monacoMocks.setLanguageConfiguration,
    setMonarchTokensProvider: monacoMocks.setMonarchTokensProvider,
  },
  typescript: {
    ModuleKind: { ESNext: 99 },
    ScriptTarget: { ES2020: 77 },
    javascriptDefaults: {
      addExtraLib: monacoMocks.addExtraLib,
      setCompilerOptions: monacoMocks.setCompilerOptions,
      setDiagnosticsOptions: monacoMocks.setDiagnosticsOptions,
      setEagerModelSync: monacoMocks.setEagerModelSync,
    },
  },
}));

beforeEach(() => {
  monacoMocks.resetRuntime();
  Object.values(monacoMocks).forEach((value) => {
    if (typeof value === "function" && "mockClear" in value) {
      (value as ReturnType<typeof vi.fn>).mockClear();
    }
  });
  Object.values(monacoMocks.editorInstance).forEach((value) => {
    if (typeof value === "function" && "mockClear" in value) {
      (value as ReturnType<typeof vi.fn>).mockClear();
    }
  });
  Object.values(monacoMocks.model).forEach((value) => {
    if (typeof value === "function" && "mockClear" in value) {
      (value as ReturnType<typeof vi.fn>).mockClear();
    }
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.localStorage.clear();
  document.body.innerHTML = "";
});

async function mountBrowserEditor() {
  vi.stubGlobal("navigator", { userAgent: "Mozilla/5.0 Chrome/126" });
  const props = {
    value: ref("const initial = true;"),
    language: ref("pine-v6"),
    readOnly: ref(false),
    markers: ref([
      { severity: "error" as const, message: "syntax", line: 0, column: 0, endLine: 0, endColumn: 0 },
      { severity: "warning" as const, message: "risk", line: 2, column: 3, endLine: 2, endColumn: 7 },
      { severity: "info" as const, message: "hint", line: 3, column: 1, endLine: 3, endColumn: 2 },
    ]),
  };
  let themeStore: ThemeStore | null = null;
  const Host = defineComponent({
    components: { MonacoCodeEditor },
    setup() {
      themeStore = provideThemeStore();
      return props;
    },
    template: `
      <MonacoCodeEditor
        ref="editor"
        v-model="value"
        :language="language"
        :read-only="readOnly"
        :diagnostic-markers="markers"
        :height="320"
        :min-height="180"
        resizable
        test-id="pine-editor"
        :extra-libs="[{ filePath: 'file:///jftrade.d.ts', content: 'declare const ctx: unknown' }]"
        :completion-items="[
          { label: 'plot', insertText: 'plot(\${1:value})', detail: 'function', documentation: '绘图', kind: 'function', insertTextRule: 'snippet' },
          { label: 'State', insertText: 'State', detail: 'interface', documentation: '状态', kind: 'interface' },
          { label: 'position', insertText: 'position', detail: 'variable', documentation: '持仓', kind: 'variable' },
          { label: 'if', insertText: 'if ', detail: 'snippet', documentation: '条件' }
        ]"
        :hover-items="[
          { target: 'ctx.kline.close', signature: 'close: number', documentation: '收盘价' },
          { target: 'ctx.kline', signature: 'kline: KLine', documentation: 'K线' },
          { target: 'close', signature: 'close: number', documentation: '字段' }
        ]"
        @blur="$emit('editor-blur')"
        @cursor-offset="$emit('cursor-offset', $event)"
      />
    `,
  });
  const wrapper = mount(Host, { attachTo: document.body });
  await vi.waitFor(() => expect(monacoMocks.create).toHaveBeenCalledTimes(1));
  await nextTick();
  return { props, themeStore: () => themeStore!, wrapper };
}

describe("MonacoCodeEditor", () => {
  it("uses a functional textarea fallback and reveals source ranges safely", async () => {
    const value = ref("line one\nline two");
    const readOnly = ref(false);
    const language = ref("pine-v6");
    const markers = ref<Array<{ severity: "error"; message: string; line: number; column: number; endLine: number; endColumn: number }>>([]);
    let themeStore: ThemeStore | null = null;
    const Host = defineComponent({
      components: { MonacoCodeEditor },
      setup() {
        themeStore = provideThemeStore();
        return { language, markers, readOnly, value };
      },
      template: `
        <MonacoCodeEditor
          ref="editor"
          v-model="value"
          :language="language"
          :diagnostic-markers="markers"
          :read-only="readOnly"
          :height="200"
          min-height="120px"
          placeholder="Pine source"
          test-id="fallback-editor"
          resizable
          @blur="$emit('editor-blur')"
        />
      `,
    });
    const wrapper = mount(Host);
    const textarea = wrapper.get<HTMLTextAreaElement>("textarea");
    expect(textarea.attributes("placeholder")).toBe("Pine source");
    expect(wrapper.get(".monaco-code-editor-shell").attributes("style")).toContain("height: 200px");

    await textarea.setValue("updated source");
    await textarea.trigger("blur");
    expect(value.value).toBe("updated source");
    expect(wrapper.emitted("editor-blur")).toHaveLength(1);

    const editor = wrapper.getComponent(MonacoCodeEditor);
    (editor.vm as unknown as { revealOffsetRange: (range: { start: number; end: number }) => void })
      .revealOffsetRange({ start: -10, end: 999 });
    expect(textarea.element.selectionStart).toBe(0);
    expect(textarea.element.selectionEnd).toBe("updated source".length);

    readOnly.value = true;
    language.value = "javascript";
    markers.value = [
      { severity: "error", message: "fallback marker", line: 1, column: 1, endLine: 1, endColumn: 2 },
    ];
    themeStore!.set("light");
    await nextTick();
    await textarea.setValue("blocked");
    expect(editor.emitted("update:modelValue")).toHaveLength(1);
    wrapper.unmount();
    (editor.vm as unknown as { revealOffsetRange: (range: { start: number; end: number }) => void })
      .revealOffsetRange({ start: 0, end: 1 });
  });

  it("initializes Pine support and keeps diagnostics, options and model state synchronized", async () => {
    const { props, themeStore, wrapper } = await mountBrowserEditor();

    expect(monacoMocks.register).toHaveBeenCalledWith({ id: "pine-v6" });
    expect(monacoMocks.setLanguageConfiguration).toHaveBeenCalledWith(
      "pine-v6",
      expect.objectContaining({ comments: { lineComment: "//" } }),
    );
    expect(monacoMocks.setMonarchTokensProvider).toHaveBeenCalled();
    expect(monacoMocks.setEagerModelSync).toHaveBeenCalledWith(true);
    expect(monacoMocks.addExtraLib).toHaveBeenCalledWith(
      "declare const ctx: unknown",
      "file:///jftrade.d.ts",
    );
    expect(monacoMocks.create).toHaveBeenCalledWith(
      expect.any(HTMLElement),
      expect.objectContaining({ language: "pine-v6", theme: "vs-dark", readOnly: false }),
    );
    expect(monacoMocks.setModelMarkers).toHaveBeenLastCalledWith(
      monacoMocks.model,
      "jftrade-pine",
      [
        expect.objectContaining({ severity: 8, startLineNumber: 1, startColumn: 1, endLineNumber: 1, endColumn: 1 }),
        expect.objectContaining({ severity: 4, startLineNumber: 2, startColumn: 3, endColumn: 2, endColumn: 7 }),
        expect.objectContaining({ severity: 2 }),
      ],
    );

    props.value.value = "const next = false;";
    props.language.value = "javascript";
    props.readOnly.value = true;
    props.markers.value = [];
    themeStore().set("light");
    await nextTick();
    await nextTick();

    expect(monacoMocks.model.setValue).toHaveBeenCalledWith("const next = false;");
    expect(monacoMocks.setModelLanguage).toHaveBeenCalledWith(monacoMocks.model, "javascript");
    expect(monacoMocks.editorInstance.updateOptions).toHaveBeenCalledWith({
      readOnly: true,
      domReadOnly: true,
      contextmenu: false,
      cursorStyle: "line-thin",
      occurrencesHighlight: "off",
    });
    expect(monacoMocks.setTheme).toHaveBeenCalledWith("vs");
    expect(monacoMocks.setModelMarkers).toHaveBeenLastCalledWith(monacoMocks.model, "jftrade-pine", []);

    monacoMocks.model.getValue.mockReturnValueOnce("already synchronized");
    props.value.value = "already synchronized";
    monacoMocks.editorInstance.getModel.mockReturnValueOnce(null);
    props.language.value = "pine-v6";
    await nextTick();
    monacoMocks.editorInstance.getModel.mockReturnValueOnce(null);
    props.markers.value = [
      { severity: "info", message: "no model", line: 1, column: 1, endLine: 1, endColumn: 2 },
    ];
    await nextTick();

    monacoMocks.model.setValue.mockImplementationOnce(() => {
      monacoMocks.triggerModelChange("ignored programmatic update");
    });
    props.value.value = "programmatic update";
    await nextTick();

    monacoMocks.triggerModelChange("strategy changed by user");
    monacoMocks.triggerBlur();
    monacoMocks.triggerCursor();
    monacoMocks.editorInstance.getModel.mockReturnValueOnce(null);
    monacoMocks.triggerCursor();
    await nextTick();
    expect(props.value.value).toBe("strategy changed by user");
    expect(wrapper.emitted("editor-blur")).toHaveLength(1);
    expect(wrapper.emitted("cursor-offset")).toEqual([[17]]);

    const editor = wrapper.getComponent(MonacoCodeEditor);
    (editor.vm as unknown as { revealOffsetRange: (range: { start: number; end: number }) => void })
      .revealOffsetRange({ start: 6, end: 50 });
    expect(monacoMocks.editorInstance.setSelection).toHaveBeenCalledWith({
      startLineNumber: 1,
      startColumn: 7,
      endLineNumber: 1,
      endColumn: "strategy changed by user".length + 1,
    });

    wrapper.unmount();
    expect(monacoMocks.modelChangeDispose).toHaveBeenCalled();
    expect(monacoMocks.blurDispose).toHaveBeenCalled();
    expect(monacoMocks.cursorDispose).toHaveBeenCalled();
    expect(monacoMocks.completionDispose).toHaveBeenCalled();
    expect(monacoMocks.hoverDispose).toHaveBeenCalled();
    expect(monacoMocks.extraLibDispose).toHaveBeenCalled();
    expect(monacoMocks.dispose).toHaveBeenCalled();
  });

  it("provides domain-aware completions and exact Pine runtime hover documentation", async () => {
    const { wrapper } = await mountBrowserEditor();
    const completionProvider = monacoMocks.getCompletionProvider();
    const hoverProvider = monacoMocks.getHoverProvider();
    expect(completionProvider).not.toBeNull();
    expect(hoverProvider).not.toBeNull();

    monacoMocks.model.getValueInRange.mockReturnValueOnce("ctx.kline.");
    const klineSuggestions = completionProvider!.provideCompletionItems(
      monacoMocks.model,
      { lineNumber: 1, column: 11 },
    ).suggestions;
    expect(klineSuggestions.map((item) => item.label)).toEqual(
      expect.arrayContaining(["open", "close", "volume", "symbol", "plot", "State", "position", "if"]),
    );
    expect(klineSuggestions.find((item) => item.label === "plot")).toMatchObject({
      kind: 1,
      insertTextRules: 4,
    });
    expect(klineSuggestions.find((item) => item.label === "State")).toMatchObject({ kind: 2 });
    expect(klineSuggestions.find((item) => item.label === "position")).toMatchObject({ kind: 5 });

    monacoMocks.model.getValueInRange.mockReturnValueOnce("ctx.");
    const contextSuggestions = completionProvider!.provideCompletionItems(
      monacoMocks.model,
      { lineNumber: 1, column: 5 },
    ).suggestions;
    expect(contextSuggestions.map((item) => item.label)).toEqual(
      expect.arrayContaining(["id", "definitionId", "kline", "plot"]),
    );

    monacoMocks.model.getValueInRange.mockReturnValueOnce("plain");
    const genericSuggestions = completionProvider!.provideCompletionItems(
      monacoMocks.model,
      { lineNumber: 1, column: 6 },
    ).suggestions;
    expect(genericSuggestions).toHaveLength(4);

    const environment = (globalThis as typeof globalThis & {
      MonacoEnvironment?: { getWorker: (moduleId: string, label: string) => Worker };
    }).MonacoEnvironment;
    expect(environment).toBeDefined();
    expect(environment!.getWorker("", "javascript")).toBeTruthy();
    expect(environment!.getWorker("", "typescript")).toBeTruthy();
    expect(environment!.getWorker("", "editor")).toBeTruthy();

    const fullExpressionHover = hoverProvider!.provideHover(
      monacoMocks.model,
      { lineNumber: 1, column: 12 },
    );
    expect(fullExpressionHover).toMatchObject({
      contents: [
        { value: "**ctx.kline.close**" },
        { value: "```ts\nclose: number\n```" },
        { value: "收盘价" },
      ],
    });

    expect(hoverProvider!.provideHover(monacoMocks.model, { lineNumber: 1, column: 10 })).toBeNull();
    monacoMocks.model.getLineContent.mockReturnValueOnce("unknown.path");
    expect(hoverProvider!.provideHover(monacoMocks.model, { lineNumber: 1, column: 3 })).toBeNull();
    monacoMocks.model.getLineContent.mockReturnValueOnce("ctx.kline.close;");
    expect(hoverProvider!.provideHover(monacoMocks.model, { lineNumber: 1, column: 16 })).not.toBeNull();
    monacoMocks.model.getLineContent.mockReturnValueOnce(".");
    expect(hoverProvider!.provideHover(monacoMocks.model, { lineNumber: 1, column: 1 })).toBeNull();

    monacoMocks.model.getLineContent.mockReturnValueOnce("const x = ");
    expect(hoverProvider!.provideHover(monacoMocks.model, { lineNumber: 1, column: 10 })).toBeNull();
    monacoMocks.model.getLineContent.mockReturnValueOnce("");
    expect(hoverProvider!.provideHover(monacoMocks.model, { lineNumber: 1, column: 1 })).toBeNull();

    wrapper.unmount();
  });

  it("does not re-register Pine when the language already exists", async () => {
    monacoMocks.setLanguages([{ id: "pine-v6" }]);
    const { wrapper } = await mountBrowserEditor();
    expect(monacoMocks.register).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("aborts initialization when navigation unmounts the editor before imports settle", async () => {
    vi.stubGlobal("navigator", { userAgent: "Mozilla/5.0 Chrome/126" });
    const Host = defineComponent({
      components: { MonacoCodeEditor },
      setup() {
        provideThemeStore();
        return { value: ref("strategy source") };
      },
      template: '<MonacoCodeEditor v-model="value" />',
    });
    const wrapper = mount(Host, { attachTo: document.body });
    wrapper.unmount();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(monacoMocks.create).not.toHaveBeenCalled();
  });

  it("disposes pending registrations when the editor target disappears during setup", async () => {
    monacoMocks.registerHoverProvider.mockImplementationOnce((_language, provider) => {
      void provider;
      document.querySelector('[data-testid="pine-editor"]')?.remove();
      return { dispose: monacoMocks.hoverDispose };
    });

    const { wrapper } = await mountBrowserEditorWithoutCreate();
    await vi.waitFor(() => expect(monacoMocks.extraLibDispose).toHaveBeenCalled());
    expect(monacoMocks.completionDispose).toHaveBeenCalled();
    expect(monacoMocks.hoverDispose).toHaveBeenCalled();
    expect(monacoMocks.create).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("falls back when Monaco creation fails and disposes an editor invalidated after creation", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    monacoMocks.create.mockImplementationOnce(() => {
      throw new Error("worker policy blocked");
    });
    const failed = await mountBrowserEditor();
    await nextTick();
    expect(failed.wrapper.find("textarea").exists()).toBe(true);
    expect(errorSpy).toHaveBeenCalledWith(
      "failed to initialize Monaco editor",
      expect.objectContaining({ message: "worker policy blocked" }),
    );
    failed.wrapper.unmount();

    monacoMocks.create.mockImplementationOnce(() => {
      document.querySelector('[data-testid="pine-editor"]')?.remove();
      return monacoMocks.editorInstance;
    });
    const invalidated = await mountBrowserEditor();
    await vi.waitFor(() => expect(monacoMocks.dispose).toHaveBeenCalled());
    expect(monacoMocks.extraLibDispose).toHaveBeenCalled();
    invalidated.wrapper.unmount();
  });
});

async function mountBrowserEditorWithoutCreate() {
  vi.stubGlobal("navigator", { userAgent: "Mozilla/5.0 Chrome/126" });
  const Host = defineComponent({
    components: { MonacoCodeEditor },
    setup() {
      provideThemeStore();
      return { value: ref("strategy source") };
    },
    template: `
      <MonacoCodeEditor
        v-model="value"
        language="pine-v6"
        test-id="pine-editor"
        :extra-libs="[{ filePath: 'file:///jftrade.d.ts', content: 'declare const ctx: unknown' }]"
        :completion-items="[{ label: 'plot', insertText: 'plot()', detail: 'function', documentation: '绘图' }]"
        :hover-items="[{ target: 'ctx', signature: 'ctx: Context', documentation: '上下文' }]"
      />
    `,
  });
  const wrapper = mount(Host, { attachTo: document.body });
  await nextTick();
  return { wrapper };
}
