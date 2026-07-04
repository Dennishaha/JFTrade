import { describe, expect, it } from "vitest";
import { createMemoryHistory } from "vue-router";

import { createConsoleRouter } from "../src/router";

describe("console router", () => {
  it("exposes ADK agents and workflows as distinct paths", () => {
    const router = createConsoleRouter(createMemoryHistory());

    expect(router.resolve("/adk/agents").matched).toHaveLength(1);
    expect(router.resolve("/adk/workflows").matched).toHaveLength(1);
    expect(router.resolve("/risk").matched).toHaveLength(1);
    expect(router.resolve("/adk").matched).toHaveLength(0);
    expect(router.resolve("/adk?view=chat").matched).toHaveLength(0);
    expect(router.resolve("/adk?view=workflows").matched).toHaveLength(0);
  });
});
