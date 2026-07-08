import fs from "node:fs";
import path from "node:path";

export const macBundleIdentifier = "com.jftrade.desktop";
export const macBundleName = "JFTrade";
export const macBundleExecutable = "JFTrade";
export const macBundleIconFile = "icons.icns";

export function writeMacAppBundle(appPath, binaryPath) {
  const rootDir = path.resolve(import.meta.dirname, "../..");
  const iconSourcePath = path.join(rootDir, "build", "desktop", "darwin", macBundleIconFile);
  const contentsDir = path.join(appPath, "Contents");
  const macOSDir = path.join(contentsDir, "MacOS");
  const resourcesDir = path.join(contentsDir, "Resources");
  const executablePath = path.join(macOSDir, macBundleExecutable);

  fs.rmSync(appPath, { recursive: true, force: true });
  fs.mkdirSync(macOSDir, { recursive: true });
  fs.mkdirSync(resourcesDir, { recursive: true });
  fs.copyFileSync(binaryPath, executablePath);
  fs.chmodSync(executablePath, 0o755);
  if (fs.existsSync(iconSourcePath)) {
    fs.copyFileSync(iconSourcePath, path.join(resourcesDir, macBundleIconFile));
  }
  fs.writeFileSync(path.join(contentsDir, "Info.plist"), macInfoPlist(), "utf8");
  fs.writeFileSync(path.join(contentsDir, "PkgInfo"), "APPL????", "utf8");

  return executablePath;
}

function macInfoPlist() {
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleExecutable</key>
  <string>${macBundleExecutable}</string>
  <key>CFBundleIdentifier</key>
  <string>${macBundleIdentifier}</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>${macBundleName}</string>
  <key>CFBundleDisplayName</key>
  <string>${macBundleName}</string>
  <key>CFBundleIconFile</key>
  <string>${macBundleIconFile}</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>0.0.0</string>
  <key>CFBundleVersion</key>
  <string>0.0.0</string>
  <key>LSMinimumSystemVersion</key>
  <string>11.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
`;
}
