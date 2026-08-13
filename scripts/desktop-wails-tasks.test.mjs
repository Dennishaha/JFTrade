import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";

const read = (file) => fs.readFileSync(file, "utf8");

const root = read("Taskfile.yml");
const common = read("build/Taskfile.yml");
const config = read("build/config.yml");
const darwin = read("build/darwin/Taskfile.yml");
const windows = read("build/windows/Taskfile.yml");
const linux = read("build/linux/Taskfile.yml");
const packageJson = read("package.json");
const prepareRelease = read("scripts/prepare-desktop-release.mjs");
const msix = JSON.parse(read("build/windows/msix.json"));

test("root Taskfile follows the Wails native dispatch shape", () => {
  for (const variable of [
    'APP_NAME: "JFTrade"',
    'BIN_DIR: "bin"',
    'PACKAGE_MANAGER: "pnpm"',
    "VITE_PORT:",
    "GOOS:",
  ]) {
    assert(root.includes(variable), `root Taskfile is missing ${variable}`);
  }
  for (const task of ["build", "package", "run", "dev"]) {
    assert(root.includes(`  ${task}:`), `root Taskfile is missing ${task}`);
  }
  assert(root.includes('task: "{{.GOOS}}:build"'));
  assert(root.includes('task: "{{.GOOS}}:package"'));
  assert(root.includes('task: "{{.GOOS}}:run"'));
  assert(root.includes("go tool wails3 dev -config ./build/config.yml"));
  assert(!root.includes("scripts/dev-desktop.mjs"));
  assert(!root.includes("scripts/build-desktop.mjs"));
  assert(!root.includes("scripts/release-desktop.mjs"));
});

test("Wails dev mode owns frontend, build and app lifecycle", () => {
  for (const command of [
    "go tool wails3 build DEV=true",
    "go tool wails3 task common:dev:frontend",
    "go tool wails3 task run",
  ]) {
    assert(config.includes(command), `dev_mode is missing ${command}`);
  }
  assert(common.includes("pnpm run dev:web"));
  assert(common.includes("WAILS_VITE_PORT"));
  assert(common.includes('JFTRADE_DESKTOP_MODE: "1"'));
  assert(common.includes("VITE_DEV_API_TARGET:"));
  assert(!common.includes("build-pineworker-dev"));
  assert(!common.includes("build-marketdata-sidecar"));
  assert(!common.includes("desktop-dev-fast-path"));
  assert(root.includes("PINEWORKER_BUNDLE:"));
  assert(root.includes("pnpm run build:pineworker:dev"));
  assert(packageJson.includes('"prepare:desktop-dev": "go tool wails3 task prepare:dev"'));
  for (const taskfile of [darwin, windows, linux]) {
    assert(taskfile.includes("JFTRADE_PINEWORKER_BUNDLE"));
  }
});

test("release preparation is explicit and outside native build tasks", () => {
  assert(packageJson.includes('"prepare:desktop-release": "node scripts/prepare-desktop-release.mjs"'));
  assert(prepareRelease.includes('"build:frontend-assets"'));
  assert(prepareRelease.includes('"generate:wails-bindings"'));
  assert(prepareRelease.includes('"build:pineworker"'));
  assert(prepareRelease.includes('"build:marketdata-sidecar"'));
  assert(common.includes("JFTRADE_DESKTOP_PREPARED"));
  assert(common.includes("node scripts/prepare-desktop-release.mjs"));
  for (const taskfile of [darwin, windows, linux]) {
    assert(taskfile.includes("common:verify:release-inputs"));
  }
});

test("platform taskfiles use bin outputs and official Wails tools", () => {
  for (const taskfile of [darwin, windows, linux]) {
    assert(taskfile.includes("{{.BIN_DIR}}"));
    assert(!taskfile.includes("dist/desktop"));
    assert(!taskfile.includes("scripts/sign-desktop-release.mjs"));
  }
  assert(common.includes("go tool wails3 update build-assets"));
  assert(darwin.includes("go tool wails3 tool package"));
  assert(darwin.includes("--format dmg"));
  assert(darwin.includes('{{.BIN_DIR}}/{{.APP_NAME}}.app/Contents/MacOS/{{.APP_NAME}}'));
  assert(windows.includes("go tool wails3 generate syso"));
  assert(windows.includes("go tool wails3 generate webview2bootstrapper"));
  assert(windows.includes("makensis"));
  assert(linux.includes("go tool wails3 generate appimage"));
  assert(linux.includes("go tool wails3 tool package"));
  assert(linux.includes("production,release_assets,gtk3"));
});

test("package scripts call the pinned Wails tool directly", () => {
  assert(packageJson.includes('"desktop:dev": "go tool wails3 dev'));
  assert(packageJson.includes('"desktop:build": "go tool wails3 build"'));
  assert(packageJson.includes('"desktop:package": "go tool wails3 package"'));
  assert(packageJson.includes('"desktop:doctor": "go tool wails3 doctor"'));
  for (const script of [
    "scripts/wails3.mjs",
    "scripts/dev-desktop.mjs",
    "scripts/build-desktop.mjs",
    "scripts/release-desktop.mjs",
  ]) {
    assert(!packageJson.includes(script), `${script} remains in package scripts`);
  }
});

test("legacy desktop wrappers and dist outputs are absent", () => {
  for (const file of [
    "scripts/wails3.mjs",
    "scripts/dev-desktop.mjs",
    "scripts/build-desktop.mjs",
    "scripts/release-desktop.mjs",
    "scripts/compile-windows-nsis.mjs",
    "scripts/sign-desktop-release.mjs",
    "scripts/lib/desktop-dev-fast-path.mjs",
    "scripts/lib/windows-nsis.mjs",
    "build/darwin/package-dmg.sh",
    "build/darwin/verify-dmg.sh",
  ]) {
    assert(!fs.existsSync(file), `${file} should have been removed`);
  }

  for (const file of [
    ".vscode/tasks.json",
    ".vscode/launch.json",
    ".github/workflows/ci.yml",
    ".github/workflows/desktop-release.yml",
    "Taskfile.yml",
    "build/Taskfile.yml",
    "build/darwin/Taskfile.yml",
    "build/windows/Taskfile.yml",
    "build/linux/Taskfile.yml",
    "package.json",
  ]) {
    const content = read(file);
    for (const marker of [
      "dist/desktop",
      "desktop:release:",
      "desktop:build:",
      "desktop:package:",
      "scripts/wails3.mjs",
      "scripts/dev-desktop.mjs",
      "scripts/build-desktop.mjs",
      "scripts/release-desktop.mjs",
      "scripts/compile-windows-nsis.mjs",
      "scripts/sign-desktop-release.mjs",
      "desktop-dev-fast-path",
    ]) {
      assert(!content.includes(marker), `${file} still contains ${marker}`);
    }
  }
});

test("Windows MSIX metadata keeps the JFTrade application identity", () => {
  assert.equal(msix.info.productName, "JFTrade");
  assert.equal(msix.info.productIdentifier, "com.jftrade.desktop");
  assert.deepEqual(msix.fileAssociations, []);
});
