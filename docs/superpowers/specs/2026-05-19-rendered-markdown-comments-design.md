# Rendered-Markdown Comments and AI Questions

Date: 2026-05-19

Allow reviewers to leave comments and start AI Ask threads from inside the rendered-markdown view of a `.md` file, with full bidirectional parity against the existing diff view.

## Goal

Today the rendered-markdown view (`packages/ui/src/components/diff/RenderedMarkdownView.svelte`) is read-only. A reviewer who notices something in a rendered `.md` has to switch to the diff view to comment or ask Claude about it. This design adds anchored comments and AI threads to the rendered view as a second surface over the same comment store the diff view writes to.

## Anchor model

Comments and AI threads continue to be stored as `{startLine, endLine, side, sha, path}` — the existing diff anchor shape. Nothing changes in the DB, the comments API, or the AI-threads API. The rendered view is a new read/write surface over that same store.

Side selection mirrors the existing `renderedSHA` / `renderedPath` logic in `DiffFile.svelte:44-62`:

- Non-deleted files: `side = "RIGHT"` (the rendered view shows the new content).
- Deleted files: `side = "LEFT"` (the rendered view falls back to `old_path` at the merge-base).

## Anchor precision

Per-line within the containing block.

Selecting any text inside a 5-line paragraph (e.g. source lines L42–L46) resolves to a source-line subrange (e.g. L43–L44) determined by which lines the selection actually crosses. This matches the diff view's per-line `{startLine, endLine}` shape rather than coarsening to whole-block anchors.

## Render pipeline

A new `Marked` custom renderer in `RenderedMarkdownView.svelte` overrides block-emitting methods (paragraph, heading, list_item, blockquote, code). For each block token:

1. Take `token.raw`, split on `\n`, compute `(blockStartLine + offset)` for each segment.
2. For prose blocks (paragraph, heading, list_item, blockquote): run each segment through `marked.parseInline()` to render inline tokens (links, emphasis, code spans). For code blocks: leave each segment as a literal text node (no inline parsing).
3. Wrap each rendered segment in `<span class="rmd-anchor" data-anchor-line="N" data-anchor-side="RIGHT">…</span>` (or `"LEFT"` for deleted files).
4. Concatenate segments with the block type's natural join character: a space for prose blocks (markdown's soft-wrap behavior, so consecutive source lines render as flowing text), a newline for code blocks (preserving the visible line breaks `<pre>` renders).
5. Pass the joined inner HTML to the standard block wrapper (`<p>`, `<li>`, `<pre><code>`, etc.).

Existing `headingLineByIdx` and changed-block detection (`changedIndexes` / `rmd-changed`) stay intact — the per-line spans nest inside the same block elements without interfering.

Markdown features that cross source-line boundaries within a paragraph (multi-line links, soft-wrapped emphasis) render slightly less faithfully because we split before inline parsing. Rare in practice; accepted trade.

## Selection → composer flow

A new `computeRenderedSelectionRange()` function mirrors `DiffFile.svelte:152` (`computeSelectionRange()`):

- Walks up from `window.getSelection()`'s `anchorNode` and `focusNode` to the nearest ancestor with `data-anchor-line`.
- Reads `data-anchor-line` / `data-anchor-side` from each end.
- Sorts to `[startLine, endLine]` and packages as `{startLine, endLine, side}`.

The composer is the existing `DiffComposer.svelte` component, used unchanged. It already accepts `initialValue`, `anchor`, `onsave`, `oncancel` props. It positions inline below the last block whose source range overlaps the selection.

Both regular comments and AI Ask flow through the same composer — same as the diff view. No separate UI path.

## Existing-thread display

Comment threads and AI threads whose anchor's `[startLine, endLine]` overlaps a rendered block's source range are displayed inline AFTER that block. Reuses the existing `ReviewCommentCard.svelte` (and equivalent AI-thread card) component unchanged.

When two threads anchor to overlapping ranges that share a block, both render below the same block in anchor order.

## Stale anchors

A thread whose `[startLine, endLine]` doesn't overlap any rendered block in the current view is "outdated". Mirror the diff view's existing outdated-comment banner at the top of the rendered view:

> N outdated review comment(s) on this file — view on GitHub to see them

(For local worktrees there's no GitHub URL; the banner there shows the count without the trailing call to action. Existing infrastructure for diffs against local worktrees already handles this distinction.)

## Components touched / added

**Modified:**
- `packages/ui/src/components/diff/RenderedMarkdownView.svelte` — custom marked block renderer, selection handler, thread embedding, outdated banner.

**Reused as-is:**
- `packages/ui/src/components/diff/DiffComposer.svelte`
- `packages/ui/src/components/diff/ReviewCommentCard.svelte`
- Existing comment store, AI-thread store, and API client.

**No backend changes.** The comments and AI-threads endpoints already accept `{path, startLine, endLine, side, sha}`. The local-source dispatch layer already routes these for local worktrees (per recent commits `0d3412f` and `5fe352d`).

## Testing strategy

**Unit (Svelte component tests):**
- Per-line span injection for paragraphs, headings, list items, code blocks, blockquotes.
- Selection-to-anchor resolution for selections that start mid-line, end mid-line, cross two source lines, cross two adjacent blocks.
- Outdated-banner triggers when a thread anchor falls outside the rendered block ranges.

**E2E (against real SQLite + filesystem):**
- Open the rendered markdown view on a local worktree, select text within a paragraph, submit a comment, switch to the diff view, verify the comment appears at the matching source line.
- Reverse: create a comment in the diff view, switch to the rendered view, verify the comment renders below the matching block.
- Same flow for an AI Ask thread.

**Edge cases:**
- Multi-line link selection (verify graceful degradation given the inline-parse-after-split limitation).
- Code-block selection — each line in a fenced code block is its own anchor span.
- Selection that begins in one block and ends in another (cross-block range).
- Deleted file: rendered view shows old content, comment anchors with `side: LEFT`.

## Out of scope

- Sub-line precision (character offsets within a line).
- Annotations on the LEFT side of a non-deleted file (the rendered view shows only the new content for non-deleted files).
- Margin-gutter placement of threads — could revisit later if inline placement reads as cluttered, but inline matches the diff view's pattern.
- Resolved-comment visibility toggles different from the diff view's behavior.
