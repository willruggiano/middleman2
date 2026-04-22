import type { components } from "../api/generated/schema.js";

export type AIBrief = components["schemas"]["AiBriefResponse"];

export interface BriefStoreOptions {
  getBasePath?: () => string;
}

export function createBriefStore(opts?: BriefStoreOptions) {
  const getBasePath = opts?.getBasePath ?? (() => "/");

  let owner = $state("");
  let name = $state("");
  let number = $state(0);
  let brief = $state<AIBrief | null>(null);
  let loading = $state(false);
  let errorMsg = $state<string | null>(null);
  let pollHandle: ReturnType<typeof setInterval> | null = null;

  function prefix(): string {
    return `${getBasePath()}api/v1/repos/${encodeURIComponent(owner)}/${encodeURIComponent(name)}/pulls/${number}`;
  }

  function current(): AIBrief | null {
    return brief;
  }

  function isLoading(): boolean {
    return loading;
  }

  function getError(): string | null {
    return errorMsg;
  }

  // The brief is "in flight" while Claude is still generating — UI
  // can show a spinner and keep polling.
  function isInFlight(): boolean {
    return brief !== null && (brief.status === "queued" || brief.status === "running");
  }

  // Stale if there's a brief but its head_sha differs from the PR's
  // current head SHA. Caller supplies currentHeadSha (from commits
  // list in the diff store).
  function isStale(currentHeadSha: string): boolean {
    if (!brief) return false;
    if (!currentHeadSha) return false;
    return brief.head_sha !== currentHeadSha;
  }

  async function fetchLatest(): Promise<void> {
    if (!owner) return;
    try {
      const res = await fetch(`${prefix()}/ai-brief`);
      if (res.status === 404) {
        brief = null;
        return;
      }
      if (!res.ok) {
        errorMsg = `Fetch brief: ${res.status} ${res.statusText}`;
        return;
      }
      brief = (await res.json()) as AIBrief;
      errorMsg = null;
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : String(err);
    }
  }

  async function generate(depth: "quick" | "deep"): Promise<void> {
    if (!owner || loading) return;
    loading = true;
    errorMsg = null;
    try {
      const res = await fetch(`${prefix()}/ai-brief`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ depth }),
      });
      if (!res.ok) {
        let detail = `${res.status} ${res.statusText}`;
        try {
          const data = (await res.json()) as { detail?: string };
          if (data?.detail) detail = data.detail;
        } catch {
          /* ignore */
        }
        errorMsg = detail;
        return;
      }
      brief = (await res.json()) as AIBrief;
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : String(err);
    } finally {
      loading = false;
    }
  }

  async function remove(): Promise<void> {
    if (!owner) return;
    try {
      await fetch(`${prefix()}/ai-brief`, { method: "DELETE" });
    } finally {
      brief = null;
    }
  }

  function start(o: string, n: string, num: number): void {
    const changed = o !== owner || n !== name || num !== number;
    owner = o;
    name = n;
    number = num;
    if (changed) {
      brief = null;
      errorMsg = null;
    }
    void fetchLatest();
    stopPolling();
    pollHandle = setInterval(() => {
      if (isInFlight()) {
        void fetchLatest();
      }
    }, 2000);
  }

  function stopPolling(): void {
    if (pollHandle !== null) {
      clearInterval(pollHandle);
      pollHandle = null;
    }
  }

  function stop(): void {
    stopPolling();
    owner = "";
    name = "";
    number = 0;
    brief = null;
    errorMsg = null;
  }

  return {
    current,
    isLoading,
    isInFlight,
    isStale,
    getError,
    fetchLatest,
    generate,
    remove,
    start,
    stop,
  };
}

export type BriefStore = ReturnType<typeof createBriefStore>;
