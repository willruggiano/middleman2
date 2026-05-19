# Rendered-Markdown Comments and AI Questions — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add inline review-comment and AI Ask threading to `RenderedMarkdownView.svelte`, sharing the same comment/thread storage as the diff view.

**Architecture:** A custom marked block renderer wraps each source line in `<span class="rmd-anchor" data-anchor-line="N" data-anchor-side="…">…</span>`. Selection logic mirrors `DiffFile.svelte`'s `computeSelectionRange()` against those spans. The existing `DiffComposer`, `ReviewCommentCard`, and `AIThreadCard` components are reused; the existing `diffStore`/`aiStore`/`detailStore` APIs are unchanged. No backend changes.

**Tech Stack:** Svelte 5 (`$state`, `$derived`, `$effect`), TypeScript, marked v15, DOMPurify, vitest + `@testing-library/svelte` for component tests, Go + Huma + SQLite for the e2e backend (already in place).

**Spec:** `docs/superpowers/specs/2026-05-19-rendered-markdown-comments-design.md`

---

## File Structure

| File | Disposition | Responsibility |
|------|------------|----------------|
| `packages/ui/src/components/diff/renderedMarkdownAnchors.ts` | **Create** | Pure helpers: per-line anchor span injection for prose/code blocks; selection→range resolution. Pure functions only — no Svelte. |
| `packages/ui/src/components/diff/renderedMarkdownAnchors.test.ts` | **Create** | Vitest unit tests for the above. |
| `packages/ui/src/components/diff/RenderedMarkdownView.svelte` | **Modify** | Wire per-line spans into the marked renderer, mount toolbar + composer, embed inline threads, emit outdated banner. |
| `packages/ui/src/components/diff/RenderedMarkdownView.test.ts` | **Create** | Svelte component tests for selection→composer flow and inline thread display. |
| `internal/server/worktrees_e2e_test.go` | **Modify** | New e2e test: comment created in rendered view round-trips to diff view (and vice versa). |

Pure helpers live in their own file because (a) they're the trickiest correctness surface (string splitting + line-number arithmetic), (b) they're heavily unit-testable without a real DOM, and (c) extracting them keeps `RenderedMarkdownView.svelte` from growing past comprehension.

---

## Task 1: Per-line span injection — pure helpers

**Files:**
- Create: `packages/ui/src/components/diff/renderedMarkdownAnchors.ts`
- Test: `packages/ui/src/components/diff/renderedMarkdownAnchors.test.ts`

This task ships a pure module with two responsibilities:
1. `wrapProseBlock(raw, startLine, side, parseInline)` — split prose source on `\n`, run each segment through the caller's inline parser, wrap each in `<span class="rmd-anchor" data-anchor-line=… data-anchor-side=…>…</span>`, join with a literal space.
2. `wrapCodeBlock(raw, startLine, side)` — split code source on `\n`, HTML-escape each line, wrap each line in the same span shape, join with `\n`.

- [ ] **Step 1: Write the failing test**

```typescript
// packages/ui/src/components/diff/renderedMarkdownAnchors.test.ts
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd packages/ui && bun run test renderedMarkdownAnchors -- --run`
Expected: FAIL with "Cannot find module './renderedMarkdownAnchors'".

- [ ] **Step 3: Write minimal implementation**

```typescript
// packages/ui/src/components/diff/renderedMarkdownAnchors.ts

// Per-line anchor spans let the rendered markdown viewer resolve a
// user's text selection back to a source-line range, the same way
// the diff view's <tr> rows do. The spans carry data-anchor-line
// (1-based source line) and data-anchor-side ("LEFT" or "RIGHT").

export type AnchorSide = "LEFT" | "RIGHT";

const ESC: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;",
};

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ESC[c]!);
}

function span(line: number, side: AnchorSide, inner: string): string {
  return `<span class="rmd-anchor" data-anchor-line="${line}" data-anchor-side="${side}">${inner}</span>`;
}

// wrapProseBlock splits raw on \n (markdown soft-wrap boundaries),
// runs each segment through the caller-supplied inline parser, and
// joins with a single space — the same join markdown's HTML output
// uses for soft-wrapped lines inside a paragraph.
export function wrapProseBlock(
  raw: string,
  startLine: number,
  side: AnchorSide,
  parseInline: (segment: string) => string,
): string {
  const lines = raw.split("\n");
  return lines
    .map((seg, i) => span(startLine + i, side, parseInline(seg)))
    .join(" ");
}

// wrapCodeBlock preserves newlines between segments because <pre>
// renders them as line breaks. Inline content is NOT parsed —
// code is rendered literally with HTML escaping applied.
export function wrapCodeBlock(
  raw: string,
  startLine: number,
  side: AnchorSide,
): string {
  if (raw === "") return "";
  const lines = raw.split("\n");
  return lines
    .map((seg, i) => span(startLine + i, side, escapeHtml(seg)))
    .join("\n");
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd packages/ui && bun run test renderedMarkdownAnchors -- --run`
Expected: PASS, both `describe` blocks green.

