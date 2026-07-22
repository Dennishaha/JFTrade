import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["src/**/*.test.ts"],
    coverage: {
      provider: "v8",
      // main.ts is owned by the process-level Pine worker smoke test. index.ts
      // is a barrel and types.ts only carries public type declarations plus
      // immutable identifiers, so neither should distort runtime coverage.
      include: ["src/**/*.ts"],
      exclude: ["src/**/*.test.ts", "src/main.ts", "src/index.ts", "src/types.ts"],
      thresholds: {
        statements: 90,
        branches: 80,
        functions: 95,
        lines: 90,
      },
    },
  },
});
