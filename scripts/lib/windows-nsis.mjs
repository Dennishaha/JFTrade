import path from "node:path";

export function resolveWindowsNSISInvocation({
  arch,
  installer,
  makensis = "makensis",
  rootDir,
}) {
  if (!new Set(["amd64", "arm64"]).has(arch)) {
    throw new Error(`Unsupported Windows architecture: ${arch}`);
  }
  if (!installer || path.basename(installer) !== installer) {
    throw new Error(`Invalid Windows installer filename: ${installer}`);
  }

  const releaseDir = path.resolve(
    rootDir,
    "dist",
    "desktop-release",
    `windows-${arch}`,
  );
  const binary = path.resolve(
    rootDir,
    "dist",
    "desktop",
    `windows-${arch}`,
    `jftrade-desktop-windows-${arch}.exe`,
  );
  const license = path.resolve(rootDir, "LICENSE");
  const thirdPartyNotices = path.resolve(
    rootDir,
    "docs",
    "legal",
    "third-party-notices.md",
  );

  return {
    command: String(makensis || "makensis").trim() || "makensis",
    args: [
      "/DWAILS_INSTALL_SCOPE=user",
      "/DREQUEST_EXECUTION_LEVEL=user",
      `/DARG_WAILS_${arch === "amd64" ? "AMD64" : "ARM64"}_BINARY=${binary}`,
      `/DJFTRADE_LICENSE_FILE=${license}`,
      `/DJFTRADE_THIRD_PARTY_NOTICES_FILE=${thirdPartyNotices}`,
      `/DOUTPUT_EXE=${path.join(releaseDir, installer)}`,
      "project.nsi",
    ],
    cwd: path.join(releaseDir, "nsis"),
  };
}
