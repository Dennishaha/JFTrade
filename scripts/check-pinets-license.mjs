import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { readFileSync } from "node:fs";

const require = createRequire(import.meta.url);
const entry = require.resolve("pinets");
const pkg = JSON.parse(readFileSync(join(dirname(dirname(entry)), "package.json"), "utf8"));

if (pkg.name !== "pinets") {
  throw new Error(`expected pinets package, got ${pkg.name}`);
}
if (pkg.version !== "0.9.26") {
  throw new Error(`pinets version = ${pkg.version}, want 0.9.26`);
}
if (pkg.license !== "AGPL-3.0-only") {
  throw new Error(`pinets license = ${pkg.license}, want AGPL-3.0-only`);
}

console.log(`pinets ${pkg.version} license ${pkg.license} allowed`);
