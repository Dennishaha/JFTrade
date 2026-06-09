import MarkdownIt from "markdown-it";

export function useADKMarkdownRenderer(options: {
  enableMermaid?: boolean;
} = {}) {
  const markdown = new MarkdownIt({
    html: false,
    linkify: true,
    breaks: true,
  });

  if (options.enableMermaid) {
    const defaultFenceRenderer = markdown.renderer.rules.fence;
    markdown.renderer.rules.fence = (tokens, idx, rendererOptions, env, self) => {
      const token = tokens[idx];
      if (!token) {
        return defaultFenceRenderer
          ? defaultFenceRenderer(tokens, idx, rendererOptions, env, self)
          : self.renderToken(tokens, idx, rendererOptions);
      }
      const language = token.info.trim().split(/\s+/)[0];
      if (language === "mermaid") {
        return `<div class="mermaid">${markdown.utils.escapeHtml(token.content.trim())}</div>`;
      }
      if (defaultFenceRenderer) {
        return defaultFenceRenderer(tokens, idx, rendererOptions, env, self);
      }
      return self.renderToken(tokens, idx, rendererOptions);
    };
  }

  function renderMarkdown(content: string): string {
    return markdown
      .render(content)
      .replace(/<a /g, '<a target="_blank" rel="noopener noreferrer" ');
  }

  return {
    markdown,
    renderMarkdown,
  };
}
