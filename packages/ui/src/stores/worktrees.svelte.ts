import type { LocalWorktree } from "../api/types.js";
import type { MiddlemanClient } from "../types.js";

export interface WorktreesStoreOptions {
  client: MiddlemanClient;
}

function apiErrorMessage(
  error: { detail?: string; title?: string },
  fallback: string,
): string {
  return error.detail ?? error.title ?? fallback;
}

// createWorktreesStore manages the list of git worktrees discovered
// under each enrolled repo's configured local_path. It mirrors the
// shape of createPullsStore: one fetch endpoint, a flat list in
// state, and a derived grouping helper for the sidebar.
export function createWorktreesStore(opts: WorktreesStoreOptions) {
  const apiClient = opts.client;

  let worktrees = $state<LocalWorktree[]>([]);
  let loading = $state(false);
  let storeError = $state<string | null>(null);

  function getWorktrees(): LocalWorktree[] {
    return worktrees;
  }
  function isLoading(): boolean {
    return loading;
  }
  function getError(): string | null {
    return storeError;
  }

  // Group by `${repo_owner}/${repo_name}` so callers can render a
  // section per repo. Insertion order follows the server response,
  // which orders by (owner, name, path) for stability.
  function worktreesByRepo(): Map<string, LocalWorktree[]> {
    const map = new Map<string, LocalWorktree[]>();
    for (const w of worktrees) {
      const key = `${w.repo_owner}/${w.repo_name}`;
      const bucket = map.get(key);
      if (bucket) bucket.push(w);
      else map.set(key, [w]);
    }
    return map;
  }

  async function loadWorktrees(): Promise<void> {
    loading = true;
    storeError = null;
    try {
      const { data, error } = await apiClient.GET("/worktrees", {});
      if (error) {
        throw new Error(
          apiErrorMessage(error, "failed to load worktrees"),
        );
      }
      worktrees = data?.worktrees ?? [];
    } catch (err) {
      storeError =
        err instanceof Error ? err.message : String(err);
    } finally {
      loading = false;
    }
  }

  return {
    getWorktrees,
    isLoading,
    getError,
    worktreesByRepo,
    loadWorktrees,
  };
}

export type WorktreesStore = ReturnType<typeof createWorktreesStore>;
