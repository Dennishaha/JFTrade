import { describe, expect, it, vi } from "vitest";

const supportMocks = vi.hoisted(() => ({
  javascriptRegister: vi.fn(),
  support: { javascriptDefaults: {} },
  typescriptDefinitionRegister: vi.fn(),
  typescriptFeatureRegister: vi.fn(),
}));

vi.mock("monaco-editor/languages/definitions/javascript/register.js", () => {
  supportMocks.javascriptRegister();
  return {};
});

vi.mock("monaco-editor/languages/definitions/typescript/register.js", () => {
  supportMocks.typescriptDefinitionRegister();
  return {};
});

vi.mock("monaco-editor/languages/features/typescript/register.js", () => {
  supportMocks.typescriptFeatureRegister();
  return supportMocks.support;
});

import { loadMonacoTypeScriptSupport } from "@/monacoTypescriptSupport";

describe("Monaco TypeScript support loader", () => {
  it("loads every registration once and reuses the in-flight result", async () => {
    const [first, second] = await Promise.all([
      loadMonacoTypeScriptSupport(),
      loadMonacoTypeScriptSupport(),
    ]);
    const third = await loadMonacoTypeScriptSupport();

    expect(first.javascriptDefaults).toBe(supportMocks.support.javascriptDefaults);
    expect(second).toBe(first);
    expect(third).toBe(first);
    expect(supportMocks.javascriptRegister).toHaveBeenCalledTimes(1);
    expect(supportMocks.typescriptDefinitionRegister).toHaveBeenCalledTimes(1);
    expect(supportMocks.typescriptFeatureRegister).toHaveBeenCalledTimes(1);
  });
});
