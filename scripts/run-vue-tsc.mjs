import { createRequire } from "node:module";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { tscPath } from "./lib/typescript6.mjs";

const require = createRequire(import.meta.url);
const webRoot = resolve(fileURLToPath(new URL("../apps/web/", import.meta.url)));
const vueTscPath = require.resolve("vue-tsc", { paths: [webRoot] });

require(vueTscPath).run(tscPath);
