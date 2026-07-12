import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";

import { resolveWindowsNSISInvocation } from "./lib/windows-nsis.mjs";

const read = (file) => fs.readFileSync(file, "utf8");
const root = read("Taskfile.yml");
const common = read("build/Taskfile.yml");
const darwin = read("build/darwin/Taskfile.yml");
const windows = read("build/windows/Taskfile.yml");
const linux = read("build/linux/Taskfile.yml");

for (const standardTask of ["build:", "package:", "dev:"]) {
  assert(
    root.includes(`  ${standardTask}`),
    `root Taskfile is missing standard Wails task ${standardTask}`,
  );
}

for (const include of [
  "build/Taskfile.yml",
  "build/darwin/Taskfile.yml",
  "build/windows/Taskfile.yml",
  "build/linux/Taskfile.yml",
]) {
  assert(root.includes(include), `root Taskfile does not include ${include}`);
}
for (const taskfile of [darwin, windows]) {
  assert(
    taskfile.includes("production,release_assets"),
    "release build is missing Wails production tag",
  );
  assert(
    taskfile.includes("-trimpath") && taskfile.includes("-w -s"),
    "release build is missing Wails production flags",
  );
}
assert(
  linux.includes("production,release_assets,gtk3"),
  "Linux release is missing production/release_assets/gtk3 tags",
);
assert(
  windows.includes("-H windowsgui"),
  "Windows release is not a GUI subsystem build",
);
assert(
  windows.includes("defer:") &&
    windows.includes("jftrade_windows_{{.ARCH}}.syso"),
  "Windows generated syso is not cleaned on build failure",
);
assert(
  windows.includes("generate syso") &&
    windows.includes("generate webview2bootstrapper"),
  "Windows does not use Wails resource and WebView2 tools",
);
assert(
  windows.includes(
    "node scripts/compile-windows-nsis.mjs {{.ARCH}} {{.INSTALLER}}",
  ) && !windows.includes("{{.MAKENSIS}}"),
  "Windows NSIS command must bypass Task shell argument parsing",
);
const nsisInvocation = resolveWindowsNSISInvocation({
  arch: "arm64",
  installer: "JFTrade-0.2.2-windows-arm64-preview-unsigned-setup.exe",
  makensis: "C:\\Program Files (x86)\\NSIS\\makensis.exe",
  rootDir: path.resolve("test workspace"),
});
assert.equal(
  nsisInvocation.command,
  "C:\\Program Files (x86)\\NSIS\\makensis.exe",
);
assert(nsisInvocation.cwd.endsWith(path.join("windows-arm64", "nsis")));
assert(
  nsisInvocation.args.some(
    (argument) =>
      argument.startsWith("/DARG_WAILS_ARM64_BINARY=") &&
      argument.includes("test workspace") &&
      argument.endsWith("jftrade-desktop-windows-arm64.exe"),
  ),
  "Windows NSIS wrapper does not preserve the absolute ARM64 binary path",
);
assert(
  nsisInvocation.args.some(
    (argument) =>
      argument.startsWith("/DOUTPUT_EXE=") &&
      argument.endsWith(
        "JFTrade-0.2.2-windows-arm64-preview-unsigned-setup.exe",
      ),
  ),
  "Windows NSIS wrapper does not preserve the absolute output path",
);
assert(
  darwin.includes("codesign --verify --deep --strict"),
  "macOS bundle sealing is not verified",
);
assert(
  darwin.includes("package-dmg.sh") && darwin.includes("verify-dmg.sh"),
  "macOS release does not build and verify the drag-install DMG",
);
const dmgPackager = read("build/darwin/package-dmg.sh");
const dmgBackground = read("build/darwin/dmg-background.svg");
assert(
  dmgPackager.includes("ln -s /Applications") &&
    dmgPackager.includes("background picture") &&
    dmgPackager.includes('position of item "JFTrade.app"') &&
    dmgPackager.includes('position of item "Applications"'),
  "macOS DMG is missing the Applications shortcut or Finder drag layout",
);
assert(
  dmgBackground.includes('width="1320"') &&
    dmgBackground.includes('height="800"') &&
    dmgPackager.includes("dpiWidth 144") &&
    dmgPackager.includes("dpiHeight 144"),
  "macOS DMG background is not generated from a Retina 2x vector source",
);
assert(
  darwin.includes("build:dev") &&
    read("build/darwin/Info.dev.plist").includes("com.jftrade.desktop.dev"),
  "macOS development bundle is not isolated through Wails tasks",
);
assert(
  linux.includes("generate appimage") && linux.includes("tool package"),
  "Linux does not use Wails packaging tools",
);
assert(
  linux.includes("tool package -name jftrade"),
  "Linux package name must remain lowercase for Debian compatibility",
);
assert(
  common.includes("update build-assets") && common.includes("generate icons"),
  "common task does not use Wails build asset generation",
);

const nfpm = read("build/linux/nfpm.yaml");
assert(nfpm.includes("name: jftrade"), "Linux package name is not lowercase");
assert(
  /maintainer: .+<[^<>\s]+@[^<>\s]+>/.test(nfpm),
  "Linux package maintainer is missing a valid email address",
);
assert(
  nfpm.includes("homepage: https://github.com/Dennishaha/jftrade"),
  "Linux package homepage does not match the source repository",
);
assert(
  nfpm.includes("AGPL-3.0-only") && !nfpm.includes("license: Proprietary"),
  "Linux package metadata omits the bundled AGPL component",
);
assert(
  nfpm.includes("libgtk-3-0") && nfpm.includes("libwebkit2gtk-4.1-0"),
  "Linux package dependencies do not match GTK3 build",
);
