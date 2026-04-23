import type { MiddlemanClient } from "../types.js";

export interface AuthorGroup {
  id: number;
  name: string;
  members: string[];
}

export interface AuthorGroupsStoreOptions {
  client: MiddlemanClient;
}

// Tracks the id of the group whose members are currently applied as
// the PR-author filter. Scoped to localStorage so a page reload
// doesn't forget which group the reviewer was using.
const ACTIVE_KEY = "author-group-active-id";

function loadActiveId(): number | null {
  try {
    const raw = localStorage.getItem(ACTIVE_KEY);
    if (!raw) return null;
    const parsed = Number.parseInt(raw, 10);
    return Number.isFinite(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function saveActiveId(id: number | null): void {
  try {
    if (id === null) localStorage.removeItem(ACTIVE_KEY);
    else localStorage.setItem(ACTIVE_KEY, String(id));
  } catch {
    /* ignore */
  }
}

function errMsg(err: { detail?: string; title?: string } | undefined, fallback: string): string {
  return err?.detail ?? err?.title ?? fallback;
}

export function createAuthorGroupsStore(opts: AuthorGroupsStoreOptions) {
  const api = opts.client;

  let groups = $state<AuthorGroup[]>([]);
  let loading = $state(false);
  let errorMsg = $state<string | null>(null);
  let activeId = $state<number | null>(loadActiveId());

  function toAuthorGroup(g: {
    id: number;
    name: string;
    members?: string[] | null;
  }): AuthorGroup {
    return { id: g.id, name: g.name, members: g.members ?? [] };
  }

  async function load(): Promise<void> {
    loading = true;
    errorMsg = null;
    try {
      const { data, error } = await api.GET("/author-groups", {});
      if (error || !data) {
        errorMsg = errMsg(error as { detail?: string; title?: string }, "Failed to load author groups");
        return;
      }
      const list = data.groups ?? [];
      groups = list.map(toAuthorGroup);
      // If the active id no longer exists (deleted elsewhere), clear it.
      if (activeId !== null && !groups.some((g) => g.id === activeId)) {
        activeId = null;
        saveActiveId(null);
      }
    } catch (err) {
      errorMsg = err instanceof Error ? err.message : String(err);
    } finally {
      loading = false;
    }
  }

  async function create(name: string, members: string[]): Promise<AuthorGroup | null> {
    errorMsg = null;
    const { data, error } = await api.POST("/author-groups", {
      body: { name, members },
    });
    if (error || !data) {
      errorMsg = errMsg(error as { detail?: string; title?: string }, "Failed to create group");
      return null;
    }
    const g = toAuthorGroup(data);
    groups = [...groups, g].sort((a, b) =>
      a.name.localeCompare(b.name, undefined, { sensitivity: "base" }),
    );
    return g;
  }

  async function update(id: number, name: string, members: string[]): Promise<AuthorGroup | null> {
    errorMsg = null;
    const { data, error } = await api.PUT("/author-groups/{id}", {
      params: { path: { id } },
      body: { name, members },
    });
    if (error || !data) {
      errorMsg = errMsg(error as { detail?: string; title?: string }, "Failed to update group");
      return null;
    }
    const g = toAuthorGroup(data);
    groups = groups
      .map((x) => (x.id === id ? g : x))
      .sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" }));
    return g;
  }

  async function remove(id: number): Promise<boolean> {
    errorMsg = null;
    const { error } = await api.DELETE("/author-groups/{id}", {
      params: { path: { id } },
    });
    if (error) {
      errorMsg = errMsg(error as { detail?: string; title?: string }, "Failed to delete group");
      return false;
    }
    groups = groups.filter((g) => g.id !== id);
    if (activeId === id) {
      activeId = null;
      saveActiveId(null);
    }
    return true;
  }

  function setActive(id: number | null): void {
    activeId = id;
    saveActiveId(id);
  }

  function getActive(): AuthorGroup | null {
    if (activeId === null) return null;
    return groups.find((g) => g.id === activeId) ?? null;
  }

  return {
    list: () => groups,
    isLoading: () => loading,
    getError: () => errorMsg,
    load,
    create,
    update,
    remove,
    setActive,
    getActiveId: () => activeId,
    getActive,
  };
}

export type AuthorGroupsStore = ReturnType<typeof createAuthorGroupsStore>;
