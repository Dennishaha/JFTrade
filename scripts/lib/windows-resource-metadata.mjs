const desktopAssemblyIdentityVersion =
  /(<assemblyIdentity\b(?=[^>]*\bname="com\.jftrade\.desktop")(?=[^>]*\bversion=")[^>]*\bversion=")[^"]+(")/;

export function withWindowsDesktopManifestVersion(manifest, numericVersion) {
  const version = String(numericVersion || "").trim();
  if (!/^\d+\.\d+\.\d+$/.test(version)) {
    throw new Error(`Invalid Windows desktop version: ${numericVersion}`);
  }

  const updated = manifest.replace(
    desktopAssemblyIdentityVersion,
    `$1${version}.0$2`,
  );
  if (updated === manifest) {
    throw new Error(
      "Windows manifest is missing the com.jftrade.desktop assembly identity.",
    );
  }
  return updated;
}
