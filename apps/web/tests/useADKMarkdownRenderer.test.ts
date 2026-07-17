import { describe, expect, it } from "vitest";

import { useADKMarkdownRenderer } from "../src/composables/useADKMarkdownRenderer";

describe("ADK markdown renderer", () => {
  it("escapes raw HTML while preserving safe links and line breaks", () => {
    const { renderMarkdown } = useADKMarkdownRenderer();
    const rendered = renderMarkdown("<script>alert(1)</script>\nhttps://jftrade.example/docs");

    expect(rendered).toContain("&lt;script&gt;alert(1)&lt;/script&gt;<br>");
    expect(rendered).toContain('target="_blank" rel="noopener noreferrer"');
    expect(rendered).toContain('href="https://jftrade.example/docs"');
  });

  it("renders Mermaid blocks as sanitized chart containers only when enabled", () => {
    const source = "```mermaid\ngraph TD\n  A[&lt;unsafe&gt;] --> B\n```";
    const enabled = useADKMarkdownRenderer({ enableMermaid: true }).renderMarkdown(source);
    const disabled = useADKMarkdownRenderer().renderMarkdown(source);

    expect(enabled).toContain('<div class="mermaid">graph TD');
    expect(enabled).toContain("&amp;lt;unsafe&amp;gt;");
    expect(disabled).toContain("<pre><code class=\"language-mermaid\">");
  });

  it("falls back to the default fence renderer for ordinary code", () => {
    const { renderMarkdown } = useADKMarkdownRenderer({ enableMermaid: true });
    const rendered = renderMarkdown("```ts\nconst value = 1;\n```");

    expect(rendered).toContain("<pre><code class=\"language-ts\">");
    expect(rendered).toContain("const value = 1;");
  });
});
