import type { LocalWorktree } from "../api/types.js";
import type { MiddlemanClient } from "../types.js";
import type { components } from "../api/generated/schema.js";

export type ChangedFile = components["schemas"]["ChangedFileResponse"];

export interface WorktreesStoreOptions {
  client: MiddlemanClient;
}

interface ChangedFilesEntry {
  files: ChangedFile[];
  loading: boolean;
  error: string | null;
  fetchedAt: number;
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
  let changedFilesById = $state<Record<number, ChangedFilesEntry>>({});
  let selectedId = $state<number | null>(null);

  function getWorktrees(): LocalWorktree[] {
    return worktrees;
  }
  function isLoading(): boolean {
    return loading;
  }
  function getError(): string | null {
    return storeError;
  }
  function getById(id: number): LocalWorktree | null {
    return worktrees.find((w) => w.id === id) ?? null;
  }
  function getChangedFiles(id: number): ChangedFilesEntry | null {
    return changedFilesById[id] ?? null;
  }
  function getSelectedId(): number | null {
    return selectedId;
  }
  function selectWorktree(id: number | null): void {
    selectedId = id;
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

  async function loadChangedFiles(id: number): Promise<void> {
    const prev = changedFilesById[id] ?? {
      files: [],
      loading: false,
      error: null,
      fetchedAt: 0,
    };
    changedFilesById = {
      ...changedFilesById,
      [id]: { ...prev, loading: true, error: null },
    };
    try {
      const { data, error } = await apiClient.GET(
        "/worktrees/{id}/changed-files",
        { params: { path: { id } } },
      );
      if (error) {
        throw new Error(
          apiErrorMessage(error, "failed to load worktree files"),
        );
      }
      changedFilesById = {
        ...changedFilesById,
        [id]: {
          files: data?.files ?? [],
          loading: false,
          error: null,
          fetchedAt: Date.now(),
        },
      };
    } catch (err) {
      changedFilesById = {
        ...changedFilesById,
        [id]: {
          ...prev,
          loading: false,
          error: err instanceof Error ? err.message : String(err),
        },
      };
    }
  }

  return {
    getWorktrees,
    isLoading,
    getError,
    getById,
    worktreesByRepo,
    loadWorktrees,
    getChangedFiles,
    loadChangedFiles,
    getSelectedId,
    selectWorktree,
  };
}

export type WorktreesStore = ReturnType<typeof createWorktreesStore>;
