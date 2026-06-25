import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

describe("strategy authoring docs", () => {
  it("describes unsupported Pine visual sync as a failure instead of snippet fallback", () => {
    const markdown = readFileSync(
      resolve(process.cwd(), "../../docs/frontend/strategy-authoring.md"),
      "utf8",
    );

    expect(markdown).toContain("无法标准化");
    expect(markdown).toContain("返回 `ok:false`");
    expect(markdown).not.toContain("`pineSnippet`");
    expect(markdown).not.toMatch(/降级为\s*`?codeBlock`?/);
    expect(markdown).not.toMatch(/`codeBlock`\s*兜底/);
  });
});
