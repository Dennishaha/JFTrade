import path from "node:path";

export const linuxPackageFormats = Object.freeze(["appimage", "deb", "rpm"]);

const linuxArtifactExtensions = Object.freeze({
  appimage: "AppImage",
  deb: "deb",
  rpm: "rpm",
});

const linuxArtifactArchitectures = Object.freeze({
  amd64: "x64",
  arm64: "arm64",
});

export function linuxReleaseArtifactName(version, arch, format) {
  if (!/^\d+\.\d+\.\d+$/.test(version))
    throw new Error(`Invalid Linux release version: ${version}`);
  const artifactArch = linuxArtifactArchitectures[arch];
  if (!artifactArch)
    throw new Error(`Unsupported Linux release architecture: ${arch}`);
  const extension = linuxArtifactExtensions[format];
  if (!extension) throw new Error(`Unsupported Linux package format: ${format}`);
  return `JFTrade-${version}-linux-${artifactArch}.${extension}`;
}

export function linuxReleaseArtifactPaths(dir, version, arch, format = "") {
  const formats = format ? [format] : linuxPackageFormats;
  return formats.map((entry) =>
    path.join(dir, linuxReleaseArtifactName(version, arch, entry)),
  );
}

export function linuxPackageExtension(format) {
  const extension = linuxArtifactExtensions[format];
  if (!extension) throw new Error(`Unsupported Linux package format: ${format}`);
  return extension;
}