- [ ] **Step 5: Commit**

```bash
git add packages/ui/src/components/diff/renderedMarkdownAnchors.ts packages/ui/src/components/diff/renderedMarkdownAnchors.test.ts
git commit -m "feat(ui): per-line anchor span helpers for rendered markdown"
```

---

## Task 2: Selection → source-line range resolver

**Files:**
- Modify: `packages/ui/src/components/diff/renderedMarkdownAnchors.ts`
- Modify: `packages/ui/src/components/diff/renderedMarkdownAnchors.test.ts`

Adds `computeRangeFromSelection(root, selection)` — given a DOM root and a `Selection` object, walk from each end of the selection up to the nearest ancestor with `data-anchor-line`, read the line + side, and return `{startLine, endLine, side}` or `null`.

- [ ] **Step 1: Write the failing test**

Add to `renderedMarkdownAnchors.test.ts`:

```typescript
import { computeRangeFromSelection } from "./renderedMarkdownAnchors";

describe("computeRangeFromSelection", () => {
  function mkBody(html: string): HTMLElement {
    const el = document.createElement("div");
    el.innerHTML = html;
    document.body.appendChild(el);
    return el;
  }

  function selectAcross(startNode: Node, endNode: Node): Selection {
    const range = document.createRange();
    range.setStart(startNode, 0);
    range.setEnd(endNode, endNode.textContent?.length ?? 0);
    const sel = window.getSelection()!;
    sel.removeAllRanges();
    sel.addRange(range);
    return sel;
  }

  it("returns null when selection is outside the root", () => {
    const root = mkBody(`<span class="rmd-anchor" data-anchor-line="1" data-anchor-side="RIGHT">x</span>`);
    const outside = document.createElement("p");
    outside.textContent = "out";
    document.body.appendChild(outside);
    const sel = selectAcross(outside.firstChild!, outside.firstChild!);
    expect(computeRangeFromSelection(root, sel)).toBeNull();
  });

  it("resolves a single-span selection to a 1-line range", () => {
    const root = mkBody(
      `<span class="rmd-anchor" data-anchor-line="5" data-anchor-side="RIGHT">hello</span>`,
    );
    const span = root.firstChild as HTMLElement;
    const sel = selectAcross(span.firstChild!, span.firstChild!);
    expect(computeRangeFromSelection(root, sel)).toEqual({
      startLine: 5,
      endLine: 5,
      side: "RIGHT",
    });
  });

  it("resolves a selection across two spans to a 2-line range", () => {
    const root = mkBody(
      `<span class="rmd-anchor" data-anchor-line="5" data-anchor-side="RIGHT">a</span>` +
      ` ` +
      `<span class="rmd-anchor" data-anchor-line="6" data-anchor-side="RIGHT">b</span>`,
    );
    const first = root.querySelector('[data-anchor-line="5"]')!.firstChild!;
    const second = root.querySelector('[data-anchor-line="6"]')!.firstChild!;
    const sel = selectAcross(first, second);
    expect(computeRangeFromSelection(root, sel)).toEqual({
      startLine: 5,
      endLine: 6,
      side: "RIGHT",
    });
  });

  it("returns null when the two ends are on different sides", () => {
    const root = mkBody(
      `<span class="rmd-anchor" data-anchor-line="5" data-anchor-side="LEFT">a</span>` +
      `<span class="rmd-anchor" data-anchor-line="6" data-anchor-side="RIGHT">b</span>`,
    );
    const left = root.querySelector('[data-anchor-side="LEFT"]')!.firstChild!;
    const right = root.querySelector('[data-anchor-side="RIGHT"]')!.firstChild!;
    const sel = selectAcross(left, right);
    expect(computeRangeFromSelection(root, sel)).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd packages/ui && bun run test renderedMarkdownAnchors -- --run`
Expected: FAIL with "computeRangeFromSelection is not a function" (or equivalent module-export error).

- [ ] **Step 3: Write minimal implementation**

Append to `renderedMarkdownAnchors.ts`:

