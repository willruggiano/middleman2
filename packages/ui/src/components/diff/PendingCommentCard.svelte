<script lang="ts">
  import type { DraftComment } from "../../stores/diff.svelte.js";

  interface Props {
    comment: DraftComment;
    stale: boolean; // true when the comment's commit SHA is no longer the PR head
    ondelete: () => void;
  }

  const { comment, stale, ondelete }: Props = $props();
</script>

<div class="pending" class:pending--stale={stale}>
  <div class="pending__header">
    <span class="pending__badge">Pending</span>
    {#if stale}
      <span class="pending__stale-badge" title="Pending against an older commit — may fail on publish">Stale</span>
    {/if}
    <span class="pending__anchor">
      {comment.side === "LEFT" ? "−" : "+"}{comment.line}
    </span>
    <button type="button" class="pending__delete" onclick={ondelete} title="Delete pending comment">
      <svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1.6">
        <path d="M2 2L8 8M8 2L2 8" stroke-linecap="round" />
      </svg>
    </button>
  </div>
  <div class="pending__body">{comment.body}</div>
</div>

<style>
  .pending {
    margin: 4px 12px 8px 68px;
    padding: 8px 10px;
    border: 1px solid var(--accent-amber);
    border-left: 3px solid var(--accent-amber);
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--accent-amber) 6%, var(--bg-surface));
  }

  .pending--stale {
    border-color: var(--accent-red);
    border-left-color: var(--accent-red);
    background: color-mix(in srgb, var(--accent-red) 6%, var(--bg-surface));
  }

  .pending__header {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 4px;
  }

  .pending__badge {
    font-size: 10px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 1px 6px;
    border-radius: 999px;
    background: var(--accent-amber);
    color: #000;
  }

  .pending__stale-badge {
    font-size: 10px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 1px 6px;
    border-radius: 999px;
    background: var(--accent-red);
    color: #fff;
  }

  .pending__anchor {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-muted);
  }

  .pending__delete {
    margin-left: auto;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    border-radius: var(--radius-sm);
    border: none;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
  }

  .pending__delete:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  .pending__body {
    font-size: 13px;
    color: var(--text-primary);
    white-space: pre-wrap;
    word-break: break-word;
    line-height: 1.5;
  }
</style>
