import { createRequire } from "node:module";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const workerBundleBanner =
  'import { createRequire as __jftradeCreateRequire } from "node:module"; const require = __jftradeCreateRequire(import.meta.url);';

export async function buildPineWorkerBundle({ rootDir, workerEntry, outFile }) {
  const rolldownBuild = await loadRolldownBuild(rootDir);
  await rolldownBuild({
    input: workerEntry,
    cwd: rootDir,
    platform: "node",
    logLevel: "info",
    output: {
      file: outFile,
      format: "es",
      minify: false,
      codeSplitting: false,
      banner: workerBundleBanner,
    },
  });
}

export function dryRunPineWorkerBundleCommand({ workerEntry, outFile }) {
  return `DRY RUN rolldown ${workerEntry} --platform node --format esm --file ${outFile} --bundle-dependencies`;
}

async function loadRolldownBuild(rootDir) {
  const requireFromWorker = createRequire(join(rootDir, "workers/pineworker/package.json"));
  const rolldownPath = requireFromWorker.resolve("rolldown");
  const rolldown = await import(pathToFileURL(rolldownPath).href);
  return rolldown.build;
}
