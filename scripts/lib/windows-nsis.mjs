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

  return {
    command: String(makensis || "makensis").trim() || "makensis",
    args: [
      "/DWAILS_INSTALL_SCOPE=user",
      "/DREQUEST_EXECUTION_LEVEL=user",
      `/DARG_WAILS_${arch === "amd64" ? "AMD64" : "ARM64"}_BINARY=${binary}`,
      `/DOUTPUT_EXE=${path.join(releaseDir, installer)}`,
      "project.nsi",
    ],
    cwd: path.join(releaseDir, "nsis"),
  };
}
