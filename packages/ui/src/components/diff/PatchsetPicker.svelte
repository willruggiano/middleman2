<script lang="ts">
  import { getStores } from "../../context.js";
  import { timeAgo } from "../../utils/time.js";

  // Phase 2 surface: shows the PSn chip strip plus a "compare to"
  // dropdown so the reviewer can pick a baseline patchset. Loads
  // lazily on first mount so PRs with one push (the common case)
  // never pay the round-trip when nothing's interesting yet.
  //
  // No diff wiring yet — selection is captured locally and will be
  // consumed by Phase 3 when the rebase-subtracted diff lands.

  const { diff } = getStores();

  const patchsets = $derived(diff.getPatchsets());
  const loading = $derived(diff.isPatchsetsLoading());
  const errorMsg = $derived(diff.getPatchsetsError());

  // Local selection state; default selected = newest, default base
  // = "(parent)" meaning the merge-base (no patchset comparison).
  let selectedNumber = $state<number | null>(null);
  let baseNumber = $state<number | null>(null); // null = none / parent

  $effect(() => {
    void diff.loadPatchsets();
  });

  // Default the selection once data arrives.
  $effect(() => {
    const list = patchsets;
    if (!list || list.length === 0) return;
    if (selectedNumber === null) {
      selectedNumber = list[list.length - 1]!.number;
    }
  });

  function pick(n: number, e?: MouseEvent): void {
    if (e?.shiftKey && selectedNumber !== null && n !== selectedNumber) {
      // Shift-click sets the base to make a "PSn vs PSm" pair.
      baseNumber = n;
      return;
    }
    selectedNumber = n;
    if (baseNumber !== null && baseNumber === n) {
      baseNumber = null;
    }
  }

  function clearBase(): void {
    baseNumber = null;
  }
</script>

{#if patchsets && patchsets.length > 1}
  <div class="ps-picker" role="toolbar" aria-label="Patchsets">
    <span class="ps-picker__label">Patchsets</span>
    <div class="ps-picker__chips">
      {#each patchsets as p (p.id)}
        {@const isSelected = p.number === selectedNumber}
        {@const isBase = p.number === baseNumber}
        <button
          type="button"
          class="ps-chip"
          class:ps-chip--selected={isSelected}
          class:ps-chip--base={isBase}
          onclick={(e) => pick(p.number, e)}
          title={
            (isBase ? "Comparing FROM this patchset.\n" : "") +
            (isSelected ? "Currently viewing this patchset.\n" : "") +
            `Head: ${p.head_sha.slice(0, 7)}\n` +
            `Observed: ${timeAgo(p.observed_at)}\n\n` +
            "Click to view; shift-click to set as compare base."
          }
        >
          PS{p.number}
        </button>
      {/each}
    </div>
    {#if baseNumber !== null}
      <button
        type="button"
        class="ps-picker__reset"
        onclick={clearBase}
        title="Clear compare base"
      >
        compare PS{baseNumber} → PS{selectedNumber} ✕
      </button>
    {:else}
      <span class="ps-picker__hint">shift-click a chip to compare</span>
    {/if}
  </div>
{:else if loading}
  <div class="ps-picker ps-picker--idle">Loading patchsets…</div>
{:else if errorMsg}
  <div class="ps-picker ps-picker--error">Failed to load patchsets: {errorMsg}</div>
{/if}

<style>
  .ps-picker {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 12px;
    background: var(--bg-inset);
    border-bottom: 1px solid var(--border-muted);
    font-size: 11px;
    color: var(--text-muted);
    flex-wrap: wrap;
  }

  .ps-picker--idle,
  .ps-picker--error {
    color: var(--text-muted);
    font-style: italic;
  }

  .ps-picker--error {
    color: var(--accent-red);
  }

  .ps-picker__label {
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-muted);
  }

  .ps-picker__chips {
    display: inline-flex;
    gap: 4px;
    align-items: center;
  }

  .ps-chip {
    display: inline-flex;
    align-items: center;
    padding: 2px 8px;
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 600;
    color: var(--text-secondary);
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: 999px;
    cursor: pointer;
  }

  .ps-chip:hover {
    color: var(--text-primary);
    border-color: var(--accent-blue);
  }

  .ps-chip--selected {
    background: var(--accent-blue);
    border-color: var(--accent-blue);
    color: #fff;
  }

  .ps-chip--base {
    border-color: var(--accent-amber);
    color: var(--accent-amber);
    background: color-mix(in srgb, var(--accent-amber) 10%, var(--bg-surface));
  }

  .ps-chip--base.ps-chip--selected {
    background: var(--accent-amber);
    color: #fff;
  }

  .ps-picker__reset {
    margin-left: auto;
    font-size: 10px;
    color: var(--accent-amber);
    background: none;
    border: none;
    cursor: pointer;
    font-family: var(--font-mono);
  }

  .ps-picker__reset:hover {
    color: var(--accent-blue);
  }

  .ps-picker__hint {
    margin-left: auto;
    font-size: 10px;
    color: var(--text-muted);
    font-style: italic;
  }
</style>
