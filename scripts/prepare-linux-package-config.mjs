import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const [templatePath, outputPath, version, arch, binary, desktopFile, icon] =
  process.argv.slice(2);
if (
  ![templatePath, outputPath, version, arch, binary, desktopFile, icon].every(
    Boolean,
  )
) {
  fail(
    "Usage: prepare-linux-package-config.mjs <template> <output> <version> <arch> <binary> <desktop> <icon>",
  );
}
if (!/^\d+\.\d+\.\d+$/.test(version))
  fail(`Invalid package version: ${version}`);
if (!/^(amd64|arm64)$/.test(arch))
  fail(`Invalid package architecture: ${arch}`);

let config = fs.readFileSync(templatePath, "utf8");
for (const [token, value] of Object.entries({
  __VERSION__: version,
  __ARCH__: arch,
  __BINARY__: path.resolve(binary),
  __DESKTOP_FILE__: path.resolve(desktopFile),
  __ICON__: path.resolve(icon),
})) {
  config = config.replaceAll(token, value);
}
if (/__[A-Z_]+__/.test(config))
  fail("Linux package config contains unresolved placeholders");
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, config, "utf8");

function fail(message) {
  console.error(message);
  process.exit(1);
}