```typescript
export interface AnchorRange {
  startLine: number;
  endLine: number;
  side: AnchorSide;
}

// nearestAnchor walks up from node looking for an ancestor with
// data-anchor-line. Returns null if none is found inside root.
function nearestAnchor(node: Node | null, root: HTMLElement): HTMLElement | null {
  let cur: Node | null = node;
  while (cur && cur !== root) {
    if (cur.nodeType === Node.ELEMENT_NODE) {
      const el = cur as HTMLElement;
      if (el.dataset.anchorLine != null) return el;
    }
    cur = cur.parentNode;
  }
  return null;
}

export function computeRangeFromSelection(
  root: HTMLElement,
  sel: Selection | null,
): AnchorRange | null {
  if (!sel || sel.rangeCount === 0) return null;
  const anchorEl = nearestAnchor(sel.anchorNode, root);
  const focusEl = nearestAnchor(sel.focusNode, root);
  if (!anchorEl || !focusEl) return null;
  if (!root.contains(anchorEl) || !root.contains(focusEl)) return null;
  const aSide = anchorEl.dataset.anchorSide as AnchorSide | undefined;
  const fSide = focusEl.dataset.anchorSide as AnchorSide | undefined;
  if (!aSide || !fSide || aSide !== fSide) return null;
  const a = parseInt(anchorEl.dataset.anchorLine ?? "", 10);
  const f = parseInt(focusEl.dataset.anchorLine ?? "", 10);
  if (Number.isNaN(a) || Number.isNaN(f)) return null;
  const [startLine, endLine] = a < f ? [a, f] : [f, a];
  return { startLine, endLine, side: aSide };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd packages/ui && bun run test renderedMarkdownAnchors -- --run`
Expected: PASS, all four `it` blocks green.

- [ ] **Step 5: Commit**

```bash
git add packages/ui/src/components/diff/renderedMarkdownAnchors.ts packages/ui/src/components/diff/renderedMarkdownAnchors.test.ts
git commit -m "feat(ui): selection-to-source-line resolver for rendered markdown"
```

---

## Task 3: Wire per-line spans into the marked renderer

**Files:**
- Modify: `packages/ui/src/components/diff/RenderedMarkdownView.svelte` (lines ~103–172, the `doc` derivation that calls `m.parser`).

Plug `wrapProseBlock` / `wrapCodeBlock` into a `marked.Renderer` so each block's HTML output contains per-line anchor spans. The existing heading-line badge and `changedIndexes` machinery stay intact.

For this task, only override the four block emitters we need: `paragraph`, `heading`, `code`, `list_item` (lists nest via list_item, not list). Other blocks (blockquote, table) fall back to defaults for now — those rarely need per-line anchoring and we can add them later if anchoring there matters.

- [ ] **Step 1: Read the current `doc` derivation**

Open `packages/ui/src/components/diff/RenderedMarkdownView.svelte` and locate the `doc = $derived.by<RenderedDoc>(() => { … })` block (around line 103). Identify where `m.parser(tokens)` is invoked.

- [ ] **Step 2: Add anchor-injecting renderer alongside the existing custom renderer**

Replace the `doc` derivation body with the version below. The key changes:
- Import the new helpers at the top of the script.
- Compute per-token start line during the lexer walk (existing code does this for headings; extend to all blocks).
- Override `paragraph`, `heading`, `code`, `list_item` to call into the anchor helpers.

Add to imports at the top of the `<script>` block:

```typescript
import {
  wrapProseBlock,
  wrapCodeBlock,
  type AnchorSide,
} from "./renderedMarkdownAnchors";
```

Add a `side` derivation just below `renderedSHA`:

```typescript
const renderedSide: AnchorSide = $derived(
  file.status === "deleted" ? "LEFT" : "RIGHT",
);
```

Inside `doc = $derived.by<RenderedDoc>(...)`, modify the renderer block to override the four methods. Replace the existing `m.use({ renderer: { heading: … } })` (or equivalent) with:

```typescript
const startLineByTokenIdx = new Map<number, number>();
let cursorLine = 1;
for (let i = 0; i < tokens.length; i++) {
  startLineByTokenIdx.set(i, cursorLine);
  const tok = tokens[i]!;
  const raw = (tok as { raw?: string }).raw ?? "";
  cursorLine += raw.split("\n").length - 1 + (raw.endsWith("\n") ? 1 : 0);
}
// Same map for inline-rendering: pass the active block's start line
// down through a small mutable cell the renderer consults.
let currentBlockStart = 1;

m.use({
  renderer: {
    paragraph({ tokens: innerTokens, raw }): string {
      return `<p>${wrapProseBlock(raw, currentBlockStart, renderedSide, (s) =>
        m.parseInline(s) as string,
      )}</p>`;
    },
    heading({ tokens: innerTokens, raw, depth }): string {
      const inner = wrapProseBlock(raw.replace(/^#+\s*/, ""), currentBlockStart, renderedSide, (s) =>
        m.parseInline(s) as string,
      );
      const badge = `<span class="rmd-line" title="Line ${currentBlockStart}">L${currentBlockStart}</span>`;
      return `<h${depth}>${inner}${badge}</h${depth}>`;
    },
    code({ text, lang }): string {
      const langAttr = lang ? ` class="language-${lang}"` : "";
      return `<pre><code${langAttr}>${wrapCodeBlock(text, currentBlockStart, renderedSide)}</code></pre>`;
    },
    list_item({ tokens: innerTokens, raw }): string {
      return `<li>${wrapProseBlock(raw.replace(/^[-*+]\s+|^\d+\.\s+/, ""), currentBlockStart, renderedSide, (s) =>
        m.parseInline(s) as string,
      )}</li>`;
    },
  },
});

// Parse one block at a time so currentBlockStart can be updated.
let html = "";
const changedIndexes = new Set<number>();
for (let i = 0; i < tokens.length; i++) {
  currentBlockStart = startLineByTokenIdx.get(i) ?? 1;
  const tok = tokens[i]!;
  const endLine = currentBlockStart + ((tok as { raw?: string }).raw?.split("\n").length ?? 1) - 1;
  if (blockOverlapsChanged(currentBlockStart, endLine, changedLines)) {
    changedIndexes.add(i);
  }
  html += m.parser([tok]);
}
return { html, changedIndexes };
```

