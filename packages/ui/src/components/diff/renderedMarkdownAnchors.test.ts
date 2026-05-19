import { describe, it, expect } from "vitest";
import { wrapProseBlock, wrapCodeBlock } from "./renderedMarkdownAnchors";

describe("wrapProseBlock", () => {
  it("wraps each source line in an anchor span using the provided inline parser", () => {
    const inline = (s: string): string => `<em>${s}</em>`;
    const out = wrapProseBlock("foo\nbar baz", 10, "RIGHT", inline);
    expect(out).toBe(
      `<span class="rmd-anchor" data-anchor-line="10" data-anchor-side="RIGHT"><em>foo</em></span>` +
      ` ` +
      `<span class="rmd-anchor" data-anchor-line="11" data-anchor-side="RIGHT"><em>bar baz</em></span>`,
    );
  });

  it("uses LEFT side when requested (for deleted files)", () => {
    const out = wrapProseBlock("x", 5, "LEFT", (s) => s);
    expect(out).toContain(`data-anchor-side="LEFT"`);
    expect(out).toContain(`data-anchor-line="5"`);
  });
});

describe("wrapCodeBlock", () => {
  it("preserves newlines as the join character and HTML-escapes each line", () => {
    const out = wrapCodeBlock("a < b\nc > d", 20, "RIGHT");
    expect(out).toBe(
      `<span class="rmd-anchor" data-anchor-line="20" data-anchor-side="RIGHT">a &lt; b</span>` +
      `\n` +
      `<span class="rmd-anchor" data-anchor-line="21" data-anchor-side="RIGHT">c &gt; d</span>`,
    );
  });

  it("returns an empty string for empty code", () => {
    expect(wrapCodeBlock("", 1, "RIGHT")).toBe("");
  });
});
