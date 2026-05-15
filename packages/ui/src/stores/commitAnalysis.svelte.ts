import type { components } from "../api/generated/schema.js";

export type AICommitAnalysis = components["schemas"]["AiCommitAnalysisResponse"];

export interface CommitAnalysisStoreOptions {
  getBasePath?: () => string;
}

// createCommitAnalysisStore is a per-PR cache of per-commit Claude
// analyses. Each commit SHA carries its own row (status, content,
// error). The active PR is set via setPR(); switching PRs clears
// the in-memory cache so we don't leak entries from one PR into
// another.
//
// Polling: while any analysis in the cache is queued/running we
// re-fetch every 2s. Stops polling when nothing is in flight.
export function createCommitAnalysisStore(opts?: CommitAnalysisStoreOptions) {
  const getBasePath = opts?.getBasePath ?? (() => "/");

  let owner = $state("");
  let name = $state("");
  let number = $state(0);
  // sha -> AICommitAnalysis. Using a plain object rather than Map so
  // Svelte 5 $state can track keyed reads naturally.
  let byCommit = $state<Record<string, AICommitAnalysis>>({});
  let pollHandle: ReturnType<typeof setInterval> | null = null;
  let errorMsg = $state<string | null>(null);

  function prefix(): string {
    return `${getBasePath()}api/v1/repos/${encodeURIComponent(owner)}/${encodeURIComponent(name)}/pulls/${number}`;
  }

  function setPR(o: string, n: string, num: number): void {
    const changed = o !== owner || n !== name || num !== number;
    owner = o;
    name = n;
    number = num;
    if (changed) {
      byCommit = {};
      errorMsg = null;
      stopPolling();
    }
  }

  function clear(): void {
    byCommit = {};
    errorMsg = null;
    stopPolling();
    owner = "";
    name = "";
    number = 0;
  }

  function get(sha: string): AICommitAnalysis | null {
    return byCommit[sha] ?? null;
  }

  function getError(): string | null {
    return errorMsg;
  }

  function isInFlight(sha: string): boolean {
    const a = byCommit[sha];
    return !!a && (a.status === "queued" || a.status === "running");
  }

  function anyInFlight(): boolean {
    for (const a of Object.values(byCommit)) {
      if (a.status === "queued" || a.status === "running") return true;
    }
    return false;
  }

  async function fetchFor(sha: string): Promise<void> {
    if (!owner || !sha) return;
    try {
      const res = await fetch(
        `${prefix()}/commits/${encodeURIComponent(sha)}/analyze`,
      );
      if (res.status === 404) {
        // No analysis yet — drop any stale cache entry.
        const next = { ...byCommit };
        delete next[sha];
        byCommit = next;
        return;
      }
      if (!res.ok) {
        errorMsg = `Fetch analysis: ${res.status} ${res.statusText}`;
        return;
      }
      const a = (await res.json()) as AICommitAnalysis;
      byCommit = { ...byCommit, [sha]: a };
      errorMsg = null;
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : String(err);
    }
  }

  async function generate(sha: string): Promise<void> {
    if (!owner || !sha) return;
    errorMsg = null;
    try {
      const res = await fetch(
        `${prefix()}/commits/${encodeURIComponent(sha)}/analyze`,
        {
          method: "POST",
          headers: { "content-type": "application/json" },
        },
      );
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        errorMsg =
          (body as Record<string, string>).detail ??
          (body as Record<string, string>).title ??
          `Generate analysis: HTTP ${res.status}`;
        return;
      }
      const a = (await res.json()) as AICommitAnalysis;
      byCommit = { ...byCommit, [sha]: a };
      ensurePolling();
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : String(err);
    }
  }

  async function remove(sha: string): Promise<void> {
    if (!owner || !sha) return;
    try {
      await fetch(
        `${prefix()}/commits/${encodeURIComponent(sha)}/analyze`,
        {
          method: "DELETE",
          headers: { "content-type": "application/json" },
        },
      );
      const next = { ...byCommit };
      delete next[sha];
      byCommit = next;
    } catch {
      /* swallow */
    }
  }

  function ensurePolling(): void {
    if (pollHandle !== null) return;
    pollHandle = setInterval(() => {
      if (!anyInFlight()) {
        stopPolling();
        return;
      }
      // Refresh every in-flight row. We don't fan out here; the API
      // call cost is dominated by the round-trip, not the work, and
      // there are usually 0–1 in-flight commits at a time.
      for (const sha of Object.keys(byCommit)) {
        const a = byCommit[sha];
        if (a && (a.status === "queued" || a.status === "running")) {
          void fetchFor(sha);
        }
      }
    }, 2_000);
  }

  function stopPolling(): void {
    if (pollHandle !== null) {
      clearInterval(pollHandle);
      pollHandle = null;
    }
  }

  return {
    setPR,
    clear,
    get,
    getError,
    isInFlight,
    fetchFor,
    generate,
    remove,
  };
}

export type CommitAnalysisStore = ReturnType<typeof createCommitAnalysisStore>;
