export type MonacoTypeScriptSupport = typeof import(
  "monaco-editor/languages/features/typescript/register.js"
);

let supportPromise: Promise<MonacoTypeScriptSupport> | null = null;

export function loadMonacoTypeScriptSupport(): Promise<MonacoTypeScriptSupport> {
  supportPromise ??= Promise.all([
    import("monaco-editor/languages/definitions/javascript/register.js"),
    import("monaco-editor/languages/definitions/typescript/register.js"),
    import("monaco-editor/languages/features/typescript/register.js"),
  ]).then(([, , typescriptSupport]) => typescriptSupport);
  return supportPromise;
}
