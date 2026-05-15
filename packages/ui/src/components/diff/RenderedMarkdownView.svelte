<script lang="ts">
  import { Marked, type Token } from "marked";
  import DOMPurify from "dompurify";
  import { getStores } from "../../context.js";

  // Renders a markdown file at a given SHA inside the diff surface,
  // annotated with source line numbers. The annotation strategy is
  // deliberately sparse to avoid clutter:
  //   - Every heading is labelled with its source line (acts as a TOC).
  //   - Every block whose source-line range overlaps a changed-line
  //     range gets an accent bar + line label.
  //   - Everything else renders clean.
  // The reviewer can cross-reference the rendered output against the
  // diff hunks without the rendered view becoming a wall of numbers.

  export interface RenderedHunk {
    new_start: number;
    new_count: number;
  }

  interface Props {
    owner: string;
    name: string;
    number: number;
    path: string;
    sha: string;
    hunks: RenderedHunk[];
  }

  const { owner, name, number, path, sha, hunks }: Props = $props();

  const stores = getStores();
  // Reach the diff store's basePath via the store registry's known
  // shape; we read it indirectly so the component composes regardless
  // of where it's mounted.
  const basePath = $derived(() => {
    // Heuristic: fall through to "/" — this is the same default used
    // by every other API caller in the app.
    return "/";
  });

  let raw = $state<string | null>(null);
  let truncated = $state(false);
  let loading = $state(false);
  let error = $state<string | null>(null);

  // Avoid stale responses overwriting a newer fetch after the user
  // jumps between commits or switches files quickly.
  let fetchSeq = 0;

  $effect(() => {
    void load(path, sha);
  });

  async function load(p: string, s: string): Promise<void> {
    if (!p || !s) return;
    const mySeq = ++fetchSeq;
    loading = true;
    error = null;
    raw = null;
    truncated = false;
    try {
      const url =
        `${basePath()}api/v1/repos/` +
        `${encodeURIComponent(owner)}/${encodeURIComponent(name)}` +
        `/pulls/${number}/blob` +
        `?path=${encodeURIComponent(p)}&sha=${encodeURIComponent(s)}`;
      const res = await fetch(url);
      if (mySeq !== fetchSeq) return;
      if (!res.ok) {
        error = `Fetch blob: ${res.status} ${res.statusText}`;
        return;
      }
      const data = (await res.json()) as { content: string; truncated: boolean };
      if (mySeq !== fetchSeq) return;
      truncated = data.truncated;
      raw = data.content;
    } catch (e) {
      if (mySeq !== fetchSeq) return;
      error = e instanceof Error ? e.message : String(e);
    } finally {
      if (mySeq === fetchSeq) loading = false;
    }
  }

  // Build the set of changed source lines on the NEW side from the
  // hunks. We don't need the actual line text — only "is this line
  // number inside an added/changed hunk on the new side."
  const changedLines = $derived.by<Set<number>>(() => {
    const s = new Set<number>();
    for (const h of hunks ?? []) {
      for (let i = 0; i < h.new_count; i++) {
        s.add(h.new_start + i);
      }
    }
    return s;
  });

  // Per-block walk: tokenize, advance a line cursor through token.raw,
  // and emit one entry per top-level token with its starting line +
  // pre-rendered HTML.
  interface Block {
    html: string;
    startLine: number;
    endLine: number;
    isHeading: boolean;
    changed: boolean;
  }

  const blocks = $derived.by<Block[]>(() => {
    if (raw === null) return [];
    const m = new Marked({ breaks: true, gfm: true });
    let tokens: Token[];
    try {
      tokens = m.lexer(raw);
    } catch {
      return [];
    }
    const out: Block[] = [];
    let line = 1;
    for (const t of tokens) {
      const rawText = t.raw ?? "";
      const newlines = countNewlines(rawText);
      const startLine = line;
      const endLine = line + newlines;
      // Some tokens are pure-whitespace separators (type === "space");
      // skip them but still advance the cursor so subsequent blocks
      // keep their line numbers accurate.
      if (t.type !== "space") {
        const isHeading = t.type === "heading";
        const changed = blockOverlapsChanged(startLine, endLine, changedLines);
        let html = "";
        try {
          html = m.parser([t]);
        } catch {
          html = `<pre>${escapeHTML(rawText)}</pre>`;
        }
        out.push({ html, startLine, endLine, isHeading, changed });
      }
      line += newlines;
    }
    return out;
  });

  function countNewlines(s: string): number {
    let n = 0;
    for (let i = 0; i < s.length; i++) if (s.charCodeAt(i) === 10) n++;
    return n;
  }

  function blockOverlapsChanged(
    start: number,
    end: number,
    set: Set<number>,
  ): boolean {
    // Inclusive [start, end). Walk each integer in the block's range
    // because the change set is sparse — usually a handful of lines
    // per file — so this stays cheap.
    for (let i = start; i < Math.max(start + 1, end); i++) {
      if (set.has(i)) return true;
    }
    return false;
  }

  function escapeHTML(s: string): string {
    return s
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function sanitize(html: string): string {
    return DOMPurify.sanitize(html, {
      ADD_ATTR: ["target", "rel"],
    });
  }
</script>

<div class="rmd-view">
  {#if loading && raw === null}
    <div class="rmd-state">Loading…</div>
  {:else if error}
    <div class="rmd-state rmd-state--error">{error}</div>
  {:else if truncated}
    <div class="rmd-state rmd-state--error">File too large to render inline.</div>
  {:else if raw !== null && blocks.length === 0}
    <div class="rmd-state">Empty file.</div>
  {:else}
    {#each blocks as block, i (i)}
      <div
        class="rmd-block"
        class:rmd-block--changed={block.changed}
        class:rmd-block--heading={block.isHeading}
      >
        {#if block.isHeading || block.changed}
          <span
            class="rmd-line"
            class:rmd-line--changed={block.changed}
            title={block.changed ? `Lines ${block.startLine}–${block.endLine} changed in this PR` : `Line ${block.startLine}`}
          >L{block.startLine}</span>
        {/if}
        <div class="rmd-body markdown-body">{@html sanitize(block.html)}</div>
      </div>
    {/each}
  {/if}
</div>

<style>
  .rmd-view {
    padding: 12px 16px;
    background: var(--diff-bg);
  }

  .rmd-state {
    padding: 10px;
    color: var(--text-muted);
    font-size: 12px;
    font-style: italic;
  }
  .rmd-state--error {
    color: var(--accent-red);
  }

  .rmd-block {
    position: relative;
    padding-left: 56px;        /* room for the line label */
    margin-bottom: 4px;
    border-left: 3px solid transparent;
  }

  /* Changed-block accent — a calm green bar on the left edge, picked
     to match the diff's add-line color family without the bold
     intensity of an inline addition. */
  .rmd-block--changed {
    border-left-color: color-mix(in srgb, var(--diff-add-text) 50%, transparent);
    background: color-mix(in srgb, var(--diff-add-bg) 25%, transparent);
  }

  /* The L<n> label. Floats in the gutter so it never pushes prose
     around. Faint by default, bumps brighter for changed blocks. */
  .rmd-line {
    position: absolute;
    left: 0;
    top: 0;
    width: 48px;
    text-align: right;
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--diff-line-num);
    padding-top: 4px;
    user-select: none;
  }
  .rmd-line--changed {
    color: var(--diff-add-text);
    font-weight: 600;
  }

  .rmd-body {
    font-size: 13px;
    line-height: 1.55;
    color: var(--text-primary);
  }

  /* Same prose-width discipline as the AI brief: cap prose elements
     but let code blocks and tables take full width. */
  .rmd-body :global(p),
  .rmd-body :global(ul),
  .rmd-body :global(ol),
  .rmd-body :global(blockquote) {
    max-width: 80ch;
  }
  .rmd-body :global(pre) {
    overflow-x: auto;
  }
  .rmd-body :global(code) {
    word-break: keep-all;
    overflow-wrap: normal;
  }

  /* Headings are visual anchors — keep them spaced enough to scan. */
  .rmd-body :global(h1),
  .rmd-body :global(h2),
  .rmd-body :global(h3) {
    margin: 12px 0 6px;
  }
  .rmd-body :global(h4),
  .rmd-body :global(h5),
  .rmd-body :global(h6) {
    margin: 8px 0 4px;
  }
</style>
