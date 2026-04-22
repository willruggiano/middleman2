<script lang="ts">
  import { getStores } from "../../context.js";
  import type { HeatmapCell, HeatmapCommit } from "../../stores/diff.svelte.js";

  const { diff } = getStores();

  let expanded = $state(false);

  const heatmap = $derived(diff.getHeatmap());
  const loading = $derived(diff.isHeatmapLoading());
  const loadError = $derived(diff.getHeatmapError());

  function toggle(): void {
    expanded = !expanded;
    if (expanded && !heatmap && !loading) {
      void diff.loadHeatmap();
    }
  }

  // Aggregate cells per file so rows can be ordered by churn and we
  // can render a totals column alongside the per-commit breakdown.
  interface FileRow {
    path: string;
    total: number;
    totalAdds: number;
    totalDels: number;
    byCommit: Map<string, HeatmapCell>;
  }

  const rows = $derived.by<FileRow[]>(() => {
    if (!heatmap) return [];
    const byPath = new Map<string, FileRow>();
    for (const c of heatmap.cells) {
      let row = byPath.get(c.path);
      if (!row) {
        row = {
          path: c.path, total: 0, totalAdds: 0, totalDels: 0,
          byCommit: new Map(),
        };
        byPath.set(c.path, row);
      }
      row.total += c.additions + c.deletions;
      row.totalAdds += c.additions;
      row.totalDels += c.deletions;
      row.byCommit.set(c.commit_sha, c);
    }
    const arr = Array.from(byPath.values());
    arr.sort((a, b) => {
      if (b.total !== a.total) return b.total - a.total;
      return a.path.localeCompare(b.path);
    });
    return arr;
  });

  // Global max intensity (log-scaled) drives a subtle cell tint — a
  // secondary cue so heavy files still "pop" before the reviewer
  // reads the numbers. Using sum(+/-) keeps the scale consistent
  // whether a commit is mostly adds or mostly deletes.
  const maxIntensity = $derived.by<number>(() => {
    if (!heatmap) return 0;
    let max = 0;
    for (const c of heatmap.cells) {
      const v = c.additions + c.deletions;
      if (v > max) max = v;
    }
    return max;
  });

  function cellStyle(cell: HeatmapCell | undefined): string {
    if (!cell) return "";
    if (cell.binary) return "";
    const intensity = cell.additions + cell.deletions;
    if (intensity === 0 || maxIntensity === 0) return "";
    const t = Math.log(1 + intensity) / Math.log(1 + maxIntensity);
    const opacity = Math.max(0.05, Math.min(0.35, 0.35 * t));
    return `background: color-mix(in srgb, var(--accent-blue) ${Math.round(opacity * 100)}%, transparent);`;
  }

  function cellTitle(
    commit: HeatmapCommit,
    path: string,
    cell: HeatmapCell | undefined,
  ): string {
    if (!cell) return `${shortSha(commit.sha)} — ${commit.title}\n(no change to ${path})`;
    if (cell.binary) {
      return `${shortSha(commit.sha)} — ${commit.title}\n${path}: binary`;
    }
    return (
      `${shortSha(commit.sha)} — ${commit.title}\n` +
      `${path}: +${cell.additions} / -${cell.deletions}`
    );
  }

  function shortSha(sha: string): string {
    return sha.slice(0, 7);
  }

  function shortPath(path: string): string {
    const parts = path.split("/");
    if (parts.length <= 2) return path;
    return `…/${parts.slice(-2).join("/")}`;
  }

  function onCellClick(sha: string): void {
    diff.selectCommit(sha);
  }
</script>

