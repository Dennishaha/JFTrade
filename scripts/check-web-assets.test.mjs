#!/usr/bin/env node
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  stagedDocumentationRequirements,
  verifyStagedWebAssets,
} from "./check-web-assets.mjs";

function write(root, relative, content) {
  const target = join(root, relative);
  mkdirSync(join(target, ".."), { recursive: true });
  writeFileSync(target, content);
  return content;
}

function documentationPage(relative) {
  return stagedDocumentationRequirements[relative].join("\n");
}

function digestOf(root, relative, override) {
  const content = override?.(relative);
  const bytes = content === undefined ? readFileSync(join(root, relative)) : Buffer.from(content);
  return createHash("sha256").update(bytes).digest("hex");
}

function manifestFor(root, relativePaths, { override } = {}) {
  return {
    schemaVersion: "jftrade.web-assets.v1",
    files: relativePaths
      .map((relative) => ({
        path: relative,
        sha256: digestOf(root, relative, override),
      }))
      .sort((left, right) => left.path.localeCompare(right.path)),
  };
}

const workspace = mkdtempSync(join(tmpdir(), "jftrade-web-assets-"));
const root = join(workspace, "dist");
const manifestPath = join(workspace, "manifest.json");
try {
  const staged = [
    "index.html",
    "runtime-config.js",
    "assets/index-a.js",
    "assets/_.contribution-a.js",
    "docs/legal/license.html",
    "docs/legal/third-party-notices.html",
  ];
  write(root, "index.html", "<html>JFTrade</html>\n");
  write(root, "runtime-config.js", "window.__JFTRADE_RUNTIME_CONFIG__ = {};\n");
  write(root, "assets/index-a.js", "export const app = true;\n");
  write(root, "assets/_.contribution-a.js", "export const shared = true;\n");
  write(root, "docs/legal/license.html", documentationPage("docs/legal/license.html"));
  write(
    root,
    "docs/legal/third-party-notices.html",
    documentationPage("docs/legal/third-party-notices.html"),
  );
  const writeManifest = (value) => writeFileSync(manifestPath, `${JSON.stringify(value, null, 2)}\n`);

  writeManifest(manifestFor(root, staged));
  assert.deepEqual(verifyStagedWebAssets({ distRoot: root, manifestPath }), []);

  writeManifest(manifestFor(root, staged.filter((path) => path !== "assets/_.contribution-a.js")));
  assert.match(
    verifyStagedWebAssets({ distRoot: root, manifestPath }).join("\n"),
    /leading underscore is not packaged: assets\/_\.contribution-a\.js/,
  );

  writeManifest(
    manifestFor(root, staged, {
      override: (relative) =>
        relative === "assets/index-a.js" ? Buffer.from("tampered bytes") : undefined,
    }),
  );
  assert.match(
    verifyStagedWebAssets({ distRoot: root, manifestPath }).join("\n"),
    /manifest digest differs for the staged asset: assets\/index-a\.js/,
  );

  writeManifest(manifestFor(root, staged));
  write(root, "docs/legal/license.html", "");
  assert.match(
    verifyStagedWebAssets({ distRoot: root, manifestPath }).join("\n"),
    /documentation page is empty: docs\/legal\/license\.html/,
  );
  rmSync(join(root, "docs/legal/license.html"));
  assert.match(
    verifyStagedWebAssets({ distRoot: root, manifestPath }).join("\n"),
    /documentation page is missing: docs\/legal\/license\.html/,
  );

  write(root, "docs/legal/license.html", documentationPage("docs/legal/license.html"));
  write(root, "assets/legacy-a.js", "const removed = 'pkg/strategy/pineruntime';\n");
  writeManifest(manifestFor(root, [...staged, "assets/legacy-a.js"]));
  assert.match(
    verifyStagedWebAssets({ distRoot: root, manifestPath }).join("\n"),
    /staged text asset assets\/legacy-a\.js still references pkg\/strategy\/pineruntime/,
  );
  rmSync(join(root, "assets/legacy-a.js"));

  write(root, "docs/reference/bbgo-doc/benchmark.html", "BenchmarkRunExecutesPineGoldenMatrix\n");
  writeManifest(manifestFor(root, [...staged, "docs/reference/bbgo-doc/benchmark.html"]));
  assert.deepEqual(verifyStagedWebAssets({ distRoot: root, manifestPath }), []);
  rmSync(join(root, "docs/reference"), { recursive: true });

  writeManifest(
    manifestFor(root, [...staged, "assets/absent-a.js"], {
      override: (relative) => (relative === "assets/absent-a.js" ? "declared but absent\n" : undefined),
    }),
  );
  assert.match(
    verifyStagedWebAssets({ distRoot: root, manifestPath }).join("\n"),
    /manifest declares an asset that is not staged: assets\/absent-a\.js/,
  );
} finally {
  rmSync(workspace, { recursive: true, force: true });
}

assert.match(
  verifyStagedWebAssets({ distRoot: "/nonexistent-dist", manifestPath: "/nonexistent-manifest.json" }).join("\n"),
  /staged web assets directory is unreadable:/,
);

const emptyWorkspace = mkdtempSync(join(tmpdir(), "jftrade-web-assets-empty-"));
try {
  mkdirSync(join(emptyWorkspace, "dist"));
  assert.match(
    verifyStagedWebAssets({
      distRoot: join(emptyWorkspace, "dist"),
      manifestPath: join(emptyWorkspace, "manifest.json"),
    }).join("\n"),
    /staged web manifest is unreadable:/,
  );
} finally {
  rmSync(emptyWorkspace, { recursive: true, force: true });
}

console.log("staged web asset checks behave as expected");
