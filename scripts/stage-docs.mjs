import fs from "node:fs/promises";
import path from "node:path";

const rootDir = process.cwd();
const docsDistDir = path.join(rootDir, "docs", ".vitepress", "dist");
const stagedDocsDir = path.join(rootDir, "apps", "web", "dist", "docs");

await fs.rm(stagedDocsDir, { force: true, recursive: true });
await fs.mkdir(path.dirname(stagedDocsDir), { recursive: true });
await fs.cp(docsDistDir, stagedDocsDir, { recursive: true });