<div class="heatmap">
  <div class="heatmap__header">
    <button class="heatmap__toggle" onclick={toggle}>
      <span class="heatmap__chevron" class:heatmap__chevron--open={expanded}
        >&#8250;</span
      >
      <span class="heatmap__label">Change map</span>
      {#if heatmap}
        <span class="heatmap__count">
          {rows.length}
          <span class="heatmap__count-suffix"> files × {heatmap.commits.length} commits</span>
        </span>
      {/if}
    </button>
  </div>

  {#if expanded}
    <div class="heatmap__body">
      {#if loading && !heatmap}
        <div class="heatmap__state">Loading change map…</div>
      {:else if loadError}
        <div class="heatmap__state heatmap__state--error">{loadError}</div>
      {:else if heatmap && rows.length === 0}
        <div class="heatmap__state">No changed files</div>
      {:else if heatmap}
        <div
          class="heatmap__grid"
          style="grid-template-columns: minmax(180px, 2fr) repeat({heatmap.commits.length}, minmax(56px, max-content)) minmax(72px, max-content);"
        >
          <!-- Header row: path column placeholder + commit SHAs + totals -->
          <div class="heatmap__corner" title="Files × commits (click a cell to scope the diff to that commit)">files ↓ / commits →</div>
          {#each heatmap.commits as c (c.sha)}
            <button
              class="heatmap__commit-header"
              onclick={() => onCellClick(c.sha)}
              title="{shortSha(c.sha)} — {c.title}"
            >
              <span class="heatmap__commit-sha">{shortSha(c.sha)}</span>
            </button>
          {/each}
          <div class="heatmap__total-header" title="Total additions / deletions across the PR range">total</div>

          <!-- Data rows -->
          {#each rows as row (row.path)}
            <div class="heatmap__path" title={row.path}>{shortPath(row.path)}</div>
            {#each heatmap.commits as c (c.sha)}
              {@const cell = row.byCommit.get(c.sha)}
              <button
                type="button"
                class="heatmap__cell"
                class:heatmap__cell--empty={!cell}
                style={cellStyle(cell)}
                onclick={() => onCellClick(c.sha)}
                title={cellTitle(c, row.path, cell)}
                aria-label={cellTitle(c, row.path, cell)}
              >
                {#if cell?.binary}
                  <span class="heatmap__cell-binary">bin</span>
                {:else if cell}
                  <span class="heatmap__add">+{cell.additions}</span>
                  <span class="heatmap__del">&minus;{cell.deletions}</span>
                {:else}
                  <span class="heatmap__cell-dash">·</span>
                {/if}
              </button>
            {/each}
            <div
              class="heatmap__totals"
              title="Sum across commits: +{row.totalAdds} / -{row.totalDels}"
            >
              <span class="heatmap__add">+{row.totalAdds}</span>
              <span class="heatmap__del">&minus;{row.totalDels}</span>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .heatmap {
    background: var(--bg-inset);
    border-bottom: 1px solid var(--diff-border);
  }

  .heatmap__header {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 2px 10px 2px 0;
  }

  .heatmap__toggle {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1;
    min-width: 0;
    padding: 4px 6px 4px 10px;
    border: none;
    background: none;
    cursor: pointer;
    text-align: left;
    color: var(--text-primary);
    border-radius: var(--radius-sm);
  }

  .heatmap__toggle:hover {
    background: var(--bg-surface-hover);
  }

  .heatmap__chevron {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    width: 12px;
    height: 12px;
    color: var(--text-muted);
    transition: transform 0.15s;
  }

  .heatmap__chevron--open {
    transform: rotate(90deg);
  }

  .heatmap__label {
    font-size: 11px;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.4px;
  }

  .heatmap__count {
    font-size: 10px;
    font-family: var(--font-mono);
    color: var(--text-muted);
    background: var(--diff-bg);
    border: 1px solid var(--diff-border);
    border-radius: 999px;
    padding: 1px 6px;
  }

  .heatmap__count-suffix {
    color: var(--text-muted);
  }

  .heatmap__body {
    padding: 6px 12px 10px 24px;
    max-height: 48vh;
    overflow: auto;
  }

  .heatmap__state {
    padding: 6px 2px;
    font-size: 11px;
    color: var(--text-muted);
  }

  .heatmap__state--error {
    color: var(--accent-red);
  }

  .heatmap__grid {
    display: grid;
    gap: 2px;
    align-items: stretch;
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .heatmap__corner {
    color: var(--text-muted);
    font-size: 10px;
    padding: 2px 6px 2px 0;
    align-self: end;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .heatmap__commit-header {
    display: flex;
    align-items: flex-end;
    justify-content: center;
    padding: 0 0 2px;
    background: none;
    border: none;
    border-bottom: 1px solid var(--border-muted);
    color: var(--text-muted);
    cursor: pointer;
    min-width: 0;
    overflow: hidden;
  }

  .heatmap__commit-header:hover {
    color: var(--accent-blue);
  }

  .heatmap__commit-sha {
    font-size: 9px;
    letter-spacing: 0;
  }

  .heatmap__path {
    font-size: 11px;
    color: var(--text-secondary);
    padding: 2px 6px 2px 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .heatmap__cell {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    min-height: 18px;
    padding: 1px 6px;
    border: 1px solid var(--border-muted);
    border-radius: 3px;
    background: var(--diff-bg);
    color: var(--text-secondary);
    font-size: 10px;
    line-height: 1;
    cursor: pointer;
    transition: outline 0.1s;
    white-space: nowrap;
  }

  .heatmap__cell:hover {
    outline: 1px solid var(--accent-blue);
    outline-offset: 1px;
    z-index: 1;
  }

  .heatmap__cell--empty {
    background: var(--bg-inset);
    border-color: var(--border-muted);
    color: var(--text-muted);
    cursor: default;
  }

  .heatmap__cell--empty:hover {
    outline: none;
  }

  .heatmap__cell-dash {
    color: var(--text-muted);
    opacity: 0.5;
  }

  .heatmap__cell-binary {
    color: var(--accent-purple);
    font-weight: 600;
  }

  .heatmap__add {
    color: var(--diff-add-text, var(--accent-green));
  }

  .heatmap__del {
    color: var(--diff-del-text, var(--accent-red));
  }

  .heatmap__total-header {
    align-self: end;
    padding: 2px 6px;
    font-size: 10px;
    color: var(--text-muted);
    border-bottom: 1px solid var(--border-muted);
    text-align: center;
  }

  .heatmap__totals {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    padding: 1px 8px;
    font-size: 10px;
    color: var(--text-secondary);
    font-weight: 600;
    white-space: nowrap;
  }
</style>
