#!/usr/bin/env node
import { runBun } from "./lib/bun.mjs";

const result = runBun(process.argv.slice(2));
process.exit(result.status ?? 0);
