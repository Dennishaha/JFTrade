import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

describe("strategy authoring docs", () => {
  it("describes Pine snippet as the fallback path instead of legacy codeBlock", () => {
    const markdown = readFileSync(
      resolve(process.cwd(), "../../docs/frontend/strategy-authoring.md"),
      "utf8",
    );

    expect(markdown).toContain("`pineSnippet`");
    expect(markdown).toContain("Pine 片段");
    expect(markdown).not.toMatch(/降级为\s*`?codeBlock`?/);
    expect(markdown).not.toMatch(/`codeBlock`\s*兜底/);
  });
});