Drop the existing `headingLineByIdx` computation and the standalone heading-only renderer override — they're subsumed by the new logic above.

- [ ] **Step 3: Rebuild the frontend**

Run: `cd /home/awong/Repos/middleman && make frontend`
Expected: Vite builds cleanly. No TypeScript errors.

- [ ] **Step 4: Manual visual smoke check**

Run: `make dev` in one terminal, `make frontend-dev` in another. Open the running middleman in a browser, navigate to a worktree with a markdown file, switch the file to "rendered" mode. Inspect the rendered HTML in DevTools and confirm `<p>` and `<h*>` elements contain `<span class="rmd-anchor" data-anchor-line="…">` children.

If you don't have a worktree handy, point your local config at any local repo with a `.md` file and let the sync pick it up.

- [ ] **Step 5: Commit**

```bash
git add packages/ui/src/components/diff/RenderedMarkdownView.svelte
git commit -m "feat(ui): emit per-line anchor spans in rendered markdown blocks"
```

---

## Task 4: Track live selection in the rendered view

**Files:**
- Modify: `packages/ui/src/components/diff/RenderedMarkdownView.svelte`

Add `liveSelection` state that updates on `selectionchange`, mirroring `DiffFile.svelte:137`. We don't show the toolbar yet (that's Task 5) — this task just establishes the reactive state so Task 5 can layer UI on top.

- [ ] **Step 1: Add the selection state and listener**

Inside the `<script>` block, after the existing `bodyEl` declaration:

```typescript
import {
  wrapProseBlock,
  wrapCodeBlock,
  computeRangeFromSelection,
  type AnchorRange,
  type AnchorSide,
} from "./renderedMarkdownAnchors";

let liveSelection = $state<AnchorRange | null>(null);

function refreshSelection(): void {
  if (!bodyEl) return;
  const sel = typeof window !== "undefined" ? window.getSelection() : null;
  liveSelection = computeRangeFromSelection(bodyEl, sel);
}

$effect(() => {
  if (typeof document === "undefined") return;
  document.addEventListener("selectionchange", refreshSelection);
  return () => document.removeEventListener("selectionchange", refreshSelection);
});
```

- [ ] **Step 2: Write a component test that verifies liveSelection updates**

Create `packages/ui/src/components/diff/RenderedMarkdownView.test.ts`:

```typescript
import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/svelte";
import RenderedMarkdownView from "./RenderedMarkdownView.svelte";

// Minimal global mock for the /blob fetch the component performs on
// mount. Returns a small markdown sample.
beforeEach(() => {
  globalThis.fetch = vi.fn(async () => ({
    ok: true,
    status: 200,
    json: async () => ({
      content: "Line one.\nLine two.\n",
      truncated: false,
    }),
  }) as unknown as Response);
});

describe("RenderedMarkdownView", () => {
  it("renders per-line anchor spans in the body", async () => {
    const { container } = render(RenderedMarkdownView, {
      owner: "local",
      name: "demo",
      number: 1,
      path: "doc.md",
      sha: "abc",
      hunks: [],
    });
    // Wait for the async load() to settle.
    await new Promise((r) => setTimeout(r, 0));
    const anchors = container.querySelectorAll(".rmd-anchor");
    expect(anchors.length).toBeGreaterThanOrEqual(2);
    expect(anchors[0]?.getAttribute("data-anchor-line")).toBe("1");
    expect(anchors[1]?.getAttribute("data-anchor-line")).toBe("2");
  });
});
```

- [ ] **Step 3: Run test to verify it passes**

Run: `cd packages/ui && bun run test RenderedMarkdownView -- --run`
Expected: PASS, anchors found with the expected line numbers.

