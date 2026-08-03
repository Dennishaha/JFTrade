import { existsSync, readFileSync, readdirSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const defaultWebPackagePath = fileURLToPath(
  new URL("../../apps/web/package.json", import.meta.url),
);

export const requiredMonacoSubpaths = Object.freeze([
  "features/register.all.js",
  "editor/browser/coreCommands.js",
  "editor/contrib/caretOperations/browser/caretOperations.js",
  "editor/contrib/dropOrPasteInto/browser/copyPasteContribution.js",
  "editor/contrib/find/browser/findController.js",
  "editor/contrib/gotoSymbol/browser/goToCommands.js",
  "editor/contrib/gotoError/browser/markerSelectionStatus.js",
  "editor/contrib/semanticTokens/browser/documentSemanticTokens.js",
  "editor/contrib/suggest/browser/suggestController.js",
  "editor/common/standaloneStrings.js",
  "editor/editor.api.js",
  "editor/editor.worker.js",
  "language/typescript/ts.worker.js",
  "languages/definitions/javascript/register.js",
  "languages/definitions/typescript/register.js",
  "languages/features/typescript/register.js",
]);

export function resolveMonacoSubpath(subpath, webPackagePath = defaultWebPackagePath) {
  const require = createRequire(resolve(webPackagePath));
  return require.resolve(`monaco-editor/${subpath}`);
}

export function inspectMonacoLayout(webPackagePath = defaultWebPackagePath) {
  const webPackage = JSON.parse(readFileSync(webPackagePath, "utf8"));
  const declaredVersion = webPackage.dependencies?.["monaco-editor"];
  if (typeof declaredVersion !== "string" || !/^\d+\.\d+\.\d+$/.test(declaredVersion)) {
    throw new Error("apps/web must pin monaco-editor to an exact version");
  }

  const resolvedSubpaths = Object.fromEntries(
    requiredMonacoSubpaths.map((subpath) => [
      subpath,
      resolveMonacoSubpath(subpath, webPackagePath),
    ]),
  );
  const javascriptRegistration = resolvedSubpaths["languages/definitions/javascript/register.js"];
  const packageRoot = findMonacoPackageRoot(javascriptRegistration);
  const installedPackage = JSON.parse(readFileSync(join(packageRoot, "package.json"), "utf8"));
  if (installedPackage.version !== declaredVersion) {
    throw new Error(
      `monaco-editor manifest ${declaredVersion} does not match installed ${installedPackage.version}`,
    );
  }

  const definitionsRoot = dirname(dirname(javascriptRegistration));
  const languageDefinitions = readdirSync(definitionsRoot, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .sort();
  for (const requiredLanguage of ["javascript", "typescript"]) {
    if (!languageDefinitions.includes(requiredLanguage)) {
      throw new Error(`monaco-editor language definition is missing: ${requiredLanguage}`);
    }
  }

  return {
    declaredVersion,
    definitionsRoot,
    installedVersion: installedPackage.version,
    languageDefinitions,
    packageRoot,
    resolvedSubpaths,
  };
}

function findMonacoPackageRoot(path) {
  let directory = dirname(path);
  while (true) {
    const packagePath = join(directory, "package.json");
    if (existsSync(packagePath)) {
      const packageData = JSON.parse(readFileSync(packagePath, "utf8"));
      if (packageData.name === "monaco-editor") return directory;
    }
    const parent = dirname(directory);
    if (parent === directory) break;
    directory = parent;
  }
  throw new Error(`unable to locate monaco-editor package root from ${path}`);
}
