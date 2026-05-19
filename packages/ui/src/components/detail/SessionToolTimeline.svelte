<script lang="ts">
  // Renders the stream-json events recorded by the SessionRunner.
  // Text bubbles surface Claude's running commentary; tool_use cards
  // pair with their tool_result to give the reviewer visibility into
  // what Claude actually did (files touched, commands run) instead
  // of just the final summary.

  interface StreamEvent {
    type: "text" | "tool_use" | "tool_result";
    text?: string;
    tool?: string;
    input?: unknown;
    id?: string;
    tool_use_id?: string;
    content?: string;
    is_error?: boolean;
  }

  interface StreamState {
    session_id?: string;
    events?: StreamEvent[];
  }

  interface Props {
    rawJSON: string;
  }
  const { rawJSON }: Props = $props();

  const parsed = $derived.by((): StreamState => {
    if (!rawJSON) return { events: [] };
    try {
      const v = JSON.parse(rawJSON) as StreamState;
      return v ?? { events: [] };
    } catch {
      return { events: [] };
    }
  });

  // Group tool_use events with their matching tool_result so the
  // card has the result inline (rather than the result floating as
  // a separate event further down). Text events stay as-is in their
  // own slot. Result-only events (orphans) still render so reviewers
  // can spot mismatches.
  type TimelineNode =
    | { kind: "text"; text: string; key: string }
    | {
        kind: "tool";
        tool: string;
        input: unknown;
        result: string | null;
        isError: boolean;
        key: string;
      };

  const nodes = $derived.by((): TimelineNode[] => {
    const events = parsed.events ?? [];
    const resultsByID = new Map<string, StreamEvent>();
    for (const e of events) {
      if (e.type === "tool_result" && e.tool_use_id) {
        resultsByID.set(e.tool_use_id, e);
      }
    }
    const consumed = new Set<string>();
    const out: TimelineNode[] = [];
    events.forEach((e, i) => {
      if (e.type === "text") {
        if (e.text) {
          out.push({ kind: "text", text: e.text, key: `t${i}` });
        }
        return;
      }
      if (e.type === "tool_use") {
        const id = e.id ?? "";
        const res = id ? resultsByID.get(id) : undefined;
        if (id) consumed.add(id);
        out.push({
          kind: "tool",
          tool: e.tool ?? "(unnamed)",
          input: e.input,
          result: res?.content ?? null,
          isError: !!res?.is_error,
          key: `u${i}`,
        });
        return;
      }
      // tool_result without a preceding tool_use — show as an
      // unattached result card so it doesn't silently disappear.
      if (e.type === "tool_result") {
        const id = e.tool_use_id ?? "";
        if (consumed.has(id)) return;
        out.push({
          kind: "tool",
          tool: "(tool result)",
          input: undefined,
          result: e.content ?? null,
          isError: !!e.is_error,
          key: `r${i}`,
        });
      }
    });
    return out;
  });

  function summarizeInput(tool: string, input: unknown): string {
    if (input === null || input === undefined) return "";
    if (typeof input !== "object") return String(input);
    const obj = input as Record<string, unknown>;
    // Heuristics for the tools we explicitly allow.
    if (tool === "Bash" && typeof obj.command === "string") {
      return obj.command;
    }
    if (
      (tool === "Read" || tool === "Edit" || tool === "Write" || tool === "MultiEdit") &&
      typeof obj.file_path === "string"
    ) {
      return obj.file_path;
    }
    if (tool === "Grep" && typeof obj.pattern === "string") {
      return obj.pattern;
    }
    if (tool === "Glob" && typeof obj.pattern === "string") {
      return obj.pattern;
    }
    // Fallback: stringify, truncated.
    try {
      const s = JSON.stringify(obj);
      return s.length > 120 ? s.slice(0, 117) + "..." : s;
    } catch {
      return "";
    }
  }

  function formatInputDetail(input: unknown): string {
    if (input === null || input === undefined) return "";
    try {
      return JSON.stringify(input, null, 2);
    } catch {
      return String(input);
    }
  }

  function truncate(s: string, n: number): string {
    return s.length > n ? s.slice(0, n - 1) + "…" : s;
  }
</script>

{#if nodes.length > 0}
  <div class="timeline">
    {#each nodes as n (n.key)}
      {#if n.kind === "text"}
        <div class="timeline__text">{n.text}</div>
      {:else}
        <details class="tool" class:tool--error={n.isError}>
          <summary class="tool__summary">
            <span class="tool__tool">{n.tool}</span>
            <span class="tool__detail" title={summarizeInput(n.tool, n.input)}>
              {truncate(summarizeInput(n.tool, n.input), 100)}
            </span>
            {#if n.isError}
              <span class="tool__badge">error</span>
            {/if}
          </summary>
          {#if n.input !== undefined && n.input !== null}
            <div class="tool__section">
              <div class="tool__label">input</div>
              <pre class="tool__pre">{formatInputDetail(n.input)}</pre>
            </div>
          {/if}
          {#if n.result !== null}
            <div class="tool__section">
              <div class="tool__label">result</div>
              <pre class="tool__pre">{n.result}</pre>
            </div>
          {/if}
        </details>
      {/if}
    {/each}
  </div>
{/if}

<style>
  .timeline {
    margin-bottom: 10px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .timeline__text {
    font-size: 12px;
    line-height: 1.5;
    color: var(--text-secondary);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .tool {
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
    overflow: hidden;
  }

  .tool--error {
    border-color: color-mix(in srgb, var(--accent-red) 40%, var(--border-muted));
    background: color-mix(in srgb, var(--accent-red) 4%, var(--bg-inset));
  }

  .tool__summary {
    display: flex;
    align-items: baseline;
    gap: 8px;
    padding: 4px 8px;
    cursor: pointer;
    font-size: 11px;
    font-family: var(--font-mono);
    user-select: none;
  }

  .tool__summary:hover {
    background: var(--bg-surface-hover);
  }

  .tool__tool {
    font-weight: 600;
    color: var(--text-primary);
  }

  .tool__detail {
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
    flex: 1;
  }

  .tool__badge {
    font-size: 10px;
    padding: 1px 5px;
    border-radius: 999px;
    background: color-mix(in srgb, var(--accent-red) 18%, transparent);
    color: var(--accent-red);
    font-weight: 600;
    flex-shrink: 0;
  }

  .tool__section {
    padding: 4px 8px 8px;
    border-top: 1px solid var(--border-muted);
  }

  .tool__label {
    font-size: 10px;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    margin-bottom: 2px;
  }

  .tool__pre {
    margin: 0;
    padding: 6px 8px;
    background: var(--bg-canvas);
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    font-size: 11px;
    font-family: var(--font-mono);
    color: var(--text-primary);
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 360px;
    overflow: auto;
  }
</style>