(If the existing component doesn't import `beforeEach` from vitest, prepend `import { beforeEach } from "vitest";` to the test file.)

- [ ] **Step 4: Commit**

```bash
git add packages/ui/src/components/diff/RenderedMarkdownView.svelte packages/ui/src/components/diff/RenderedMarkdownView.test.ts
git commit -m "feat(ui): track live selection range in rendered markdown view"
```

---

## Task 5: Floating toolbar + composer mount for new comments

**Files:**
- Modify: `packages/ui/src/components/diff/RenderedMarkdownView.svelte`

When `liveSelection` is non-null, show a small floating toolbar with "+" (comment) and "?" (Ask) buttons positioned near the end of the selection — same shape as `DiffFile.svelte:236-260`. Clicking "+" mounts a `<DiffComposer>` anchored to the snapshot range; saving calls `diffStore.addDraftComment`.

The Ask button is wired in Task 6.

- [ ] **Step 1: Add toolbar position state + snapshot**

In the `<script>` block, alongside `liveSelection`:

```typescript
import DiffComposer from "./DiffComposer.svelte";
import { getStores } from "@middleman/ui/stores";

const { diff: diffStore } = getStores();

let rangeSnapshot = $state<AnchorRange | null>(null);
let toolbarTop = $state(0);
let toolbarLeft = $state(0);
let openComposerKey = $state<string | null>(null);

function updateToolbarPosition(): void {
  if (typeof window === "undefined" || !liveSelection) return;
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) return;
  const rect = sel.getRangeAt(0).getBoundingClientRect();
  if (rect.width === 0 && rect.height === 0) return;
  toolbarTop = rect.bottom + window.scrollY + 4;
  toolbarLeft = rect.right + window.scrollX - 90;
}

$effect(() => {
  if (liveSelection) updateToolbarPosition();
});

function openComposerFromToolbar(): void {
  if (!liveSelection) return;
  rangeSnapshot = liveSelection;
  openComposerKey = `${liveSelection.endLine}:${liveSelection.side}`;
}

function closeComposer(): void {
  openComposerKey = null;
  rangeSnapshot = null;
}

function saveDraft(body: string): void {
  const range = rangeSnapshot;
  if (!range) return;
  diffStore.addDraftComment({
    path,
    line: range.endLine,
    side: range.side,
    ...(range.startLine !== range.endLine ? { startLine: range.startLine } : {}),
    commitSha: sha,
    body,
  });
  closeComposer();
}
```

(The `path` and `sha` props are already available — they're declared in `Props` at line 30.)

- [ ] **Step 2: Add the toolbar and composer to the template**

Inside the existing markup at the bottom of the template (after the `{#if raw !== null}` block that renders the body), add:

```svelte
{#if liveSelection}
  <div
    class="rmd-toolbar"
    style:top="{toolbarTop}px"
    style:left="{toolbarLeft}px"
  >
    <button type="button" class="rmd-tb-btn" onclick={openComposerFromToolbar}
      title="Comment on lines {liveSelection.startLine}–{liveSelection.endLine}">+</button>
  </div>
{/if}

{#if openComposerKey && rangeSnapshot}
  <div class="rmd-composer-wrap">
    <DiffComposer
      anchor={{ line: rangeSnapshot.endLine, side: rangeSnapshot.side, startLine: rangeSnapshot.startLine }}
      onsave={saveDraft}
      oncancel={closeComposer}
    />
  </div>
{/if}
```

Add minimal styles at the bottom of the `<style>` block:

```css
.rmd-toolbar {
  position: absolute;
  display: flex;
  gap: 4px;
  padding: 4px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-muted);
  border-radius: var(--radius-md);
  box-shadow: 0 2px 8px rgba(0,0,0,0.15);
  z-index: 5;
}
.rmd-tb-btn {
  width: 24px;
  height: 24px;
  background: transparent;
  border: 0;
  cursor: pointer;
  color: var(--text-primary);
}
.rmd-composer-wrap {
  position: relative;
  margin-top: 12px;
}
```

- [ ] **Step 3: Manual visual smoke check**

`make frontend && make build`, restart middleman, open a markdown file in rendered mode, select text spanning two lines. Confirm the "+" toolbar appears below the selection. Click it — the composer should mount with the line range visible. Submit (Cmd/Ctrl+Enter). The draft should appear in the diff view's drafts panel for the same file at the same line range.

- [ ] **Step 4: Commit**

```bash
git add packages/ui/src/components/diff/RenderedMarkdownView.svelte
git commit -m "feat(ui): floating composer for review comments in rendered markdown"
```

---

## Task 6: AI Ask flow in the rendered view

**Files:**
- Modify: `packages/ui/src/components/diff/RenderedMarkdownView.svelte`

Add a "?" button to the toolbar that mounts `AIAskComposer` (sibling of `DiffComposer`, located at `packages/ui/src/components/diff/AIAskComposer.svelte`) and wires submit to `aiStore.createThread`. Mirrors `DiffFile.svelte:337-380` (`submitAsk`).

- [ ] **Step 1: Add the Ask state and submit function**

Add to the `<script>` block:

```typescript
import AIAskComposer from "./AIAskComposer.svelte";

const { ai: aiStore } = getStores(); // adjust the destructure from Task 5 to include ai

let openAskKey = $state<string | null>(null);
let askError = $state<string | null>(null);
let askSubmitting = $state(false);
let selectionSnapshot = $state<string | null>(null);

function openAskFromToolbar(): void {
  if (!liveSelection) return;
  rangeSnapshot = liveSelection;
  const sel = typeof window !== "undefined" ? window.getSelection() : null;
  selectionSnapshot = sel?.toString().trim() || null;
  openAskKey = `${liveSelection.endLine}:${liveSelection.side}`;
  askError = null;
}

function closeAsk(): void {
  openAskKey = null;
  rangeSnapshot = null;
  selectionSnapshot = null;
  askError = null;
  askSubmitting = false;
}

async function submitAsk(question: string): Promise<void> {
  const range = rangeSnapshot;
  if (!range || askSubmitting) return;
  askSubmitting = true;
  askError = null;
  try {
    const body: Parameters<typeof aiStore.createThread>[0] = {
      path,
      anchor_side: range.side,
      anchor_line: range.endLine,
      commit_sha: sha,
      question,
    };
    if (range.startLine !== range.endLine) {
      body.hunk_start_line = range.startLine;
      body.hunk_end_line = range.endLine;
    }
    const result = await aiStore.createThread(body);
    if (result.ok) {
      closeAsk();
    } else {
      askError = result.error;
    }
  } finally {
    askSubmitting = false;
  }
}
```

- [ ] **Step 2: Add the "?" button and Ask composer to the template**

Inside the existing `.rmd-toolbar` div from Task 5, after the "+" button:

```svelte
<button type="button" class="rmd-tb-btn" onclick={openAskFromToolbar}
  title="Ask Claude about lines {liveSelection.startLine}–{liveSelection.endLine}">?</button>
```

Below the `{#if openComposerKey ...}` block:

```svelte
{#if openAskKey && rangeSnapshot}
  <div class="rmd-composer-wrap">
    <AIAskComposer
      anchor={{ line: rangeSnapshot.endLine, side: rangeSnapshot.side, startLine: rangeSnapshot.startLine }}
      {...(selectionSnapshot ? { selectionPreview: selectionSnapshot } : {})}
      error={askError}
      submitting={askSubmitting}
      onsubmit={(q) => void submitAsk(q)}
      oncancel={closeAsk}
    />
  </div>
{/if}
```

- [ ] **Step 3: Manual visual smoke check**

Rebuild + restart. Select text, click "?", type a question, submit. Switch to the diff view for the same file — the AI thread should appear at the same anchor.

- [ ] **Step 4: Commit**

```bash
git add packages/ui/src/components/diff/RenderedMarkdownView.svelte
git commit -m "feat(ui): AI Ask composer in rendered markdown view"
```

---

## Task 7: Inline thread display + outdated banner

**Files:**
- Modify: `packages/ui/src/components/diff/RenderedMarkdownView.svelte`

Read draft comments, published comments, and AI threads anchored to lines in this file. For each rendered block, find threads whose anchor's `[startLine, endLine]` overlaps `[blockStart, blockEnd]` and render them inline after the block. Surface an "N outdated review comments" banner at the top when published comments have line ≤ 0.

Because the block-level placement requires per-block source-line ranges that are computed inside the marked walk, this task threads that data out of the `doc` derivation as `blockRangeByIdx: Map<number, [number, number]>`. After mount, a post-render `$effect` (analogous to the existing `rmd-changed` walker at line 202) inserts thread cards as DOM siblings of the right block.

- [ ] **Step 1: Expose block ranges from the `doc` derivation**

Extend `RenderedDoc`:

```typescript
interface RenderedDoc {
  html: string;
  changedIndexes: Set<number>;
  blockRangeByIdx: Map<number, [number, number]>;
}
```

In the `for (let i = 0; i < tokens.length; i++)` loop from Task 3, also store:

```typescript
blockRangeByIdx.set(i, [currentBlockStart, endLine]);
```

Return `blockRangeByIdx` alongside `html` and `changedIndexes`.

- [ ] **Step 2: Read threads + drafts + published comments**

In the `<script>` block:

```typescript
import { mount, unmount } from "svelte";
import { getStores } from "@middleman/ui/stores"; // ensure detail is destructured
import ReviewCommentCard from "./ReviewCommentCard.svelte";
import AIThreadCard from "./AIThreadCard.svelte";

const { diff: diffStore, ai: aiStore, detail: detailStore } = getStores();

const drafts = $derived(diffStore.getDraftCommentsForPath(path));
const publishedForFile = $derived(
  detailStore.getReviewCommentsByFilePath().get(path) ?? [],
);
const aiThreadsForFile = $derived(aiStore.getThreadsForFile(path));

const outdatedCount = $derived(
  publishedForFile.filter((c: { line: number }) => c.line <= 0).length,
);
```

The `aiStore.getThreadsForFile(path: string): AIThread[]` accessor already exists at `packages/ui/src/stores/ai.svelte.ts:39`. Drafts live in localStorage only (no API); both the diff view and rendered view share the same in-memory snapshot via `diffStore`.

- [ ] **Step 3: Add a helper that collects threads overlapping a block range**

In the `<script>` block, below the `$derived` declarations:

```typescript
type CardSpec =
  | { kind: "draft"; key: string; comment: typeof drafts[number] }
  | { kind: "published"; key: string; comment: typeof publishedForFile[number] }
  | { kind: "ai"; key: string; thread: typeof aiThreadsForFile[number] };

function cardsForRange(start: number, end: number): CardSpec[] {
  const out: CardSpec[] = [];
  for (const c of drafts) {
    const cStart = c.startLine ?? c.line;
    if (c.side === renderedSide && cStart <= end && c.line >= start) {
      out.push({ kind: "draft", key: `d:${c.id ?? `${c.line}:${c.side}`}`, comment: c });
    }
  }
  for (const c of publishedForFile) {
    if (c.line <= 0) continue; // outdated; covered by banner
    const cStart = (c as { startLine?: number }).startLine ?? c.line;
    if (c.side === renderedSide && cStart <= end && c.line >= start) {
      out.push({ kind: "published", key: `p:${c.id}`, comment: c });
    }
  }
  for (const t of aiThreadsForFile) {
    const tStart = (t.hunk_start_line ?? t.anchor_line);
    const tEnd = (t.hunk_end_line ?? t.anchor_line);
    if (t.anchor_side === renderedSide && tStart <= end && tEnd >= start) {
      out.push({ kind: "ai", key: `a:${t.id}`, thread: t });
    }
  }
  return out;
}
```

- [ ] **Step 4: Post-render `$effect` that mounts thread cards inline**

Replace the existing `rmd-changed` walker `$effect` (around line 202 in the file before this task's edits) with this combined version. Tracking the mounted instances in a set lets us unmount cleanly when the effect re-runs.

```typescript
const mountedInstances = new Set<ReturnType<typeof mount>>();

$effect(() => {
  if (!bodyEl) return;
  // Touch all reactive deps so the effect re-runs.
  const _ = doc;
  const __ = drafts;
  const ___ = publishedForFile;
  const ____ = aiThreadsForFile;

  // Tear down previous mounts and previous thread wrappers.
  for (const inst of mountedInstances) unmount(inst);
  mountedInstances.clear();
  bodyEl.querySelectorAll(".rmd-thread-wrap").forEach((el) => el.remove());

  const children = Array.from(bodyEl.children) as HTMLElement[];
  for (let i = 0; i < children.length; i++) {
    const el = children[i]!;
    if (doc.changedIndexes.has(i)) el.classList.add("rmd-changed");
    else el.classList.remove("rmd-changed");

    const range = doc.blockRangeByIdx.get(i);
    if (!range) continue;
    const cards = cardsForRange(range[0], range[1]);
    if (cards.length === 0) continue;

    const wrap = document.createElement("div");
    wrap.className = "rmd-thread-wrap";
    for (const spec of cards) {
      const host = document.createElement("div");
      host.className = "rmd-thread-host";
      wrap.appendChild(host);
      if (spec.kind === "ai") {
        const inst = mount(AIThreadCard, {
          target: host,
          props: { thread: spec.thread, repoOwner: owner, repoName: name },
        });
        mountedInstances.add(inst);
      } else {
        // Both draft and published comments use ReviewCommentCard.
        // Check ReviewCommentCard.svelte's Props for the exact prop
        // name (`comment` vs `draft` vs `published`) and adapt this
        // line accordingly — keep the if/else if the component
        // expects different props per kind.
        const inst = mount(ReviewCommentCard, {
          target: host,
          props: { comment: spec.comment, kind: spec.kind },
        });
        mountedInstances.add(inst);
      }
    }
    el.after(wrap);
  }
});
```

Open `ReviewCommentCard.svelte` once to confirm the exact prop shape (`Props` interface at the top of the file). If it expects different props for draft vs published, split the branch above accordingly. Don't guess — adapt to what the component actually accepts.

- [ ] **Step 5: Outdated banner**

In the template, just before the body div:

```svelte
{#if outdatedCount > 0}
  <div class="outdated-banner" title="These comments don't resolve in the current rendered view.">
    {outdatedCount} outdated review comment{outdatedCount === 1 ? "" : "s"} on this file
  </div>
{/if}
```

Reuse `.outdated-banner` CSS from `DiffFile.svelte:1241` or copy it into the rendered view's `<style>` block.

- [ ] **Step 6: Manual visual check**

Create a comment in the diff view on a markdown file. Switch to rendered mode. Verify the comment card appears below the rendered block that contains the comment's source lines. Force a stale anchor (e.g., by simulating a published comment with `line = 0`) and verify the banner appears.

- [ ] **Step 7: Commit**

```bash
git add packages/ui/src/components/diff/RenderedMarkdownView.svelte
git commit -m "feat(ui): inline threads + outdated banner in rendered markdown"
```

---

## Task 8: End-to-end AI thread round-trip for local worktrees

**Files:**
- Modify: `internal/server/worktrees_e2e_test.go`

Draft review comments are localStorage-only, so the "diff view ↔ rendered view" round-trip for them is purely client-side (already verified by the unit tests in Task 4–5). AI threads, however, do go through the server. This test pins that the AI thread create endpoint accepts the anchor shape (`path`, `anchor_line`, `anchor_side`, `commit_sha`, `hunk_start_line/end_line`) that the rendered view will send and the diff view can already send.

- [ ] **Step 1: Add the test**

Append to `internal/server/worktrees_e2e_test.go`, just above `func runGitWT`:

```go
// TestAPILocalDispatchAIThreadAcceptsRangeAnchor pins the contract
// the rendered markdown view (and the diff view) rely on: the AI
// thread create endpoint accepts an anchor of {path, anchor_line,
// anchor_side, commit_sha, hunk_start_line, hunk_end_line} for a
// local worktree, persists it, and returns it on subsequent reads.
func TestAPILocalDispatchAIThreadAcceptsRangeAnchor(t *testing.T) {
    if _, err := exec.LookPath("git"); err != nil {
        t.Skip("git not available on PATH")
    }
    require := require.New(t)
    assert := Assert.New(t)
    srv, database := setupTestServer(t)
    client := setupTestClient(t, srv)
    ctx := context.Background()

    dir := t.TempDir()
    runGitWT(t, "", "init", "--initial-branch=main", dir)
    runGitWT(t, dir, "config", "user.email", "test@example.com")
    runGitWT(t, dir, "config", "user.name", "Test")
    require.NoError(os.WriteFile(filepath.Join(dir, "doc.md"),
        []byte("line one\nline two\nline three\n"), 0o644))
    runGitWT(t, dir, "add", "doc.md")
    runGitWT(t, dir, "commit", "-m", "init")
    headOut, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
    require.NoError(err)
    headSHA := string(headOut[:len(headOut)-1])

    repoID, err := database.UpsertLocalRepo(ctx, "demo")
    require.NoError(err)
    canonDir, err := filepath.EvalSymlinks(dir)
    require.NoError(err)
    w, err := database.UpsertWorktree(ctx, repoID, db.ScannedWorktree{
        Path:   canonDir,
        Branch: "main",
    })
    require.NoError(err)

    docPath := "doc.md"
    hunkStart, hunkEnd := int64(1), int64(2)
    body := generated.PostReposByOwnerByNamePullsByNumberAiThreadsJSONRequestBody{
        Path:          docPath,
        AnchorLine:    2,
        AnchorSide:    "RIGHT",
        CommitSha:     headSHA,
        HunkStartLine: &hunkStart,
        HunkEndLine:   &hunkEnd,
        Question:      "What does this section say?",
    }
    createResp, err := client.HTTP.PostReposByOwnerByNamePullsByNumberAiThreadsWithResponse(
        ctx, "local", "demo", w.ID, body,
    )
    require.NoError(err)
    require.Equal(http.StatusCreated, createResp.StatusCode())
    require.NotNil(createResp.JSON201)

    listResp, err := client.HTTP.GetReposByOwnerByNamePullsByNumberAiThreadsWithResponse(
        ctx, "local", "demo", w.ID,
    )
    require.NoError(err)
    require.Equal(http.StatusOK, listResp.StatusCode())
    require.NotNil(listResp.JSON200)
    require.NotNil(listResp.JSON200.Threads)
    threads := *listResp.JSON200.Threads
    require.Len(threads, 1)
    assert.Equal(docPath, threads[0].Path)
    assert.EqualValues(2, threads[0].AnchorLine)
    assert.Equal("RIGHT", threads[0].AnchorSide)
}
```

If the field names on `PostReposByOwnerByNamePullsByNumberAiThreadsJSONRequestBody` differ from what's shown (for example `CreateAIThreadInputBody` instead), substitute them. Run:
`grep "PostReposByOwnerByNamePullsByNumberAiThreads\b\|CreateAIThreadInputBody" internal/apiclient/generated/client.gen.go | head -20`
to discover the exact names. The shape (path / anchor_line / anchor_side / commit_sha / hunk_start_line / hunk_end_line / question) is pinned by `CreateAIThreadInputBody` at `internal/apiclient/generated/client.gen.go:288`.

- [ ] **Step 2: Run the test**

Run: `go test ./internal/server -run TestAPILocalDispatchAIThreadAcceptsRangeAnchor -shuffle=on`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/server/worktrees_e2e_test.go
git commit -m "test(server): AI thread create accepts range anchor for local worktrees"
```

---

## Verification checklist (before declaring done)

- [ ] All vitest tests pass: `cd packages/ui && bun run test -- --run`
- [ ] All Go tests pass: `make test` (the pre-existing `TestAPIActivityReturnsUTCCreatedAt` failure can stay)
- [ ] `make vet` clean
- [ ] `make build` clean
- [ ] Manual: select text in a rendered .md file, comment via toolbar, confirm it appears in the diff view. Reverse direction.
- [ ] Manual: same for AI Ask.
- [ ] Manual: outdated comment banner shows when a published comment has `line <= 0`.
