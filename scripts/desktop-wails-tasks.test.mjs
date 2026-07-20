import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";

import { resolveWindowsNSISInvocation } from "./lib/windows-nsis.mjs";

const read = (file) => fs.readFileSync(file, "utf8");
const root = read("Taskfile.yml");
const packageJson = read("package.json");
const common = read("build/Taskfile.yml");
const darwin = read("build/darwin/Taskfile.yml");
const windows = read("build/windows/Taskfile.yml");
const linux = read("build/linux/Taskfile.yml");
const nsisProject = read("build/windows/nsis/project.nsi");
const releaseWorkflow = read(".github/workflows/desktop-release.yml");

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
      argument.startsWith("/DJFTRADE_LICENSE_FILE=") &&
      argument.endsWith("LICENSE"),
  ) &&
    nsisInvocation.args.some(
      (argument) =>
        argument.startsWith("/DJFTRADE_THIRD_PARTY_NOTICES_FILE=") &&
        argument.endsWith("third-party-notices.md"),
    ),
  "Windows NSIS wrapper does not pass legal notice inputs",
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
  darwin.includes("Contents/Resources/licenses/LICENSE") &&
    darwin.includes("Contents/Resources/licenses/THIRD-PARTY-NOTICES.md"),
  "macOS bundles do not carry the project license and third-party notices",
);
assert(
  nsisProject.includes("$INSTDIR\\licenses") &&
    nsisProject.includes("/oname=LICENSE") &&
    nsisProject.includes("/oname=THIRD-PARTY-NOTICES.md"),
  "Windows installer does not carry the project license and third-party notices",
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
  dmgPackager.includes("for attempt in 1 2 3 4 5") &&
    dmgPackager.includes('hdiutil detach "$device" -force'),
  "macOS DMG packaging does not recover from a temporarily busy mounted image",
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
  !linux.includes("LDAI_COMP=") &&
    linux.includes("FORMAT: deb") &&
    linux.includes("FORMAT: rpm"),
  "Linux release must use compatible AppImage defaults and build deb and rpm artifacts",
);
assert(
  linux.includes("manage-linux-release-artifacts.mjs verify") &&
    linux.includes("dist/desktop-release/.staging/linux-{{.ARCH}}"),
  "Linux release does not isolate staging or verify its final artifact set",
);
assert(
  !linux.includes("archlinux") &&
    !root.includes("desktop:package:linux-arch") &&
    !packageJson.includes("desktop:package:linux-arch"),
  "Arch Linux packaging entrypoints must not remain",
);
assert(
  !linux.includes(
    "cp dist/desktop/linux-{{.ARCH}}/jftrade-desktop-linux-{{.ARCH}} dist/desktop-release",
  ),
  "Linux raw binary must not be copied into the release directory",
);
assert(
  linux.includes("tool package -name jftrade"),
  "Linux package name must remain lowercase for Debian compatibility",
);
assert(
  common.includes("update build-assets") && common.includes("generate icons"),
  "common task does not use Wails build asset generation",
);
assert(
  common.includes("Copyright (C) 2026 JFTrade Contributors"),
  "desktop metadata does not use the project copyright notice",
);
assert(
  releaseWorkflow.includes("release/LICENSE") &&
    releaseWorkflow.includes("release/THIRD-PARTY-NOTICES.md"),
  "GitHub Release does not publish the legal notice files",
);
assert(
  releaseWorkflow.includes("sha256sum > SHA256SUMS") &&
    releaseWorkflow.includes("gh release upload") &&
    releaseWorkflow.includes("gh release delete-asset") &&
    releaseWorkflow.includes("release/*"),
  "GitHub Release does not clean legacy assets, checksum and upload every release asset",
);
assert(
  releaseWorkflow.includes('base="JFTrade-${version}-linux-x64"') &&
    releaseWorkflow.includes("outputs.rpm") &&
    releaseWorkflow.includes("sudo apt-get install -y rpm squashfs-tools") &&
    releaseWorkflow.includes("unsquashfs -s -offset") &&
    releaseWorkflow.includes('rpm -qpR "$rpm"'),
  "GitHub Release does not verify the canonical Linux package set and AppImage",
);
assert(
  !releaseWorkflow.includes("linux-amd64/*.AppImage") &&
    !releaseWorkflow.includes("linux-amd64/*.deb") &&
    !releaseWorkflow.includes(
      "dist/desktop-release/linux-amd64/jftrade-desktop-linux-amd64",
    ),
  "GitHub Release must use exact Linux package paths without a raw binary",
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
  nfpm.includes("license: AGPL-3.0-only") &&
    !nfpm.includes("LicenseRef-Proprietary"),
  "Linux package metadata is not AGPL-3.0-only",
);
assert(
  nfpm.includes("/usr/share/licenses/jftrade/LICENSE") &&
    nfpm.includes("/usr/share/licenses/jftrade/THIRD-PARTY-NOTICES.md"),
  "Linux package does not carry the project license and third-party notices",
);
assert(
  nfpm.includes("libgtk-3-0") && nfpm.includes("libwebkit2gtk-4.1-0"),
  "Linux package dependencies do not match GTK3 build",
);
assert(
  !nfpm.includes("archlinux"),
  "Linux package metadata must not retain an Arch override",
);
