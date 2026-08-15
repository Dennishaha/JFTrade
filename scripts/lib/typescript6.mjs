import { createRequire } from "node:module";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const webRoot = resolve(fileURLToPath(new URL("../../apps/web/", import.meta.url)));
const typescript6Package = require.resolve("@typescript/typescript6/package.json", {
  paths: [webRoot],
});
const typescript6Root = dirname(typescript6Package);

export const tscPath = require.resolve("@typescript/old/lib/tsc", {
  paths: [typescript6Root],
});
export default require(require.resolve("@typescript/old", { paths: [typescript6Root] }));
