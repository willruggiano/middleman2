<script lang="ts">
  import type { LocalWorktree } from "../../api/types.js";

  interface Props {
    worktree: LocalWorktree;
  }
  const { worktree }: Props = $props();

  function basename(path: string): string {
    const i = path.lastIndexOf("/");
    return i >= 0 ? path.slice(i + 1) : path;
  }

  const branchLabel = $derived(
    worktree.is_detached
      ? "(detached)"
      : worktree.branch || "",
  );
</script>

<div
  class="worktree-item"
  class:worktree-item--locked={worktree.is_locked}
  class:worktree-item--prunable={worktree.is_prunable}
  title={worktree.path}
>
  <span class="worktree-item__icon" aria-hidden="true">
    <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
      <circle cx="5" cy="4" r="2" />
      <circle cx="11" cy="12" r="2" />
      <path d="M5 6v6M5 12h4M11 4v6" stroke-linecap="round" />
    </svg>
  </span>
  <span class="worktree-item__name">{basename(worktree.path)}</span>
  {#if branchLabel}
    <span class="worktree-item__branch">{branchLabel}</span>
  {/if}
  {#if worktree.is_locked}
    <span class="worktree-item__flag" title="Locked worktree">L</span>
  {/if}
  {#if worktree.is_prunable}
    <span class="worktree-item__flag" title="Prunable worktree">!</span>
  {/if}
</div>

<style>
  .worktree-item {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 12px 4px 20px;
    font-size: 12px;
    color: var(--text-secondary);
    background: var(--bg-surface);
    border-top: 1px solid var(--border-muted);
  }

  .worktree-item__icon {
    display: inline-flex;
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .worktree-item__name {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex-shrink: 1;
    min-width: 0;
  }

  .worktree-item__branch {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex-shrink: 1;
    min-width: 0;
  }

  .worktree-item__flag {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 600;
    padding: 1px 4px;
    border-radius: 999px;
    background: var(--bg-inset);
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .worktree-item--locked .worktree-item__flag {
    color: var(--accent-amber);
  }

  .worktree-item--prunable .worktree-item__flag {
    color: var(--accent-red);
  }
</style>
